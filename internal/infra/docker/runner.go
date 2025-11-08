package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"scrc/internal/domain/execution"
	"scrc/internal/ports"
)

const (
	pythonScriptFilename = "script.py"
	goSourceFilename     = "main.go"
	goBinaryFilename     = "program"
)

// Config describes how to create a new Runner service.
type Config struct {
	Languages     map[execution.Language]LanguageConfig
	DefaultLimits execution.RunLimits
}

// LanguageConfig specifies container settings for a single language.
type LanguageConfig struct {
	Image   string
	Workdir string
}

type languageStrategy interface {
	Prepare(ctx context.Context, runner *Runner, runtime *languageRuntime, script execution.Script) (ports.PreparedScript, *execution.Result, error)
}

// Runner manages execution of scripts inside Docker containers via the official SDK.
type Runner struct {
	cli    *client.Client
	limits execution.RunLimits
	langs  map[execution.Language]*languageRuntime
}

type languageRuntime struct {
	config   LanguageConfig
	strategy languageStrategy
	pullOnce sync.Once
	pullErr  error
}

// ensure Runner implements ports.Runner.
var _ ports.Runner = (*Runner)(nil)

// New creates a new Runner using the provided configuration.
func New(cfg Config) (*Runner, error) {
	if len(cfg.Languages) == 0 {
		return nil, fmt.Errorf("at least one language must be configured")
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	langs := make(map[execution.Language]*languageRuntime, len(cfg.Languages))
	for lang, langCfg := range cfg.Languages {
		if langCfg.Image == "" {
			return nil, fmt.Errorf("language %q missing image configuration", lang)
		}
		if langCfg.Workdir == "" {
			langCfg.Workdir = "/tmp"
		}

		strategy, err := strategyForLanguage(lang)
		if err != nil {
			return nil, err
		}

		langs[lang] = &languageRuntime{
			config:   langCfg,
			strategy: strategy,
		}
	}

	return &Runner{
		cli:    cli,
		limits: normalizeLimits(cfg.DefaultLimits),
		langs:  langs,
	}, nil
}

// Close releases the underlying Docker client resources.
func (r *Runner) Close() error {
	if r.cli == nil {
		return nil
	}
	return r.cli.Close()
}

// Prepare prepares the supplied script for execution.
func (r *Runner) Prepare(ctx context.Context, script execution.Script) (ports.PreparedScript, *execution.Result, error) {
	runtime, ok := r.langs[script.Language]
	if !ok {
		return nil, nil, fmt.Errorf("unsupported language %q", script.Language)
	}

	if err := r.ensureImage(ctx, runtime); err != nil {
		return nil, nil, err
	}

	return runtime.strategy.Prepare(ctx, r, runtime, script)
}

func strategyForLanguage(lang execution.Language) (languageStrategy, error) {
	switch lang {
	case execution.LanguagePython:
		return &pythonStrategy{}, nil
	case execution.LanguageGo:
		return &goStrategy{}, nil
	default:
		return nil, fmt.Errorf("no docker strategy registered for language %q", lang)
	}
}

// pythonStrategy implements languageStrategy for Python scripts.
type pythonStrategy struct{}

func (p *pythonStrategy) Prepare(ctx context.Context, runner *Runner, runtime *languageRuntime, script execution.Script) (ports.PreparedScript, *execution.Result, error) {
	return &pythonPreparedScript{
		runner:  runner,
		runtime: runtime,
		script:  script,
	}, nil, nil
}

type pythonPreparedScript struct {
	runner  *Runner
	runtime *languageRuntime
	script  execution.Script
}

func (p *pythonPreparedScript) Run(ctx context.Context, stdin string) (*execution.Result, error) {
	return p.runner.runProgram(ctx, p.runtime, p.script.Limits, []string{"python", pythonScriptFilename}, []fileSpec{
		{
			Name: pythonScriptFilename,
			Mode: 0o644,
			Data: []byte(p.script.Source),
		},
	}, stdin, true)
}

func (p *pythonPreparedScript) Close() error {
	return nil
}

// goStrategy implements languageStrategy for Go programs.
type goStrategy struct{}

func (g *goStrategy) Prepare(ctx context.Context, runner *Runner, runtime *languageRuntime, script execution.Script) (ports.PreparedScript, *execution.Result, error) {
	effectiveLimits := runner.effectiveLimits(script.Limits)

	containerID, cleanup, err := runner.createContainer(ctx, runtime, effectiveLimits, []string{"go", "build", "-o", goBinaryFilename, goSourceFilename}, false)
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()

	if err := runner.copyFiles(ctx, containerID, runtime.config.Workdir, []fileSpec{
		{
			Name: goSourceFilename,
			Mode: 0o644,
			Data: []byte(script.Source),
		},
	}); err != nil {
		return nil, nil, fmt.Errorf("copy source: %w", err)
	}

	start := time.Now()
	if err := runner.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, nil, fmt.Errorf("start container: %w", err)
	}

	waitCtx := ctx
	var cancel context.CancelFunc
	if effectiveLimits.TimeLimit > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, effectiveLimits.TimeLimit)
	}
	status, err := runner.waitForExit(waitCtx, containerID)
	if cancel != nil {
		cancel()
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && effectiveLimits.TimeLimit > 0 && ctx.Err() == nil {
			result, handleErr := runner.handleTimeLimit(containerID, start)
			if handleErr != nil {
				return nil, nil, handleErr
			}
			result.Status = execution.StatusBuildFail
			return nil, result, nil
		}
		return nil, nil, err
	}

	inspectCtx := ctx
	if inspectCtx.Err() != nil {
		inspectCtx = context.Background()
	}
	inspect, err := runner.cli.ContainerInspect(inspectCtx, containerID)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect container: %w", err)
	}

	logCtx := ctx
	if logCtx.Err() != nil {
		logCtx = context.Background()
	}
	stdout, stderr, err := runner.fetchLogs(logCtx, containerID)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch logs: %w", err)
	}

	buildResult := &execution.Result{
		Status:   execution.StatusOK,
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: status.StatusCode,
		Duration: time.Since(start),
	}

	if inspect.State != nil && inspect.State.OOMKilled {
		buildResult.Status = execution.StatusMemoryLimit
	}

	if buildResult.Status != execution.StatusOK || buildResult.ExitCode != 0 {
		buildResult.Status = execution.StatusBuildFail
		return nil, buildResult, nil
	}

	binaryPath := path.Join(runtime.config.Workdir, goBinaryFilename)
	binaryData, err := runner.copyFileFromContainer(ctx, containerID, binaryPath)
	if err != nil {
		return nil, nil, fmt.Errorf("extract compiled binary: %w", err)
	}

	return &goPreparedScript{
		runner:  runner,
		runtime: runtime,
		binary:  binaryData,
		limits:  script.Limits,
	}, nil, nil
}

type goPreparedScript struct {
	runner  *Runner
	runtime *languageRuntime
	binary  []byte
	limits  execution.RunLimits
}

func (g *goPreparedScript) Run(ctx context.Context, stdin string) (*execution.Result, error) {
	return g.runner.runProgram(ctx, g.runtime, g.limits, []string{"./" + goBinaryFilename}, []fileSpec{
		{
			Name: goBinaryFilename,
			Mode: 0o755,
			Data: g.binary,
		},
	}, stdin, true)
}

func (g *goPreparedScript) Close() error {
	return nil
}

type fileSpec struct {
	Name string
	Mode int64
	Data []byte
}

func (r *Runner) ensureImage(ctx context.Context, runtime *languageRuntime) error {
	runtime.pullOnce.Do(func() {
		reader, err := r.cli.ImagePull(ctx, runtime.config.Image, image.PullOptions{})
		if err != nil {
			runtime.pullErr = fmt.Errorf("pull image: %w", err)
			return
		}
		defer reader.Close()
		_, err = io.Copy(io.Discard, reader)
		if err != nil {
			runtime.pullErr = fmt.Errorf("consume pull output: %w", err)
		}
	})
	return runtime.pullErr
}

func (r *Runner) effectiveLimits(request execution.RunLimits) execution.RunLimits {
	effective := normalizeLimits(r.limits)
	overrides := normalizeLimits(request)

	if overrides.TimeLimit > 0 {
		effective.TimeLimit = overrides.TimeLimit
	}
	if overrides.MemoryLimitBytes > 0 {
		effective.MemoryLimitBytes = overrides.MemoryLimitBytes
	}

	return effective
}

func (r *Runner) runProgram(
	ctx context.Context,
	runtime *languageRuntime,
	limits execution.RunLimits,
	command []string,
	files []fileSpec,
	stdin string,
	attachStdin bool,
) (*execution.Result, error) {
	effectiveLimits := r.effectiveLimits(limits)

	containerID, cleanup, err := r.createContainer(ctx, runtime, effectiveLimits, command, attachStdin)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if err := r.copyFiles(ctx, containerID, runtime.config.Workdir, files); err != nil {
		return nil, fmt.Errorf("copy files: %w", err)
	}

	var attach types.HijackedResponse
	if attachStdin {
		attachCtx := ctx
		if attachCtx.Err() != nil {
			attachCtx = context.Background()
		}
		attach, err = r.cli.ContainerAttach(attachCtx, containerID, container.AttachOptions{
			Stream: true,
			Stdin:  true,
		})
		if err != nil {
			return nil, fmt.Errorf("attach container: %w", err)
		}
		defer attach.Close()
	}

	start := time.Now()
	if err := r.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}

	if attachStdin && attach.Conn != nil {
		reader := strings.NewReader(stdin)
		if _, err := io.Copy(attach.Conn, reader); err != nil {
			return nil, fmt.Errorf("write stdin: %w", err)
		}
		if closer, ok := attach.Conn.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
	}

	waitCtx := ctx
	var cancel context.CancelFunc
	if effectiveLimits.TimeLimit > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, effectiveLimits.TimeLimit)
	}
	status, err := r.waitForExit(waitCtx, containerID)
	if cancel != nil {
		cancel()
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && effectiveLimits.TimeLimit > 0 && ctx.Err() == nil {
			return r.handleTimeLimit(containerID, start)
		}
		return nil, err
	}

	inspectCtx := ctx
	if inspectCtx.Err() != nil {
		inspectCtx = context.Background()
	}

	inspect, err := r.cli.ContainerInspect(inspectCtx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}

	logCtx := ctx
	if logCtx.Err() != nil {
		logCtx = context.Background()
	}

	stdout, stderr, err := r.fetchLogs(logCtx, containerID)
	if err != nil {
		return nil, fmt.Errorf("fetch logs: %w", err)
	}

	result := &execution.Result{
		Status:   execution.StatusOK,
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: status.StatusCode,
		Duration: time.Since(start),
	}

	if inspect.State != nil && inspect.State.OOMKilled {
		result.Status = execution.StatusMemoryLimit
	}

	return result, nil
}

func (r *Runner) createContainer(ctx context.Context, runtime *languageRuntime, limits execution.RunLimits, cmd []string, attachStdin bool) (string, func(), error) {
	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			NanoCPUs: 1_000_000_000,
		},
	}
	if limits.MemoryLimitBytes > 0 {
		hostConfig.Resources.Memory = limits.MemoryLimitBytes
		hostConfig.Resources.MemorySwap = limits.MemoryLimitBytes
	}

	resp, err := r.cli.ContainerCreate(
		ctx,
		&container.Config{
			Image:        runtime.config.Image,
			Cmd:          cmd,
			AttachStdout: true,
			AttachStderr: true,
			AttachStdin:  attachStdin,
			OpenStdin:    attachStdin,
			StdinOnce:    attachStdin,
			WorkingDir:   runtime.config.Workdir,
		},
		hostConfig,
		nil,
		nil,
		"",
	)
	if err != nil {
		return "", nil, fmt.Errorf("create container: %w", err)
	}

	cleanup := func() {
		_ = r.cli.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
	}

	return resp.ID, cleanup, nil
}

func (r *Runner) copyFiles(ctx context.Context, containerID, workdir string, files []fileSpec) error {
	if len(files) == 0 {
		return nil
	}

	reader, err := makeArchive(files)
	if err != nil {
		return err
	}

	return r.cli.CopyToContainer(ctx, containerID, workdir, reader, container.CopyToContainerOptions{AllowOverwriteDirWithFile: true})
}

func makeArchive(files []fileSpec) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	now := time.Now()
	for _, file := range files {
		mode := file.Mode
		if mode == 0 {
			mode = 0o644
		}

		header := &tar.Header{
			Name:    file.Name,
			Mode:    mode,
			Size:    int64(len(file.Data)),
			ModTime: now,
		}

		if err := tw.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("write tar header: %w", err)
		}

		if _, err := tw.Write(file.Data); err != nil {
			return nil, fmt.Errorf("write tar contents: %w", err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar writer: %w", err)
	}

	return bytes.NewReader(buf.Bytes()), nil
}

func (r *Runner) copyFileFromContainer(ctx context.Context, containerID, sourcePath string) ([]byte, error) {
	reader, _, err := r.cli.CopyFromContainer(ctx, containerID, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("copy from container: %w", err)
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}

		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("read file contents: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("file %s not found in container archive", sourcePath)
}

func (r *Runner) handleTimeLimit(containerID string, start time.Time) (*execution.Result, error) {
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStop()

	if err := r.cli.ContainerStop(stopCtx, containerID, container.StopOptions{}); err != nil && !client.IsErrNotFound(err) {
		return nil, fmt.Errorf("stop container after time limit: %w", err)
	}

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelWait()

	status, waitErr := r.waitForExit(waitCtx, containerID)
	if waitErr != nil && !errors.Is(waitErr, context.DeadlineExceeded) && !client.IsErrNotFound(waitErr) {
		return nil, fmt.Errorf("wait for container after time limit: %w", waitErr)
	}

	stdout, stderr, err := r.fetchLogs(context.Background(), containerID)
	if err != nil {
		return nil, fmt.Errorf("fetch logs: %w", err)
	}

	exitCode := int64(-1)
	if status != nil {
		exitCode = status.StatusCode
	}

	return &execution.Result{
		Status:   execution.StatusTimeLimit,
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Duration: time.Since(start),
	}, nil
}

func normalizeLimits(l execution.RunLimits) execution.RunLimits {
	if l.TimeLimit < 0 {
		l.TimeLimit = 0
	}
	if l.MemoryLimitBytes < 0 {
		l.MemoryLimitBytes = 0
	}
	return l
}

func (r *Runner) waitForExit(ctx context.Context, containerID string) (*container.WaitResponse, error) {
	statusCh, errCh := r.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case status := <-statusCh:
		if status.Error != nil {
			return nil, fmt.Errorf("container error: %s", status.Error.Message)
		}
		return &status, nil
	case err := <-errCh:
		return nil, fmt.Errorf("wait for container: %w", err)
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for container: %w", ctx.Err())
	}
}

func (r *Runner) fetchLogs(ctx context.Context, containerID string) (stdout, stderr string, err error) {
	logs, err := r.cli.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", "", err
	}
	defer logs.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, logs); err != nil {
		return "", "", err
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}
