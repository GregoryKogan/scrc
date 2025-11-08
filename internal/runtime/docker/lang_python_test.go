package docker

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"

	"scrc/internal/domain/execution"
)

func TestPythonStrategyPrepareAndRun(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		language: execution.LanguagePython,
		config:   LanguageConfig{Image: "python", Workdir: "/workspace"},
		engine:   engine,
	}

	client.onCreate(func(id string) {
		client.setAttachResponse(id, types.HijackedResponse{Conn: &fakeConn{}})
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 0}})
		client.setInspect(id, types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{},
			},
		})
		client.setLogs(id, "hello", "")
	})

	strategy := &pythonStrategy{}
	script := execution.Script{
		ID:       "python",
		Language: execution.LanguagePython,
		Source:   "print('hello')",
	}

	prepared, result, err := strategy.Prepare(context.Background(), runtime, script)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil build result for python")
	}

	runResult, runErr := prepared.Run(context.Background(), "")
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}
	if runResult.Stdout != "hello" {
		t.Fatalf("unexpected stdout: %q", runResult.Stdout)
	}
}
