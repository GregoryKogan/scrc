package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"path"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"

	"scrc/internal/domain/execution"
)

func TestGoStrategyPrepareBuildFailure(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		language: execution.LanguageGo,
		config:   LanguageConfig{Image: "golang", Workdir: "/workspace"},
		engine:   engine,
	}

	client.onCreate(func(id string) {
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 2}})
		client.setLogs(id, "", "build error")
		client.setInspect(id, types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{State: &types.ContainerState{}}})
	})

	strategy := &goStrategy{}
	script := execution.Script{
		ID:       "build-fail",
		Language: execution.LanguageGo,
		Source:   "package main\nfunc main(){}\n",
	}

	prepared, result, err := strategy.Prepare(context.Background(), runtime, script)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if prepared != nil {
		t.Fatalf("expected nil prepared script on build failure")
	}
	if result == nil {
		t.Fatalf("expected build result")
	}
	if result.Status != execution.StatusBuildFail {
		t.Fatalf("expected status BuildFail, got %q", result.Status)
	}
	if result.Stderr != "build error" {
		t.Fatalf("expected stderr %q, got %q", "build error", result.Stderr)
	}
}

func TestGoStrategyPrepareSuccess(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		language: execution.LanguageGo,
		config:   LanguageConfig{Image: "golang", Workdir: "/workspace"},
		engine:   engine,
	}

	binary := []byte("compiled-binary")
	client.onCreate(func(id string) {
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 0}})
		client.setInspect(id, types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{State: &types.ContainerState{}}})
		client.setLogs(id, "", "")
		client.setCopyFrom(id, path.Join(runtime.config.Workdir, goBinaryFilename), newTarArchive(t, goBinaryFilename, binary))
	})

	strategy := &goStrategy{}
	script := execution.Script{
		ID:       "build-ok",
		Language: execution.LanguageGo,
		Source:   "package main\nfunc main(){}\n",
	}

	prepared, result, err := strategy.Prepare(context.Background(), runtime, script)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil build result on success")
	}
	prep, ok := prepared.(*goPreparedScript)
	if !ok {
		t.Fatalf("expected goPreparedScript type")
	}
	if string(prep.binary) != string(binary) {
		t.Fatalf("unexpected binary contents")
	}
}

func TestGoPreparedScriptRunPropagatesError(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		language: execution.LanguageGo,
		config:   LanguageConfig{Image: "golang", Workdir: "/workspace"},
		engine:   engine,
	}

	client.onCreate(func(id string) {
		client.setAttachResponse(id, types.HijackedResponse{Conn: &fakeConn{}})
		client.setWaitSequence(id, waitCall{err: errors.New("wait failure")})
		client.setInspect(id, types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{State: &types.ContainerState{}},
		})
	})

	prepared := &goPreparedScript{
		runtime: runtime,
		binary:  []byte("compiled"),
		limits:  executionLimits(0, 0),
	}

	_, err := prepared.Run(context.Background(), "")
	if err == nil {
		t.Fatalf("expected run error")
	}
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
		t.Fatalf("write tar contents: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	return buf.Bytes()
}
