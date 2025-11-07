package ports

import (
	"context"

	"scrc/internal/domain/execution"
)

// PythonRunner describes capabilities for executing Python code.
type PythonRunner interface {
	RunPython(ctx context.Context, source string, limits execution.RunLimits) (*execution.Result, error)
	Close() error
}
