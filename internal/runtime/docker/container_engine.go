package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	typesimage "github.com/docker/docker/api/types/image"

	"scrc/internal/domain/execution"
)

type containerEngine struct {
	cli           dockerClient
	defaultLimits execution.RunLimits
}

func newContainerEngine(cli dockerClient, defaultLimits execution.RunLimits) *containerEngine {
	return &containerEngine{
		cli:           cli,
		defaultLimits: normalizeLimits(defaultLimits),
	}
}

func (c *containerEngine) pullImage(ctx context.Context, ref string) error {
	reader, err := c.cli.ImagePull(ctx, ref, typesimage.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", ref, err)
	}
	defer reader.Close()
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("consume pull output for %s: %w", ref, err)
	}
	return nil
}

func (c *containerEngine) effectiveLimits(request execution.RunLimits) execution.RunLimits {
	effective := c.defaultLimits
	overrides := normalizeLimits(request)

	if overrides.TimeLimit > 0 {
		effective.TimeLimit = overrides.TimeLimit
	}
	if overrides.MemoryLimitBytes > 0 {
		effective.MemoryLimitBytes = overrides.MemoryLimitBytes
	}

	return effective
}

func (c *containerEngine) runProgram(
	ctx context.Context,
	runtime *languageRuntime,
	limits execution.RunLimits,
	command []string,
	files []fileSpec,
	stdin string,
	attachStdin bool,
) (*execution.Result, error) {
	effectiveLimits := c.effectiveLimits(limits)

	containerID, cleanup, err := c.createContainer(ctx, runtime, effectiveLimits, command, attachStdin)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if err := c.copyFiles(ctx, containerID, runtime.config.Workdir, files); err != nil {
		return nil, fmt.Errorf("copy files: %w", err)
	}

	var attach types.HijackedResponse
	if attachStdin {
		attachCtx := ctx
		if attachCtx.Err() != nil {
			attachCtx = context.Background()
		}
		attach, err = c.cli.ContainerAttach(attachCtx, containerID, container.AttachOptions{
			Stream: true,
			Stdin:  true,
		})
		if err != nil {
			return nil, fmt.Errorf("attach container: %w", err)
		}
		defer attach.Close()
	}

	start := time.Now()
	if err := c.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
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
	status, err := c.waitForExit(waitCtx, containerID)
	if cancel != nil {
		cancel()
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && effectiveLimits.TimeLimit > 0 && ctx.Err() == nil {
			return c.handleTimeLimit(containerID, start)
		}
		return nil, err
	}

	inspectCtx := ctx
	if inspectCtx.Err() != nil {
		inspectCtx = context.Background()
	}

	inspect, err := c.cli.ContainerInspect(inspectCtx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}

	logCtx := ctx
	if logCtx.Err() != nil {
		logCtx = context.Background()
	}

	stdout, stderr, err := c.fetchLogs(logCtx, containerID)
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

func (c *containerEngine) createContainer(ctx context.Context, runtime *languageRuntime, limits execution.RunLimits, cmd []string, attachStdin bool) (string, func(), error) {
	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			NanoCPUs: 1_000_000_000,
		},
	}
	if limits.MemoryLimitBytes > 0 {
		hostConfig.Resources.Memory = limits.MemoryLimitBytes
		hostConfig.Resources.MemorySwap = limits.MemoryLimitBytes
	}

	resp, err := c.cli.ContainerCreate(
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
		_ = c.cli.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
	}

	return resp.ID, cleanup, nil
}
