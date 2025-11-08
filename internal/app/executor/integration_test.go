//go:build integration

package executor_test

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"scrc/internal/app/executor"
	"scrc/internal/domain/execution"
	"scrc/internal/infra/docker"
)

func TestServiceExecutesScriptsAgainstDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping docker integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	runner, err := docker.New(docker.Config{
		Languages: map[execution.Language]docker.LanguageConfig{
			execution.LanguagePython: {
				Image:   "python:3.12-alpine",
				Workdir: "/workspace",
			},
		},
		DefaultLimits: execution.RunLimits{
			TimeLimit: 10 * time.Second,
		},
	})
	if err != nil {
		t.Skipf("docker runner unavailable: %v", err)
	}
	defer runner.Close()

	service := executor.NewService(runner)

	producer := &sliceProducer{
		scripts: []execution.Script{
			{
				ID:       "python-no-tests",
				Language: execution.LanguagePython,
				Source:   "print('hello from integration test')\n",
			},
			{
				ID:       "python-with-tests",
				Language: execution.LanguagePython,
				Source: `
import sys

def main():
    data = sys.stdin.read().strip()
    if not data:
        print("0")
        return
    n = int(data)
    print(n * 2)

if __name__ == "__main__":
    main()
`,
				Tests: []execution.TestCase{
					{Number: 1, Input: "2\n", ExpectedOutput: "4\n"},
					{Number: 2, Input: "5\n", ExpectedOutput: "15\n"},
				},
			},
		},
	}

	var (
		mu      sync.Mutex
		reports []execution.RunReport
	)

	err = service.ExecuteFromProducer(ctx, producer, 0, 1, func(report execution.RunReport) {
		mu.Lock()
		defer mu.Unlock()
		reports = append(reports, report)
	})
	if err != nil {
		t.Fatalf("ExecuteFromProducer returned error: %v", err)
	}

	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}

	noTests := findReport(t, reports, "python-no-tests")
	if noTests.Result == nil {
		t.Fatalf("expected result for script without tests")
	}
	if noTests.Result.Status != execution.StatusOK {
		t.Fatalf("expected OK status, got %q", noTests.Result.Status)
	}
	if noTests.Err != nil {
		t.Fatalf("unexpected error executing script: %v", noTests.Err)
	}

	withTests := findReport(t, reports, "python-with-tests")
	if withTests.Result == nil {
		t.Fatalf("expected result for script with tests")
	}
	if withTests.Result.Status != execution.StatusWrongAnswer {
		t.Fatalf("expected WrongAnswer status, got %q", withTests.Result.Status)
	}
	if len(withTests.Result.Tests) != 2 {
		t.Fatalf("expected 2 test results, got %d", len(withTests.Result.Tests))
	}
	if withTests.Result.Tests[0].Status != execution.StatusOK {
		t.Fatalf("expected first test to pass, got %q", withTests.Result.Tests[0].Status)
	}
	if withTests.Result.Tests[1].Status != execution.StatusWrongAnswer {
		t.Fatalf("expected second test to be WrongAnswer, got %q", withTests.Result.Tests[1].Status)
	}
}

type sliceProducer struct {
	mu      sync.Mutex
	scripts []execution.Script
	index   int
}

func (s *sliceProducer) NextScript(ctx context.Context) (execution.Script, error) {
	select {
	case <-ctx.Done():
		return execution.Script{}, ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.index >= len(s.scripts) {
		return execution.Script{}, io.EOF
	}

	script := s.scripts[s.index]
	s.index++
	return script, nil
}

func findReport(t *testing.T, reports []execution.RunReport, id string) execution.RunReport {
	t.Helper()
	for _, report := range reports {
		if report.Script.ID == id {
			return report
		}
	}
	t.Fatalf("report with id %q not found", id)
	return execution.RunReport{}
}
