package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"scrc/internal/domain/execution"
)

const scriptFilename = "script.py"

// Runner manages execution of scripts inside Docker containers via the official SDK.
type Runner struct {
	cli      *client.Client
	image    string
	workdir  string
	limits   execution.RunLimits
	pullOnce sync.Once
	pullErr  error
}

// Config describes how to create a new Runner service.
type Config struct {
	Image         string
	Workdir       string
	DefaultLimits execution.RunLimits
}

// New creates a new Runner using the provided configuration.
func New(cfg Config) (*Runner, error) {
	if cfg.Image == "" {
		return nil, fmt.Errorf("image must be provided")
	}
	if cfg.Workdir == "" {
		cfg.Workdir = "/tmp"
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	return &Runner{
		cli:     cli,
		image:   cfg.Image,
		workdir: cfg.Workdir,
		limits:  normalizeLimits(cfg.DefaultLimits),
	}, nil
}

// Close releases the underlying Docker client resources.
func (r *Runner) Close() error {
	if r.cli == nil {
		return nil
	}
	return r.cli.Close()
}

// RunPython executes the provided Python source inside a new container instance.
func (r *Runner) RunPython(ctx context.Context, source string, limits execution.RunLimits) (*execution.Result, error) {
	if err := r.ensureImage(ctx); err != nil {
		return nil, err
	}

	effectiveLimits := r.effectiveLimits(limits)

	containerID, cleanup, err := r.createContainer(ctx, effectiveLimits)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if err := r.copyScript(ctx, containerID, source); err != nil {
		return nil, fmt.Errorf("copy script: %w", err)
	}

	start := time.Now()
	if err := r.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
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

func (r *Runner) ensureImage(ctx context.Context) error {
	r.pullOnce.Do(func() {
		reader, err := r.cli.ImagePull(ctx, r.image, image.PullOptions{})
		if err != nil {
			r.pullErr = fmt.Errorf("pull image: %w", err)
			return
		}
		defer reader.Close()
		_, err = io.Copy(io.Discard, reader)
		if err != nil {
			r.pullErr = fmt.Errorf("consume pull output: %w", err)
		}
	})
	return r.pullErr
}

func (r *Runner) createContainer(ctx context.Context, limits execution.RunLimits) (string, func(), error) {
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
			Image:        r.image,
			Cmd:          []string{"python", r.workdir + "/" + scriptFilename},
			AttachStdout: true,
			AttachStderr: true,
			WorkingDir:   r.workdir,
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

func (r *Runner) copyScript(ctx context.Context, containerID, source string) error {
	tarReader, err := makeScriptArchive(source)
	if err != nil {
		return err
	}

	return r.cli.CopyToContainer(ctx, containerID, r.workdir, tarReader, container.CopyToContainerOptions{AllowOverwriteDirWithFile: true})
}

func makeScriptArchive(source string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	header := &tar.Header{
		Name:    scriptFilename,
		Mode:    0o644,
		Size:    int64(len(source)),
		ModTime: time.Now(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return nil, fmt.Errorf("write tar header: %w", err)
	}

	if _, err := tw.Write([]byte(source)); err != nil {
		return nil, fmt.Errorf("write script contents: %w", err)
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar writer: %w", err)
	}

	return bytes.NewReader(buf.Bytes()), nil
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
