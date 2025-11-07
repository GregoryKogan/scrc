package producer

import (
	"context"
	"io"
	"sync"
	"time"

	"scrc/internal/domain/execution"
	"scrc/internal/ports"
)

// Service implements ports.ScriptProducer by returning predefined Python scripts.
type Service struct {
	mu      sync.Mutex
	scripts []execution.Script
	index   int
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
				Source: "import datetime\nprint('Current time:', datetime.datetime.now(datetime.timezone.utc).isoformat())\n",
			},
		},
	}
}

// ProduceScripts returns the available Python scripts for execution.
func (s *Service) NextScript(ctx context.Context) (execution.Script, error) {
	select {
	case <-ctx.Done():
		return execution.Script{}, ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.index >= len(s.scripts) {
		return execution.Script{}, io.EOF
	}

	script := s.scripts[s.index]
	s.index++

	return script, nil
}

// AddScript allows extending the producer catalogue at runtime.
func (s *Service) AddScript(script execution.Script) {
	if script.ID == "" {
		script.ID = time.Now().UTC().Format(time.RFC3339Nano)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.scripts = append(s.scripts, script)
}
