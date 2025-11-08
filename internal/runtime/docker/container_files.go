package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"scrc/internal/domain/execution"
)

type fileSpec struct {
	Name string
	Mode int64
	Data []byte
}

func (c *containerEngine) copyFiles(ctx context.Context, containerID, workdir string, files []fileSpec) error {
	if len(files) == 0 {
		return nil
	}

	reader, err := makeArchive(files)
	if err != nil {
		return err
	}

	return c.cli.CopyToContainer(ctx, containerID, workdir, reader, types.CopyToContainerOptions{AllowOverwriteDirWithFile: true})
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

func (c *containerEngine) copyFileFromContainer(ctx context.Context, containerID, sourcePath string) ([]byte, error) {
	reader, _, err := c.cli.CopyFromContainer(ctx, containerID, sourcePath)
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

func (c *containerEngine) handleTimeLimit(containerID string, start time.Time) (*execution.Result, error) {
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStop()

	if err := c.cli.ContainerStop(stopCtx, containerID, container.StopOptions{}); err != nil && !client.IsErrNotFound(err) {
		return nil, fmt.Errorf("stop container after time limit: %w", err)
	}

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelWait()

	status, waitErr := c.waitForExit(waitCtx, containerID)
	if waitErr != nil && !errors.Is(waitErr, context.DeadlineExceeded) && !client.IsErrNotFound(waitErr) {
		return nil, fmt.Errorf("wait for container after time limit: %w", waitErr)
	}

	stdout, stderr, err := c.fetchLogs(context.Background(), containerID)
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

func (c *containerEngine) waitForExit(ctx context.Context, containerID string) (*container.WaitResponse, error) {
	statusCh, errCh := c.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
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

func (c *containerEngine) fetchLogs(ctx context.Context, containerID string) (stdout, stderr string, err error) {
	logs, err := c.cli.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
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
