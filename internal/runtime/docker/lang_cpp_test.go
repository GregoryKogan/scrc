package docker

import (
	"context"
	"errors"
	"path"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"

	"scrc/internal/domain/execution"
)

func TestCPPStrategyPrepareBuildFailure(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		language: execution.LanguageCPP,
		config:   LanguageConfig{Image: "gcc", Workdir: "/workspace"},
		engine:   engine,
	}

	client.onCreate(func(id string) {
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 1}})
		client.setLogs(id, "", "compile error")
		client.setInspect(id, types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{State: &types.ContainerState{}}})
	})

	strategy := &cppStrategy{}
	script := execution.Script{
		ID:       "build-fail",
		Language: execution.LanguageCPP,
		Source:   "int main(){return 0;}",
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
	if result.Stderr != "compile error" {
		t.Fatalf("expected stderr %q, got %q", "compile error", result.Stderr)
	}
}

func TestCPPStrategyPrepareSuccess(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		language: execution.LanguageCPP,
		config:   LanguageConfig{Image: "gcc", Workdir: "/workspace"},
		engine:   engine,
	}

	binary := []byte("compiled-binary")
	client.onCreate(func(id string) {
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 0}})
		client.setInspect(id, types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{State: &types.ContainerState{}}})
		client.setLogs(id, "", "")
		client.setCopyFrom(id, path.Join(runtime.config.Workdir, cppBinaryFilename), newTarArchive(t, cppBinaryFilename, binary))
	})

	strategy := &cppStrategy{}
	script := execution.Script{
		ID:       "build-ok",
		Language: execution.LanguageCPP,
		Source:   "#include <iostream>\nint main(){std::cout<<\"hi\";}\n",
	}

	prepared, result, err := strategy.Prepare(context.Background(), runtime, script)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil build result on success")
	}
	prep, ok := prepared.(*cppPreparedScript)
	if !ok {
		t.Fatalf("expected cppPreparedScript type")
	}
	if string(prep.binary) != string(binary) {
		t.Fatalf("unexpected binary contents")
	}
}

func TestCPPPreparedScriptRunPropagatesError(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		language: execution.LanguageCPP,
		config:   LanguageConfig{Image: "gcc", Workdir: "/workspace"},
		engine:   engine,
	}

	client.onCreate(func(id string) {
		client.setAttachResponse(id, types.HijackedResponse{Conn: &fakeConn{}})
		client.setWaitSequence(id, waitCall{err: errors.New("wait failure")})
		client.setInspect(id, types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{State: &types.ContainerState{}},
		})
	})

	prepared := &cppPreparedScript{
		runtime: runtime,
		binary:  []byte("compiled"),
		limits:  executionLimits(0, 0),
	}

	_, err := prepared.Run(context.Background(), "")
	if err == nil {
		t.Fatalf("expected run error")
	}
}
