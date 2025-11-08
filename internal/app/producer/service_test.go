package producer

import (
	"context"
	"errors"
	"io"
	"testing"

	"scrc/internal/domain/execution"
)

func TestNewServiceProvidesDefaultScripts(t *testing.T) {
	t.Parallel()

	service := NewService()

	first, err := service.NextScript(context.Background())
	if err != nil {
		t.Fatalf("NextScript returned error: %v", err)
	}
	if first.ID != "hello" {
		t.Fatalf("expected first script ID 'hello', got %q", first.ID)
	}

	second, err := service.NextScript(context.Background())
	if err != nil {
		t.Fatalf("NextScript returned error: %v", err)
	}
	if second.ID != "time" {
		t.Fatalf("expected second script ID 'time', got %q", second.ID)
	}
}

func TestNextScriptReturnsEOFWhenExhausted(t *testing.T) {
	t.Parallel()

	service := NewService()

	_, _ = service.NextScript(context.Background())
	_, _ = service.NextScript(context.Background())

	_, err := service.NextScript(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestNextScriptContextCancellation(t *testing.T) {
	t.Parallel()

	service := NewService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.NextScript(ctx)
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestAddScriptAssignsIDWhenMissing(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.AddScript(execution.Script{Source: "print('hello')"})

	// consume defaults
	_, _ = service.NextScript(context.Background())
	_, _ = service.NextScript(context.Background())

	script, err := service.NextScript(context.Background())
	if err != nil {
		t.Fatalf("NextScript returned error: %v", err)
	}

	if script.ID == "" {
		t.Fatalf("expected generated script ID")
	}
	if script.Source != "print('hello')" {
		t.Fatalf("unexpected script source: %q", script.Source)
	}
}

func TestAddScriptPreservesExistingID(t *testing.T) {
	t.Parallel()

	service := NewService()
	expectedID := "custom"
	service.AddScript(execution.Script{ID: expectedID, Source: "print('x')"})

	// consume defaults
	_, _ = service.NextScript(context.Background())
	_, _ = service.NextScript(context.Background())

	script, err := service.NextScript(context.Background())
	if err != nil {
		t.Fatalf("NextScript returned error: %v", err)
	}

	if script.ID != expectedID {
		t.Fatalf("expected script ID %q, got %q", expectedID, script.ID)
	}
}
