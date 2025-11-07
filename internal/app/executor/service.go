package executor

import (
	"context"

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

// Close releases any resources owned by the underlying runtime.
func (s *Service) Close() error {
	return s.runtime.Close()
}
