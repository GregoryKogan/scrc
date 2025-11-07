package executor

import (
	"context"
	"errors"
	"fmt"
	"io"

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
	return s.runtime.RunPython(ctx, source, limits)
}

// ExecuteFromProducer pulls scripts from the supplied producer and runs them sequentially.
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
	onReport func(execution.RunReport),
) error {
	processed := 0

	for {
		if maxScripts > 0 && processed >= maxScripts {
			return nil
		}

		script, err := producer.NextScript(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("get next script: %w", err)
		}

		result, runErr := s.runtime.RunPython(ctx, script.Source, script.Limits)
		report := execution.RunReport{
			Script: script,
			Result: result,
			Err:    runErr,
		}

		if onReport != nil {
			onReport(report)
		}

		processed++
	}
}

// Close releases any resources owned by the underlying runtime.
func (s *Service) Close() error {
	return s.runtime.Close()
}
