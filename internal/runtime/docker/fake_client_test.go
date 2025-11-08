package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

type fakeDockerClient struct {
	mu           sync.Mutex
	nextID       int
	imagePulls   []string
	createCalls  []containerCreateCall
	copyToCalls  []copyToCall
	waitCalls    map[string][]waitCall
	logs         map[string][]byte
	inspect      map[string]types.ContainerJSON
	copyFromData map[string][]byte
	stopCalls    []string
	attach       map[string]types.HijackedResponse
	createHooks  []func(string)
	closed       bool
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
	return nil
}

func (f *fakeDockerClient) CopyToContainer(ctx context.Context, containerID, dstPath string, content io.Reader, options types.CopyToContainerOptions) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.copyToCalls = append(f.copyToCalls, copyToCall{containerID: containerID, path: dstPath, data: data})
	f.mu.Unlock()
	return nil
}

func (f *fakeDockerClient) ContainerAttach(ctx context.Context, containerID string, options container.AttachOptions) (types.HijackedResponse, error) {
	f.mu.Lock()
	resp := f.attach[containerID]
	f.mu.Unlock()
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
	return f.inspect[containerID], nil
}

func (f *fakeDockerClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	data := f.logs[containerID]
	f.mu.Unlock()
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

func (f *fakeDockerClient) setLogs(containerID string, stdout, stderr string) {
	var buf bytes.Buffer
	if stdout != "" {
		w := stdcopy.NewStdWriter(&buf, stdcopy.Stdout)
		_, _ = w.Write([]byte(stdout))
	}
	if stderr != "" {
		w := stdcopy.NewStdWriter(&buf, stdcopy.Stderr)
		_, _ = w.Write([]byte(stderr))
	}
	f.mu.Lock()
	f.logs[containerID] = buf.Bytes()
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

func (f *fakeDockerClient) setAttachResponse(containerID string, resp types.HijackedResponse) {
	f.mu.Lock()
	f.attach[containerID] = resp
	f.mu.Unlock()
}

func (f *fakeDockerClient) onCreate(hook func(string)) {
	f.mu.Lock()
	f.createHooks = append(f.createHooks, hook)
	f.mu.Unlock()
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

type fakeConn struct {
	bytes.Buffer
	closed bool
}

func (c *fakeConn) Close() error {
	c.closed = true
	return nil
}

func (c *fakeConn) CloseWrite() error {
	return c.Close()
}

func (c *fakeConn) LocalAddr() net.Addr              { return fakeAddr("local") }
func (c *fakeConn) RemoteAddr() net.Addr             { return fakeAddr("remote") }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

type fakeAddr string

func (a fakeAddr) Network() string { return string(a) }
func (a fakeAddr) String() string  { return string(a) }
