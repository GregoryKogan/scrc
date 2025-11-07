package executor

import (
	"context"
	"fmt"

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
	return s.runtime.RunPython(ctx, source)
}

// ExecuteFromProducer pulls scripts from the supplied producer and runs them sequentially.
func (s *Service) ExecuteFromProducer(ctx context.Context, producer ports.ScriptProducer) ([]execution.RunReport, error) {
	scripts, err := producer.ProduceScripts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get scripts: %w", err)
	}

	reports := make([]execution.RunReport, 0, len(scripts))
	for _, script := range scripts {
		result, runErr := s.runtime.RunPython(ctx, script.Source)
		reports = append(reports, execution.RunReport{
			Script: script,
			Result: result,
			Err:    runErr,
		})
	}

	return reports, nil
}

// Close releases any resources owned by the underlying runtime.
func (s *Service) Close() error {
	return s.runtime.Close()
}
