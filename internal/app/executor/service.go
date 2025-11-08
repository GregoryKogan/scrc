package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"scrc/internal/domain/execution"
	"scrc/internal/ports"
)

// Service coordinates script execution through a runtime implementation.
type Service struct {
	runtime ports.Runner
	suites  *suiteRunner
}

// NewService constructs a Service with the provided runtime dependency.
func NewService(runtime ports.Runner) *Service {
	return &Service{
		runtime: runtime,
		suites:  newSuiteRunner(runtime),
	}
}

// ExecuteFromProducer pulls scripts from the supplied producer and runs them with bounded parallelism.
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
	maxParallel int,
	onReport func(execution.RunReport),
) error {
	if maxParallel <= 0 {
		maxParallel = 1
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)
	processed := 0

	finish := func(err error) error {
		wg.Wait()
		return err
	}

	for {
		if maxScripts > 0 && processed >= maxScripts {
			return finish(nil)
		}

		script, err := producer.NextScript(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
				return finish(nil)
			}

			return finish(fmt.Errorf("get next script: %w", err))
		}

		sem <- struct{}{}
		wg.Add(1)
		processed++
		go func(script execution.Script) {
			defer wg.Done()
			defer func() { <-sem }()

			report := s.suites.Run(ctx, script)
			if onReport != nil {
				onReport(report)
			}
		}(script)
	}
}

// Close releases any resources owned by the underlying runtime.
func (s *Service) Close() error {
	return s.runtime.Close()
}
