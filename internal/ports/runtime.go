package ports

import (
	"context"

	"scrc/internal/domain/execution"
)

// PreparedScript represents a compiled or otherwise ready-to-run script instance.
type PreparedScript interface {
	Run(ctx context.Context, stdin string) (*execution.Result, error)
	Close() error
}

// Runner prepares and executes scripts written in various languages.
type Runner interface {
	Prepare(ctx context.Context, script execution.Script) (PreparedScript, *execution.Result, error)
	Close() error
}

