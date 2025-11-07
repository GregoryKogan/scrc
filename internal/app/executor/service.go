package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"scrc/internal/domain/execution"
	"scrc/internal/ports"
)

// Service coordinates script execution through a runtime implementation.
type Service struct {
	runtime ports.PythonRunner
}

// NewService constructs a Service with the provided runtime dependency.
func NewService(runtime ports.PythonRunner) *Service {
	return &Service{runtime: runtime}
}

// ExecutePython runs the supplied Python source and returns execution details.
func (s *Service) ExecutePython(ctx context.Context, source string) (*execution.Result, error) {
	return s.ExecutePythonWithLimits(ctx, source, execution.RunLimits{})
}

// ExecutePythonWithLimits runs the supplied Python source with the provided resource limits.
func (s *Service) ExecutePythonWithLimits(ctx context.Context, source string, limits execution.RunLimits) (*execution.Result, error) {
	return s.runtime.RunPython(ctx, source, limits, "")
}

// ExecuteFromProducer pulls scripts from the supplied producer and runs them with bounded parallelism.
//
// If maxScripts is greater than zero the execution stops after the specified
// number of scripts has been processed. Otherwise it keeps consuming until the
// context is cancelled or the producer signals completion via io.EOF.
//
// When onReport is provided it is invoked after every script execution with
// the corresponding run report.
func (s *Service) ExecuteFromProducer(
	ctx context.Context,
	producer ports.ScriptProducer,
	maxScripts int,
	maxParallel int,
	onReport func(execution.RunReport),
) error {
	if maxParallel <= 0 {
		maxParallel = 1
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)
	processed := 0

	finish := func(err error) error {
		wg.Wait()
		return err
	}

	for {
		if maxScripts > 0 && processed >= maxScripts {
			return finish(nil)
		}

		script, err := producer.NextScript(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
				return finish(nil)
			}

			return finish(fmt.Errorf("get next script: %w", err))
		}

		sem <- struct{}{}
		wg.Add(1)
		processed++
		go func(script execution.Script) {
			defer wg.Done()
			defer func() { <-sem }()

			report := s.executeScriptSuite(ctx, script)
			if onReport != nil {
				onReport(report)
			}
		}(script)
	}
}

func (s *Service) executeScriptSuite(ctx context.Context, script execution.Script) execution.RunReport {
	if len(script.Tests) == 0 {
		result, runErr := s.runtime.RunPython(ctx, script.Source, script.Limits, "")
		return execution.RunReport{
			Script: script,
			Result: result,
			Err:    runErr,
		}
	}

	tests := make([]execution.TestResult, len(script.Tests))
	overallStatus := execution.StatusOK
	var suiteResult execution.Result
	var runErr error
	var totalDuration time.Duration

	for idx, test := range script.Tests {
		testResult := execution.TestResult{
			Case: test,
		}

		if overallStatus != execution.StatusOK {
			testResult.Status = execution.StatusNotRun
			tests[idx] = testResult
			continue
		}

		run, err := s.runtime.RunPython(ctx, script.Source, script.Limits, test.Input)
		if err != nil && runErr == nil {
			runErr = err
			testResult.Error = err.Error()
		}

		if run != nil {
			testResult.Stdout = run.Stdout
			testResult.Stderr = run.Stderr
			testResult.ExitCode = run.ExitCode
			testResult.Duration = run.Duration
			totalDuration += run.Duration
			if run.Status != "" {
				testResult.Status = run.Status
			}

			suiteResult.Stdout = run.Stdout
			suiteResult.Stderr = run.Stderr
			suiteResult.ExitCode = run.ExitCode
			suiteResult.Duration = run.Duration
		}

		if testResult.Status == "" {
			testResult.Status = execution.StatusOK
		}

		if testResult.Status == execution.StatusOK && testResult.Stdout != test.ExpectedOutput {
			testResult.Status = execution.StatusWrongAnswer
		}

		tests[idx] = testResult
		if testResult.Status != execution.StatusOK {
			overallStatus = testResult.Status
		}
	}
	suiteResult.Status = overallStatus
	suiteResult.Tests = tests
	suiteResult.Duration = totalDuration

	return execution.RunReport{
		Script: script,
		Result: &suiteResult,
		Err:    runErr,
	}
}

// Close releases any resources owned by the underlying runtime.
func (s *Service) Close() error {
	return s.runtime.Close()
}
