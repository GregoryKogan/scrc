package docker

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"

	"scrc/internal/domain/execution"
)

func TestNormalizeLimitsClampsNegative(t *testing.T) {
	t.Parallel()

	limits := normalizeLimits(executionLimits(-5*time.Second, -10))
	if limits.TimeLimit != 0 {
		t.Fatalf("expected zero time limit, got %v", limits.TimeLimit)
	}
	if limits.MemoryLimitBytes != 0 {
		t.Fatalf("expected zero memory limit, got %d", limits.MemoryLimitBytes)
	}
}

func TestContainerEngineEffectiveLimitsMergesOverrides(t *testing.T) {
	t.Parallel()

	engine := newContainerEngine(nil, executionLimits(5*time.Second, 1024))
	got := engine.effectiveLimits(executionLimits(2*time.Second, 0))

	if got.TimeLimit != 2*time.Second {
		t.Fatalf("expected time limit 2s, got %v", got.TimeLimit)
	}
	if got.MemoryLimitBytes != 1024 {
		t.Fatalf("expected memory limit 1024, got %d", got.MemoryLimitBytes)
	}
}

func TestRunProgramHandlesTimeLimit(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		config: LanguageConfig{Image: "python", Workdir: "/tmp"},
		engine: engine,
	}

	client.onCreate(func(id string) {
		client.setWaitSequence(id,
			waitCall{block: true},
			waitCall{status: &container.WaitResponse{StatusCode: 137}},
		)
		client.setLogs(id, "partial", "timeout")
	})

	result, err := engine.runProgram(
		context.Background(),
		runtime,
		executionLimits(10*time.Millisecond, 0),
		[]string{"python", "script.py"},
		[]fileSpec{{Name: "script.py", Data: []byte("print('hi')")}},
		"",
		false,
	)
	if err != nil {
		t.Fatalf("runProgram returned error: %v", err)
	}
	if result.Status != execution.StatusTimeLimit {
		t.Fatalf("expected status TimeLimit, got %q", result.Status)
	}
	if result.ExitCode != 137 {
		t.Fatalf("expected exit code 137, got %d", result.ExitCode)
	}
	if len(client.stopCalls) != 1 {
		t.Fatalf("expected ContainerStop to be invoked once, got %d", len(client.stopCalls))
	}
}

func TestRunProgramSuccessWithStdin(t *testing.T) {
	t.Parallel()

	client := newFakeDockerClient()
	engine := newContainerEngine(client, executionLimits(0, 0))
	runtime := &languageRuntime{
		config: LanguageConfig{Image: "python", Workdir: "/tmp"},
		engine: engine,
	}

	attachConn := &fakeConn{}
	client.onCreate(func(id string) {
		client.setAttachResponse(id, types.HijackedResponse{Conn: attachConn})
		client.setWaitSequence(id, waitCall{status: &container.WaitResponse{StatusCode: 0}})
		client.setInspect(id, types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{},
			},
		})
		client.setLogs(id, "answer", "")
	})

	stdin := "42\n"
	result, err := engine.runProgram(
		context.Background(),
		runtime,
		executionLimits(0, 0),
		[]string{"python", "script.py"},
		[]fileSpec{{Name: "script.py", Data: []byte("print(input())")}},
		stdin,
		true,
	)
	if err != nil {
		t.Fatalf("runProgram returned error: %v", err)
	}
	if result.Status != execution.StatusOK {
		t.Fatalf("expected OK status, got %q", result.Status)
	}
	if attachConn.String() != stdin {
		t.Fatalf("expected stdin to be forwarded, got %q", attachConn.String())
	}
	if !attachConn.closed {
		t.Fatalf("expected connection to be closed")
	}
	if len(client.copyToCalls) == 0 {
		t.Fatalf("expected file contents to be copied to container")
	}
}

func executionLimits(duration time.Duration, memory int64) execution.RunLimits {
	return execution.RunLimits{
		TimeLimit:        duration,
		MemoryLimitBytes: memory,
	}
}
