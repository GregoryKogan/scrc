package executor

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"scrc/internal/domain/execution"
	"scrc/internal/ports"
)

func TestExecuteFromProducerRespectsMaxParallel(t *testing.T) {
	t.Parallel()

	scripts := []execution.Script{
		{ID: "s1"},
		{ID: "s2"},
		{ID: "s3"},
		{ID: "s4"},
	}

	maxParallel := 2
	startCh := make(chan struct{}, len(scripts))
	releaseCh := make(chan struct{})
	tracker := &concurrencyTracker{}

	runner := &stubRunner{
		prepareFn: func(ctx context.Context, script execution.Script) (ports.PreparedScript, *execution.Result, error) {
			return &stubPreparedScript{
				runFn: func(ctx context.Context, stdin string) (*execution.Result, error) {
					done := tracker.enter()
					select {
					case startCh <- struct{}{}:
					default:
					}
					select {
					case <-releaseCh:
					case <-ctx.Done():
						done()
						return nil, ctx.Err()
					}
					done()
					return &execution.Result{Status: execution.StatusOK}, nil
				},
			}, nil, nil
		},
	}

	producer := &sequenceScriptProducer{scripts: scripts}
	service := NewService(runner)
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	var mu sync.Mutex
	var reports []execution.RunReport

	go func() {
		errCh <- service.ExecuteFromProducer(ctx, producer, 0, maxParallel, func(report execution.RunReport) {
			mu.Lock()
			reports = append(reports, report)
			mu.Unlock()
		})
	}()

	for range scripts {
		select {
		case <-startCh:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for script to start")
		}
		releaseCh <- struct{}{}
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ExecuteFromProducer error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("ExecuteFromProducer did not finish")
	}

	if tracker.maxActive > maxParallel {
		t.Fatalf("expected max %d concurrent runs, got %d", maxParallel, tracker.maxActive)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(reports) != len(scripts) {
		t.Fatalf("expected %d reports, got %d", len(scripts), len(reports))
	}
}

func TestExecuteFromProducerProducerError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("producer failed")
	service := NewService(&stubRunner{
		prepareFn: func(ctx context.Context, script execution.Script) (ports.PreparedScript, *execution.Result, error) {
			t.Fatalf("unexpected prepare call")
			return nil, nil, nil
		},
	})
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	}()

	err := service.ExecuteFromProducer(context.Background(), errorScriptProducer{err: wantErr}, 0, 1, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error wrapping %v, got %v", wantErr, err)
	}
}

func TestExecuteScriptSuitePrepareError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("prepare failed")
	service := &Service{runtime: &stubRunner{
		prepareFn: func(ctx context.Context, script execution.Script) (ports.PreparedScript, *execution.Result, error) {
			return nil, nil, wantErr
		},
	}}

	script := execution.Script{ID: "s1"}
	report := service.executeScriptSuite(context.Background(), script)
	if !errors.Is(report.Err, wantErr) {
		t.Fatalf("expected report err %v, got %v", wantErr, report.Err)
	}
	if report.Result != nil {
		t.Fatalf("expected no result, got %#v", report.Result)
	}
}

func TestExecuteScriptSuiteBuildResult(t *testing.T) {
	t.Parallel()

	build := &execution.Result{Status: execution.StatusBuildFail}
	service := &Service{runtime: &stubRunner{
		prepareFn: func(ctx context.Context, script execution.Script) (ports.PreparedScript, *execution.Result, error) {
			return nil, build, nil
		},
	}}

	script := execution.Script{ID: "build"}
	report := service.executeScriptSuite(context.Background(), script)
	if report.Result != build {
		t.Fatalf("expected build result pointer, got %#v", report.Result)
	}
	if report.Err != nil {
		t.Fatalf("expected no error, got %v", report.Err)
	}
}

func TestExecuteScriptSuiteWithTests(t *testing.T) {
	t.Parallel()

	runErr := errors.New("execution failed")
	prepared := &stubPreparedScript{
		runs: []preparedRun{
			{
				result: &execution.Result{
					Stdout:   "answer\n",
					Stderr:   "",
					ExitCode: 0,
					Duration: 10 * time.Millisecond,
				},
			},
			{
				result: &execution.Result{
					Stdout:   "wrong\n",
					Stderr:   "stderr",
					ExitCode: 1,
					Duration: 20 * time.Millisecond,
				},
				err: runErr,
			},
		},
	}

	service := &Service{runtime: &stubRunner{
		prepareFn: func(ctx context.Context, script execution.Script) (ports.PreparedScript, *execution.Result, error) {
			return prepared, nil, nil
		},
	}}

	script := execution.Script{
		ID: "test-suite",
		Tests: []execution.TestCase{
			{Number: 1, ExpectedOutput: "answer\n"},
			{Number: 2, ExpectedOutput: "correct\n"},
			{Number: 3, ExpectedOutput: "unused"},
		},
	}

	report := service.executeScriptSuite(context.Background(), script)
	if report.Err == nil || !errors.Is(report.Err, runErr) {
		t.Fatalf("expected run error %v, got %v", runErr, report.Err)
	}

	if report.Result == nil {
		t.Fatalf("expected suite result")
	}

	if prepared.calls != len(prepared.runs) {
		t.Fatalf("expected %d executed runs, got %d", len(prepared.runs), prepared.calls)
	}

	if got := report.Result.Status; got != execution.StatusWrongAnswer {
		t.Fatalf("expected overall status %q, got %q", execution.StatusWrongAnswer, got)
	}

	if report.Result.Duration != 30*time.Millisecond {
		t.Fatalf("expected aggregated duration 30ms, got %v", report.Result.Duration)
	}

	tests := report.Result.Tests
	if len(tests) != len(script.Tests) {
		t.Fatalf("expected %d test results, got %d", len(script.Tests), len(tests))
	}

	if tests[0].Status != execution.StatusOK {
		t.Fatalf("expected first test status OK, got %q", tests[0].Status)
	}
	if tests[0].Stdout != "answer\n" {
		t.Fatalf("unexpected stdout for test 1: %q", tests[0].Stdout)
	}

	if tests[1].Status != execution.StatusWrongAnswer {
		t.Fatalf("expected second test wrong answer, got %q", tests[1].Status)
	}
	if tests[1].Error != runErr.Error() {
		t.Fatalf("expected error captured for second test")
	}

	if tests[2].Status != execution.StatusNotRun {
		t.Fatalf("expected third test not run, got %q", tests[2].Status)
	}
}

type concurrencyTracker struct {
	mu        sync.Mutex
	active    int
	maxActive int
}

func (c *concurrencyTracker) enter() func() {
	c.mu.Lock()
	c.active++
	if c.active > c.maxActive {
		c.maxActive = c.active
	}
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		c.active--
		c.mu.Unlock()
	}
}

type stubRunner struct {
	prepareFn func(ctx context.Context, script execution.Script) (ports.PreparedScript, *execution.Result, error)
	closeFn   func() error
}

func (s *stubRunner) Prepare(ctx context.Context, script execution.Script) (ports.PreparedScript, *execution.Result, error) {
	if s.prepareFn != nil {
		return s.prepareFn(ctx, script)
	}
	return nil, nil, nil
}

func (s *stubRunner) Close() error {
	if s.closeFn != nil {
		return s.closeFn()
	}
	return nil
}

type stubPreparedScript struct {
	runFn   func(ctx context.Context, stdin string) (*execution.Result, error)
	closeFn func() error

	runs  []preparedRun
	mu    sync.Mutex
	calls int
}

type preparedRun struct {
	result *execution.Result
	err    error
}

func (s *stubPreparedScript) Run(ctx context.Context, stdin string) (*execution.Result, error) {
	if s.runFn != nil {
		return s.runFn(ctx, stdin)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.calls >= len(s.runs) {
		return nil, errors.New("unexpected run invocation")
	}
	call := s.runs[s.calls]
	s.calls++
	return call.result, call.err
}

func (s *stubPreparedScript) Close() error {
	if s.closeFn != nil {
		return s.closeFn()
	}
	return nil
}

type sequenceScriptProducer struct {
	scripts []execution.Script
	index   int
	mu      sync.Mutex
}

func (p *sequenceScriptProducer) NextScript(ctx context.Context) (execution.Script, error) {
	select {
	case <-ctx.Done():
		return execution.Script{}, ctx.Err()
	default:
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.index >= len(p.scripts) {
		return execution.Script{}, io.EOF
	}

	script := p.scripts[p.index]
	p.index++
	return script, nil
}

type errorScriptProducer struct {
	err error
}

func (p errorScriptProducer) NextScript(ctx context.Context) (execution.Script, error) {
	return execution.Script{}, p.err
}
