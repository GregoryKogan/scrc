package ports

import (
	"context"

	"scrc/internal/domain/execution"
)

// ScriptProducer provides Python scripts that can be executed by a runner service.
type ScriptProducer interface {
	ProduceScripts(ctx context.Context) ([]execution.Script, error)
}
