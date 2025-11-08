package runtime

import (
	"context"

	"scrc/internal/domain/execution"
)

// PreparedScript represents a compiled or otherwise ready-to-run script instance.
type PreparedScript interface {
	Run(ctx context.Context, stdin string) (*execution.Result, error)
	Close() error
}

// Engine executes scripts by delegating to language-specific modules.
type Engine interface {
	Prepare(ctx context.Context, script execution.Script) (PreparedScript, *execution.Result, error)
	Close() error
}

// Module provides runtime support for a specific language.
type Module interface {
	Language() execution.Language
	Prepare(ctx context.Context, script execution.Script) (PreparedScript, *execution.Result, error)
	Close() error
}
