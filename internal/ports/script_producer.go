package ports

import (
	"context"

	"scrc/internal/domain/execution"
)

// ScriptProducer provides Python scripts that can be executed by a runner service.
//
// Implementations are expected to block until a script becomes available or the
// supplied context is cancelled. Returning io.EOF indicates that no further
// scripts will be produced.
type ScriptProducer interface {
	NextScript(ctx context.Context) (execution.Script, error)
}
