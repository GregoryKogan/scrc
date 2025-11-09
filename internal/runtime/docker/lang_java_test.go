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

func TestJavaStrategyPrepareBuildFailure(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		language: execution.LanguageJava,
		config:   LanguageConfig{Image: "openjdk", Workdir: "/workspace"},
		engine:   engine,
	}

	client.onCreate(func(id string) {
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 1}})
		client.setLogs(id, "", "compile error")
		client.setInspect(id, types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{State: &types.ContainerState{}}})
	})

	strategy := &javaStrategy{}
	script := execution.Script{
		ID:       "build-fail",
		Language: execution.LanguageJava,
		Source:   "class Main { public static void main(String[] args) {}",
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

func TestJavaStrategyPrepareSuccess(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		language: execution.LanguageJava,
		config:   LanguageConfig{Image: "openjdk", Workdir: "/workspace"},
		engine:   engine,
	}

	jar := []byte("compiled-jar")
	client.onCreate(func(id string) {
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 0}})
		client.setInspect(id, types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{State: &types.ContainerState{}}})
		client.setLogs(id, "", "")
		client.setCopyFrom(id, path.Join(runtime.config.Workdir, javaBinaryFilename), newTarArchive(t, javaBinaryFilename, jar))
	})

	strategy := &javaStrategy{}
	script := execution.Script{
		ID:       "build-ok",
		Language: execution.LanguageJava,
		Source:   "class Main { public static void main(String[] args) { System.out.println(\"hi\"); }}",
	}

	prepared, result, err := strategy.Prepare(context.Background(), runtime, script)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil build result on success")
	}
	prep, ok := prepared.(*javaPreparedScript)
	if !ok {
		t.Fatalf("expected javaPreparedScript type")
	}
	if string(prep.binary) != string(jar) {
		t.Fatalf("unexpected binary contents")
	}
}

func TestJavaPreparedScriptRunPropagatesError(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		language: execution.LanguageJava,
		config:   LanguageConfig{Image: "openjdk", Workdir: "/workspace"},
		engine:   engine,
	}

	client.onCreate(func(id string) {
		client.setAttachResponse(id, types.HijackedResponse{Conn: &fakeConn{}})
		client.setWaitSequence(id, waitCall{err: errors.New("wait failure")})
		client.setInspect(id, types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{State: &types.ContainerState{}},
		})
	})

	prepared := &javaPreparedScript{
		runtime: runtime,
		binary:  []byte("compiled-jar"),
		limits:  executionLimits(0, 0),
	}

	_, err := prepared.Run(context.Background(), "")
	if err == nil {
		t.Fatalf("expected run error")
	}
}
