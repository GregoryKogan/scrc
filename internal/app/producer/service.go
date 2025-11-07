package producer

import (
	"context"
	"time"

	"scrc/internal/domain/execution"
	"scrc/internal/ports"
)

// Service implements ports.ScriptProducer by returning predefined Python scripts.
type Service struct {
	scripts []execution.Script
}

var _ ports.ScriptProducer = (*Service)(nil)

// NewService builds a new producer service with a default script catalogue.
func NewService() *Service {
	return &Service{
		scripts: []execution.Script{
			{
				ID:     "hello",
				Source: "print('Hello from Python inside Docker!')\n",
			},
			{
				ID:     "time",
				Source: "import datetime\nprint('Current time:', datetime.datetime.utcnow().isoformat())\n",
			},
		},
	}
}

// ProduceScripts returns the available Python scripts for execution.
func (s *Service) ProduceScripts(ctx context.Context) ([]execution.Script, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	scripts := make([]execution.Script, len(s.scripts))
	copy(scripts, s.scripts)

	return scripts, nil
}

// AddScript allows extending the producer catalogue at runtime.
func (s *Service) AddScript(script execution.Script) {
	if script.ID == "" {
		script.ID = time.Now().UTC().Format(time.RFC3339Nano)
	}
	s.scripts = append(s.scripts, script)
}
