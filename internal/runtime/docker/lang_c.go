package docker

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/docker/docker/api/types/container"

	"scrc/internal/domain/execution"
	runtimex "scrc/internal/runtime"
)

type cStrategy struct{}

func (c *cStrategy) Prepare(ctx context.Context, lang *languageRuntime, script execution.Script) (runtimex.PreparedScript, *execution.Result, error) {
	runLimits := lang.engine.effectiveLimits(script.Limits)
	buildLimits := runLimits
	buildLimits.TimeLimit = 0

	containerID, cleanup, err := lang.engine.createContainer(ctx, lang, buildLimits, []string{"gcc", "-static", "-O2", "-pipe", "-o", cBinaryFilename, cSourceFilename}, false)
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()

	if err := lang.engine.copyFiles(ctx, containerID, lang.config.Workdir, []fileSpec{
		{
			Name: cSourceFilename,
			Mode: 0o644,
			Data: []byte(script.Source),
		},
	}); err != nil {
		return nil, nil, fmt.Errorf("copy source: %w", err)
	}

	start := time.Now()
	if err := lang.engine.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, nil, fmt.Errorf("start container: %w", err)
	}

	waitCtx := ctx
	var cancel context.CancelFunc
	if buildLimits.TimeLimit > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, buildLimits.TimeLimit)
	}
	status, err := lang.engine.waitForExit(waitCtx, containerID)
	if cancel != nil {
		cancel()
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && buildLimits.TimeLimit > 0 && ctx.Err() == nil {
			result, handleErr := lang.engine.handleTimeLimit(containerID, start)
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
	inspect, err := lang.engine.cli.ContainerInspect(inspectCtx, containerID)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect container: %w", err)
	}

	logCtx := ctx
	if logCtx.Err() != nil {
		logCtx = context.Background()
	}
	stdout, stderr, err := lang.engine.fetchLogs(logCtx, containerID)
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

	binaryPath := path.Join(lang.config.Workdir, cBinaryFilename)
	binaryData, err := lang.engine.copyFileFromContainer(ctx, containerID, binaryPath)
	if err != nil {
		return nil, nil, fmt.Errorf("extract compiled binary: %w", err)
	}

	return &cPreparedScript{
		runtime: lang,
		binary:  binaryData,
		limits:  runLimits,
	}, nil, nil
}

func (c *cStrategy) Close() error {
	return nil
}

type cPreparedScript struct {
	runtime *languageRuntime
	binary  []byte
	limits  execution.RunLimits
}

func (c *cPreparedScript) Run(ctx context.Context, stdin string) (*execution.Result, error) {
	return c.runtime.engine.runProgram(ctx, c.runtime, c.limits, []string{"./" + cBinaryFilename}, []fileSpec{
		{
			Name: cBinaryFilename,
			Mode: 0o755,
			Data: c.binary,
		},
	}, stdin, true)
}

func (c *cPreparedScript) Close() error {
	return nil
}
