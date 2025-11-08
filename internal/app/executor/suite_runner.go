package executor

import (
	"context"
	"fmt"
	"time"

	"scrc/internal/domain/execution"
	"scrc/internal/ports"
)

type suiteRunner struct {
	runtime ports.Runner
}

func newSuiteRunner(runtime ports.Runner) *suiteRunner {
	return &suiteRunner{runtime: runtime}
}

func (r *suiteRunner) Run(ctx context.Context, script execution.Script) execution.RunReport {
	prepared, buildResult, err := r.runtime.Prepare(ctx, script)
	if err != nil {
		return execution.RunReport{
			Script: script,
			Err:    err,
		}
	}
	if prepared != nil {
		defer prepared.Close()
	}

	if buildResult != nil {
		return execution.RunReport{
			Script: script,
			Result: buildResult,
		}
	}

	if prepared == nil {
		return execution.RunReport{
			Script: script,
			Err:    fmt.Errorf("runner returned nil prepared script without build result"),
		}
	}

	if len(script.Tests) == 0 {
		return r.runSingle(ctx, script, prepared)
	}

	return r.runSuite(ctx, script, prepared)
}

func (r *suiteRunner) runSingle(ctx context.Context, script execution.Script, prepared ports.PreparedScript) execution.RunReport {
	result, err := prepared.Run(ctx, "")
	return execution.RunReport{
		Script: script,
		Result: result,
		Err:    err,
	}
}

func (r *suiteRunner) runSuite(ctx context.Context, script execution.Script, prepared ports.PreparedScript) execution.RunReport {
	exec := newSuiteExecution(script, prepared)

	for idx := range script.Tests {
		exec.executeTest(ctx, idx)
	}

	return exec.finalize()
}

type suiteExecution struct {
	script    execution.Script
	prepared  ports.PreparedScript
	results   []execution.TestResult
	overall   execution.Status
	runErr    error
	totalTime time.Duration
}

func newSuiteExecution(script execution.Script, prepared ports.PreparedScript) *suiteExecution {
	return &suiteExecution{
		script:   script,
		prepared: prepared,
		results:  make([]execution.TestResult, len(script.Tests)),
		overall:  execution.StatusOK,
	}
}

func (s *suiteExecution) executeTest(ctx context.Context, idx int) {
	test := s.script.Tests[idx]
	result := execution.TestResult{Case: test}

	if s.overall != execution.StatusOK {
		result.Status = execution.StatusNotRun
		s.results[idx] = result
		return
	}

	runResult, err := s.prepared.Run(ctx, test.Input)
	if err != nil && s.runErr == nil {
		s.runErr = err
		result.Error = err.Error()
	}

	if runResult != nil {
		result.Stdout = runResult.Stdout
		result.Stderr = runResult.Stderr
		result.ExitCode = runResult.ExitCode
		result.Duration = runResult.Duration
		s.totalTime += runResult.Duration

		if runResult.Status != "" {
			result.Status = runResult.Status
		}
	}

	if result.Status == "" {
		result.Status = execution.StatusOK
	}
	if result.Status == execution.StatusOK && result.Stdout != test.ExpectedOutput {
		result.Status = execution.StatusWrongAnswer
	}

	s.results[idx] = result
	if result.Status != execution.StatusOK && s.overall == execution.StatusOK {
		s.overall = result.Status
	}
}

func (s *suiteExecution) finalize() execution.RunReport {
	suiteResult := execution.Result{
		Status:   s.overall,
		Stdout:   s.tailStdout(),
		Stderr:   s.tailStderr(),
		ExitCode: s.tailExitCode(),
		Duration: s.totalTime,
		Tests:    s.results,
	}

	return execution.RunReport{
		Script: s.script,
		Result: &suiteResult,
		Err:    s.runErr,
	}
}

func (s *suiteExecution) tailStdout() string {
	for i := len(s.results) - 1; i >= 0; i-- {
		if s.results[i].Stdout != "" {
			return s.results[i].Stdout
		}
	}
	return ""
}

func (s *suiteExecution) tailStderr() string {
	for i := len(s.results) - 1; i >= 0; i-- {
		if s.results[i].Stderr != "" {
			return s.results[i].Stderr
		}
	}
	return ""
}

func (s *suiteExecution) tailExitCode() int64 {
	for i := len(s.results) - 1; i >= 0; i-- {
		if s.results[i].ExitCode != 0 {
			return s.results[i].ExitCode
		}
	}
	return 0
}
