package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	specs "github.com/opencontainers/image-spec/specs-go/v1"

	"scrc/internal/domain/execution"
)

func TestNormalizeLimits(t *testing.T) {
	t.Parallel()

	limits := normalizeLimits(execution.RunLimits{TimeLimit: -10 * time.Second, MemoryLimitBytes: -1})
	if limits.TimeLimit != 0 {
		t.Fatalf("expected zero time limit, got %v", limits.TimeLimit)
	}
	if limits.MemoryLimitBytes != 0 {
		t.Fatalf("expected zero memory limit, got %d", limits.MemoryLimitBytes)
	}
}

func TestEffectiveLimitsOverrides(t *testing.T) {
	t.Parallel()

	runner := &Runner{limits: normalizeLimits(execution.RunLimits{TimeLimit: 5 * time.Second, MemoryLimitBytes: 1024})}
	overrides := execution.RunLimits{TimeLimit: 2 * time.Second}

	got := runner.effectiveLimits(overrides)
	if got.TimeLimit != overrides.TimeLimit {
		t.Fatalf("expected time limit %v, got %v", overrides.TimeLimit, got.TimeLimit)
	}
	if got.MemoryLimitBytes != int64(1024) {
		t.Fatalf("expected inherited memory limit 1024, got %d", got.MemoryLimitBytes)
	}
}

func TestStrategyForLanguage(t *testing.T) {
	t.Parallel()

	pyStrategy, err := strategyForLanguage(execution.LanguagePython)
	if err != nil {
		t.Fatalf("unexpected error for python strategy: %v", err)
	}
	if _, ok := pyStrategy.(*pythonStrategy); !ok {
		t.Fatalf("expected pythonStrategy implementation")
	}

	_, err = strategyForLanguage("unknown")
	if err == nil {
		t.Fatalf("expected error for unknown language")
	}
}

func TestMakeArchive(t *testing.T) {
	t.Parallel()

	data := []byte("print('hello')\n")
	reader, err := makeArchive([]fileSpec{{Name: "script.py", Mode: 0o600, Data: data}})
	if err != nil {
		t.Fatalf("makeArchive returned error: %v", err)
	}

	tr := tar.NewReader(reader)
	header, err := tr.Next()
	if err != nil {
		t.Fatalf("failed to read tar header: %v", err)
	}
	if header.Name != "script.py" {
		t.Fatalf("expected script.py in archive, got %s", header.Name)
	}
	if header.Mode != 0o600 {
		t.Fatalf("expected mode 0600, got %o", header.Mode)
	}

	contents, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("failed to read tar contents: %v", err)
	}
	if !bytes.Equal(contents, data) {
		t.Fatalf("archive contents mismatch")
	}

	if _, err := tr.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF after single file, got %v", err)
	}
}

func TestCopyFileFromContainer(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	runner := &Runner{cli: client}
	containerID := "container-1"
	targetPath := "/tmp/program"

	tarData := newTarArchive(t, "program", []byte("binary"))
	client.setCopyFrom(containerID, targetPath, tarData)

	data, err := runner.copyFileFromContainer(context.Background(), containerID, targetPath)
	if err != nil {
		t.Fatalf("copyFileFromContainer returned error: %v", err)
	}
	if string(data) != "binary" {
		t.Fatalf("unexpected file contents: %q", data)
	}

	client.clearCopyFrom(containerID, targetPath)
	if _, err := runner.copyFileFromContainer(context.Background(), containerID, targetPath); err == nil {
		t.Fatalf("expected error when file missing")
	}
}

func TestGoStrategyPrepareBuildFailure(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	runner := mustNewRunner(t, client)
	runtime := runner.langs[execution.LanguageGo]

	client.createHooks = append(client.createHooks, func(id string) {
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 2}})
		client.setInspect(id, types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{},
			},
		})
		client.setLogs(id, encodeDockerLogs("build output", "build error"))
	})

	script := execution.Script{ID: "build-fail", Language: execution.LanguageGo, Source: "package main\nfunc main(){}"}
	prepared, result, err := runtime.strategy.Prepare(context.Background(), runner, runtime, script)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if prepared != nil {
		t.Fatalf("expected no prepared script on build failure")
	}
	if result == nil {
		t.Fatalf("expected build result")
	}
	if result.Status != execution.StatusBuildFail {
		t.Fatalf("expected status BuildFail, got %q", result.Status)
	}
	if result.ExitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", result.ExitCode)
	}
	if result.Stderr != "build error" {
		t.Fatalf("unexpected stderr: %q", result.Stderr)
	}
}

func TestGoStrategyPrepareSuccess(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	runner := mustNewRunner(t, client)
	runtime := runner.langs[execution.LanguageGo]

	binary := []byte("compiled-binary")
	client.createHooks = append(client.createHooks, func(id string) {
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 0}})
		client.setInspect(id, types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{OOMKilled: false},
			},
		})
		client.setLogs(id, encodeDockerLogs("", ""))
		binaryPath := path.Join(runtime.config.Workdir, goBinaryFilename)
		client.setCopyFrom(id, binaryPath, newTarArchive(t, goBinaryFilename, binary))
	})

	script := execution.Script{ID: "build-ok", Language: execution.LanguageGo, Source: "package main\nfunc main(){}"}
	prepared, result, err := runtime.strategy.Prepare(context.Background(), runner, runtime, script)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected no build result for successful prepare")
	}
	if prepared == nil {
		t.Fatalf("expected prepared script")
	}

	goPrep, ok := prepared.(*goPreparedScript)
	if !ok {
		t.Fatalf("expected goPreparedScript type")
	}
	if !bytes.Equal(goPrep.binary, binary) {
		t.Fatalf("binary contents mismatch")
	}
}

func TestRunProgramHandlesTimeLimit(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	runner := &Runner{
		cli:    client,
		limits: normalizeLimits(execution.RunLimits{}),
	}

	client.createHooks = append(client.createHooks, func(id string) {
		client.setWaitSequence(id,
			waitCall{block: true},
			waitCall{status: &container.WaitResponse{StatusCode: 137}},
		)
		client.setLogs(id, encodeDockerLogs("partial output", "time limit"))
	})

	runtime := &languageRuntime{config: LanguageConfig{Image: "python", Workdir: "/tmp"}}
	limits := execution.RunLimits{TimeLimit: 20 * time.Millisecond}
	files := []fileSpec{{Name: "program.py", Data: []byte("print('hi')")}}

	result, err := runner.runProgram(context.Background(), runtime, limits, []string{"python", "script.py"}, files, "", false)
	if err != nil {
		t.Fatalf("runProgram returned error: %v", err)
	}
	if result.Status != execution.StatusTimeLimit {
		t.Fatalf("expected time limit status, got %q", result.Status)
	}
	if result.ExitCode != 137 {
		t.Fatalf("expected exit code 137, got %d", result.ExitCode)
	}
	if result.Stdout != "partial output" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if result.Stderr != "time limit" {
		t.Fatalf("unexpected stderr: %q", result.Stderr)
	}
	if result.Duration <= 0 {
		t.Fatalf("expected positive duration, got %v", result.Duration)
	}

	if len(client.stopCalls) != 1 {
		t.Fatalf("expected container stop to be invoked, got %d", len(client.stopCalls))
	}
}

func TestRunProgramSuccessWithStdin(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	runner := &Runner{
		cli:    client,
		limits: normalizeLimits(execution.RunLimits{}),
	}

	runtime := &languageRuntime{config: LanguageConfig{Image: "python", Workdir: "/tmp"}}
	stdin := "42\n"
	attachConn := &fakeConn{}

	client.createHooks = append(client.createHooks, func(id string) {
		client.setAttachResponse(id, types.HijackedResponse{Conn: attachConn})
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 0}})
		client.setInspect(id, types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{},
			},
		})
		client.setLogs(id, encodeDockerLogs("answer", ""))
	})

	files := []fileSpec{{
		Name: "script.py",
		Data: []byte("print(input())"),
	}}

	result, err := runner.runProgram(context.Background(), runtime, execution.RunLimits{}, []string{"python", "script.py"}, files, stdin, true)
	if err != nil {
		t.Fatalf("runProgram returned error: %v", err)
	}
	if result.Status != execution.StatusOK {
		t.Fatalf("expected OK status, got %q", result.Status)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Stdout != "answer" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if attachConn.String() != stdin {
		t.Fatalf("expected stdin to be written, got %q", attachConn.String())
	}
	if !attachConn.closed {
		t.Fatalf("expected attach connection to be closed")
	}
}

func TestRunnerCloseInvokesClientClose(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	runner := &Runner{cli: client}

	if err := runner.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !client.wasClosed() {
		t.Fatalf("expected client Close to be called")
	}
}

func TestRunnerPrepareUnsupportedLanguage(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	runner := mustNewRunner(t, client)

	_, _, err := runner.Prepare(context.Background(), execution.Script{
		ID:       "unknown",
		Language: execution.Language("ruby"),
		Source:   "puts 'hi'",
	})
	if err == nil {
		t.Fatalf("expected error for unsupported language")
	}
}

func TestEnsureImagePullOnce(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	runner := &Runner{cli: client}
	runtime := &languageRuntime{config: LanguageConfig{Image: "python:3.12"}}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runner.ensureImage(context.Background(), runtime); err != nil {
				t.Errorf("ensureImage error: %v", err)
			}
		}()
	}
	wg.Wait()

	if pulls := client.imagePullRefs(); len(pulls) != 1 {
		t.Fatalf("expected one image pull, got %d", len(pulls))
	}
	if pulls := client.imagePullRefs(); pulls[0] != "python:3.12" {
		t.Fatalf("unexpected image ref %q", pulls[0])
	}
}

func TestRunnerPreparePythonAndRun(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	runner := mustNewRunner(t, client)

	script := execution.Script{
		ID:       "py-script",
		Language: execution.LanguagePython,
		Source:   "print('hello')",
	}

	prepared, buildResult, err := runner.Prepare(context.Background(), script)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if buildResult != nil {
		t.Fatalf("expected no build result for python prepare")
	}

	attachConn := &fakeConn{}
	client.createHooks = append(client.createHooks, func(id string) {
		client.setAttachResponse(id, types.HijackedResponse{Conn: attachConn})
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 0}})
		client.setInspect(id, types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{},
			},
		})
		client.setLogs(id, encodeDockerLogs("hello\n", ""))
	})

	result, err := prepared.Run(context.Background(), "input\n")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != execution.StatusOK {
		t.Fatalf("expected OK status, got %q", result.Status)
	}
	if result.Stdout != "hello\n" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if attachConn.String() != "input\n" {
		t.Fatalf("expected stdin to be written, got %q", attachConn.String())
	}
	if !attachConn.closed {
		t.Fatalf("expected attach connection to close")
	}
	if err := prepared.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestGoPreparedScriptRun(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	runner := mustNewRunner(t, client)

	attachConn := &fakeConn{}

	goPrep := &goPreparedScript{
		runner:  runner,
		runtime: runner.langs[execution.LanguageGo],
		binary:  []byte("compiled"),
		limits:  execution.RunLimits{},
	}

	client.createHooks = append(client.createHooks, func(id string) {
		client.setAttachResponse(id, types.HijackedResponse{Conn: attachConn})
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 0}})
		client.setInspect(id, types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{},
			},
		})
		client.setLogs(id, encodeDockerLogs("ok", ""))
	})

	result, err := goPrep.Run(context.Background(), "")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != execution.StatusOK {
		t.Fatalf("expected OK status, got %q", result.Status)
	}
	if result.Stdout != "ok" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}

	if count := client.copyToCount(); count == 0 {
		t.Fatalf("expected binary to be copied to container")
	}
	if call, ok := client.lastCopyTo(); ok {
		if !bytes.Contains(call.data, []byte("compiled")) {
			t.Fatalf("expected binary data to be copied")
		}
	}
	if !attachConn.closed {
		t.Fatalf("expected attach connection to close")
	}

	if err := goPrep.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestRunnerCloseNilClient(t *testing.T) {
	t.Parallel()

	var runner Runner
	if err := runner.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestCopyFilesNoop(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	runner := &Runner{cli: client}

	if err := runner.copyFiles(context.Background(), "id", "/tmp", nil); err != nil {
		t.Fatalf("copyFiles returned error: %v", err)
	}
	if client.copyToCount() != 0 {
		t.Fatalf("expected no copy operations for empty file list")
	}
}

type fakeDockerClient struct {
	mu           sync.Mutex
	nextID       int
	closed       bool
	imagePulls   []string
	createCalls  []containerCreateCall
	copyToCalls  []copyToCall
	waitCalls    map[string][]waitCall
	logs         map[string][]byte
	inspect      map[string]types.ContainerJSON
	copyFromData map[string][]byte
	removeCalls  []string
	stopCalls    []string
	attach       map[string]types.HijackedResponse
	createHooks  []func(string)
}

type containerCreateCall struct {
	id         string
	config     *container.Config
	hostConfig *container.HostConfig
}

type copyToCall struct {
	containerID string
	path        string
	data        []byte
}

type waitCall struct {
	status *container.WaitResponse
	err    error
	block  bool
}

func newFakeDockerClient() *fakeDockerClient {
	return &fakeDockerClient{
		waitCalls:    make(map[string][]waitCall),
		logs:         make(map[string][]byte),
		inspect:      make(map[string]types.ContainerJSON),
		copyFromData: make(map[string][]byte),
		attach:       make(map[string]types.HijackedResponse),
	}
}

func (f *fakeDockerClient) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

func (f *fakeDockerClient) ImagePull(ctx context.Context, ref string, opts image.PullOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	f.imagePulls = append(f.imagePulls, ref)
	f.mu.Unlock()
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (f *fakeDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
	f.mu.Lock()
	id := fmt.Sprintf("container-%d", f.nextID)
	f.nextID++
	f.createCalls = append(f.createCalls, containerCreateCall{id: id, config: config, hostConfig: hostConfig})
	hook := popHook(&f.createHooks)
	f.mu.Unlock()

	if hook != nil {
		hook(id)
	}

	return container.CreateResponse{ID: id}, nil
}

func (f *fakeDockerClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	f.mu.Lock()
	f.removeCalls = append(f.removeCalls, containerID)
	f.mu.Unlock()
	return nil
}

func (f *fakeDockerClient) CopyToContainer(ctx context.Context, containerID, dstPath string, content io.Reader, options container.CopyToContainerOptions) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.copyToCalls = append(f.copyToCalls, copyToCall{containerID: containerID, path: dstPath, data: data})
	f.mu.Unlock()
	return nil
}

func (f *fakeDockerClient) ContainerAttach(ctx context.Context, containerID string, opts container.AttachOptions) (types.HijackedResponse, error) {
	f.mu.Lock()
	resp, ok := f.attach[containerID]
	f.mu.Unlock()
	if !ok {
		return types.HijackedResponse{}, nil
	}
	return resp, nil
}

func (f *fakeDockerClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	return nil
}

func (f *fakeDockerClient) ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	statusCh := make(chan container.WaitResponse, 1)
	errCh := make(chan error, 1)

	f.mu.Lock()
	calls := f.waitCalls[containerID]
	if len(calls) > 0 {
		call := calls[0]
		f.waitCalls[containerID] = calls[1:]
		f.mu.Unlock()

		if call.block {
			return statusCh, errCh
		}
		if call.status != nil {
			statusCh <- *call.status
		}
		if call.err != nil {
			errCh <- call.err
		}
		return statusCh, errCh
	}
	f.mu.Unlock()

	return statusCh, errCh
}

func (f *fakeDockerClient) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if info, ok := f.inspect[containerID]; ok {
		return info, nil
	}
	return types.ContainerJSON{}, nil
}

func (f *fakeDockerClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	data := f.logs[containerID]
	f.mu.Unlock()
	if data == nil {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeDockerClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	f.mu.Lock()
	f.stopCalls = append(f.stopCalls, containerID)
	f.mu.Unlock()
	return nil
}

func (f *fakeDockerClient) CopyFromContainer(ctx context.Context, containerID, srcPath string) (io.ReadCloser, types.ContainerPathStat, error) {
	key := copyKey(containerID, srcPath)
	f.mu.Lock()
	data, ok := f.copyFromData[key]
	f.mu.Unlock()
	if !ok {
		return nil, types.ContainerPathStat{}, fmt.Errorf("not found")
	}
	return io.NopCloser(bytes.NewReader(data)), types.ContainerPathStat{}, nil
}

func (f *fakeDockerClient) setWaitSequence(containerID string, calls ...waitCall) {
	f.mu.Lock()
	f.waitCalls[containerID] = append([]waitCall{}, calls...)
	f.mu.Unlock()
}

func (f *fakeDockerClient) setLogs(containerID string, data []byte) {
	f.mu.Lock()
	f.logs[containerID] = data
	f.mu.Unlock()
}

func (f *fakeDockerClient) setInspect(containerID string, info types.ContainerJSON) {
	f.mu.Lock()
	f.inspect[containerID] = info
	f.mu.Unlock()
}

func (f *fakeDockerClient) setCopyFrom(containerID, srcPath string, data []byte) {
	f.mu.Lock()
	f.copyFromData[copyKey(containerID, srcPath)] = data
	f.mu.Unlock()
}

func (f *fakeDockerClient) clearCopyFrom(containerID, srcPath string) {
	f.mu.Lock()
	delete(f.copyFromData, copyKey(containerID, srcPath))
	f.mu.Unlock()
}

func (f *fakeDockerClient) setAttachResponse(containerID string, resp types.HijackedResponse) {
	f.mu.Lock()
	f.attach[containerID] = resp
	f.mu.Unlock()
}

func (f *fakeDockerClient) imagePullRefs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(f.imagePulls))
	copy(cp, f.imagePulls)
	return cp
}

func (f *fakeDockerClient) wasClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *fakeDockerClient) copyToCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.copyToCalls)
}

func (f *fakeDockerClient) lastCopyTo() (copyToCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.copyToCalls) == 0 {
		return copyToCall{}, false
	}
	return f.copyToCalls[len(f.copyToCalls)-1], true
}

func copyKey(containerID, srcPath string) string {
	return containerID + "|" + srcPath
}

func popHook(hooks *[]func(string)) func(string) {
	if len(*hooks) == 0 {
		return nil
	}
	hook := (*hooks)[0]
	*hooks = (*hooks)[1:]
	return hook
}

func mustNewRunner(t *testing.T, client *fakeDockerClient) *Runner {
	t.Helper()

	cfg := Config{
		Languages: map[execution.Language]LanguageConfig{
			execution.LanguagePython: {Image: "python", Workdir: "/tmp"},
			execution.LanguageGo:     {Image: "golang", Workdir: "/tmp"},
		},
		DefaultLimits: execution.RunLimits{TimeLimit: 5 * time.Second, MemoryLimitBytes: 256 * 1024 * 1024},
	}

	runner, err := newRunner(client, cfg)
	if err != nil {
		t.Fatalf("failed to create runner: %v", err)
	}
	return runner
}

func encodeDockerLogs(stdout, stderr string) []byte {
	var buf bytes.Buffer
	if stdout != "" {
		w := stdcopy.NewStdWriter(&buf, stdcopy.Stdout)
		_, _ = w.Write([]byte(stdout))
	}
	if stderr != "" {
		w := stdcopy.NewStdWriter(&buf, stdcopy.Stderr)
		_, _ = w.Write([]byte(stderr))
	}
	return buf.Bytes()
}

func newTarArchive(t *testing.T, name string, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("write tar data: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	return buf.Bytes()
}

type fakeConn struct {
	bytes.Buffer
	closed bool
}

func (c *fakeConn) Read(b []byte) (int, error) {
	return c.Buffer.Read(b)
}

func (c *fakeConn) Write(b []byte) (int, error) {
	return c.Buffer.Write(b)
}

func (c *fakeConn) Close() error {
	c.closed = true
	return nil
}

func (c *fakeConn) CloseWrite() error {
	return c.Close()
}

func (c *fakeConn) LocalAddr() net.Addr {
	return fakeAddr("local")
}

func (c *fakeConn) RemoteAddr() net.Addr {
	return fakeAddr("remote")
}

func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

type fakeAddr string

func (a fakeAddr) Network() string { return string(a) }
func (a fakeAddr) String() string  { return string(a) }
