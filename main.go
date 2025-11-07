package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	dockerImage       = "python:3.12-alpine"
	containerWorkdir  = "/tmp"
	scriptFilename    = "script.py"
	containerExecPath = containerWorkdir + "/" + scriptFilename
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	scriptPath, cleanup, err := createPythonScript()
	if err != nil {
		log.Fatalf("failed to create python script: %v", err)
	}
	defer cleanup()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("failed to create docker client: %v", err)
	}
	defer cli.Close()

	if err := pullImage(ctx, cli, dockerImage); err != nil {
		log.Fatalf("failed to pull image: %v", err)
	}

	resp, err := cli.ContainerCreate(
		ctx,
		&container.Config{
			Image:        dockerImage,
			Cmd:          []string{"python", containerExecPath},
			AttachStdout: true,
			AttachStderr: true,
		},
		nil,
		nil,
		nil,
		"",
	)
	if err != nil {
		log.Fatalf("failed to create container: %v", err)
	}
	defer func() {
		_ = cli.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
	}()

	if err := copyScriptToContainer(ctx, cli, resp.ID, scriptPath, containerWorkdir); err != nil {
		log.Fatalf("failed to copy script into container: %v", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		log.Fatalf("failed to start container: %v", err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case status := <-statusCh:
		if status.Error != nil {
			log.Fatalf("container exited with error: %s", status.Error.Message)
		}
		fmt.Printf("container exited with status %d\n", status.StatusCode)
	case err := <-errCh:
		if err != nil {
			log.Fatalf("error while waiting for container: %v", err)
		}
	}

	if err := printContainerLogs(ctx, cli, resp.ID); err != nil {
		log.Fatalf("failed to read container logs: %v", err)
	}
}

func createPythonScript() (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "docker-python-script-")
	if err != nil {
		return "", func() {}, err
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	scriptPath := filepath.Join(tmpDir, scriptFilename)
	scriptContent := []byte("print('Hello from Python inside Docker!')\n")

	if err := os.WriteFile(scriptPath, scriptContent, 0o644); err != nil {
		cleanup()
		return "", func() {}, err
	}

	return scriptPath, cleanup, nil
}

func pullImage(ctx context.Context, cli *client.Client, ref string) error {
	reader, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	_, err = io.Copy(io.Discard, reader)
	return err
}

func copyScriptToContainer(ctx context.Context, cli *client.Client, containerID, scriptPath, dstDir string) error {
	file, err := os.Open(scriptPath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	header := &tar.Header{
		Name:    filepath.Base(scriptPath),
		Mode:    0o644,
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	if _, err := io.Copy(tw, file); err != nil {
		return err
	}

	if err := tw.Close(); err != nil {
		return err
	}

	return cli.CopyToContainer(ctx, containerID, dstDir, bytes.NewReader(buf.Bytes()), container.CopyToContainerOptions{AllowOverwriteDirWithFile: true})
}

func printContainerLogs(ctx context.Context, cli *client.Client, containerID string) error {
	logs, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return err
	}
	defer logs.Close()

	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, logs)
	return err
}
