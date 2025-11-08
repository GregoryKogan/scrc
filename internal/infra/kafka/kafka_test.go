package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"scrc/internal/domain/execution"
)

func TestNewConsumerValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewConsumer(Config{}); err == nil {
		t.Fatalf("expected error when brokers missing")
	}
	if _, err := NewConsumer(Config{Brokers: []string{"localhost:9092"}}); err == nil {
		t.Fatalf("expected error when topic missing")
	}
}

func TestNewConsumerAppliesDefaults(t *testing.T) {
	t.Parallel()

	consumer, err := NewConsumer(Config{
		Brokers: []string{"localhost:9092"},
		Topic:   "scripts",
	})
	if err != nil {
		t.Fatalf("NewConsumer returned error: %v", err)
	}
	if err := consumer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestConsumerNextScriptParsesEnvelope(t *testing.T) {
	t.Parallel()

	envelope := scriptEnvelope{
		Language: string(execution.LanguagePython),
		Source:   "print('hi')",
		Limits: &scriptLimits{
			TimeLimitMs:      500,
			MemoryLimitBytes: 128,
		},
		Tests: []scriptTestCase{{Number: 0, Input: "1", ExpectedOutput: "1"}},
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}

	reader := &fakeReader{messages: []kafkago.Message{{Key: []byte("script-1"), Value: payload}}}
	consumer := newConsumer(reader)

	script, err := consumer.NextScript(context.Background())
	if err != nil {
		t.Fatalf("NextScript returned error: %v", err)
	}

	if script.ID != "script-1" {
		t.Fatalf("expected script ID from key, got %q", script.ID)
	}
	if script.Language != execution.LanguagePython {
		t.Fatalf("unexpected language: %q", script.Language)
	}
	if script.Limits.TimeLimit != 500*time.Millisecond {
		t.Fatalf("unexpected time limit: %v", script.Limits.TimeLimit)
	}
	if script.Limits.MemoryLimitBytes != 128 {
		t.Fatalf("unexpected memory limit: %d", script.Limits.MemoryLimitBytes)
	}
	if len(script.Tests) != 1 {
		t.Fatalf("expected one test case")
	}
	if script.Tests[0].Number != 1 {
		t.Fatalf("expected test number to default to 1, got %d", script.Tests[0].Number)
	}
}

func TestConsumerNextScriptValidationErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		envelope scriptEnvelope
		match    string
	}{
		{
			name:     "missing source",
			envelope: scriptEnvelope{Language: string(execution.LanguagePython)},
			match:    "missing source",
		},
		{
			name: "missing language",
			envelope: scriptEnvelope{
				Source: "print('hi')",
			},
			match: "missing language",
		},
		{
			name: "unknown type",
			envelope: scriptEnvelope{
				Type:     "weird",
				Language: string(execution.LanguagePython),
				Source:   "print('hi')",
			},
			match: "unknown message type",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload, err := json.Marshal(tc.envelope)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			reader := &fakeReader{messages: []kafkago.Message{{Value: payload}}}
			consumer := newConsumer(reader)

			_, err = consumer.NextScript(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.match) {
				t.Fatalf("expected error containing %q, got %v", tc.match, err)
			}
		})
	}
}

func TestConsumerNextScriptDoneMessage(t *testing.T) {
	t.Parallel()

	envelope := scriptEnvelope{Type: messageTypeDone}
	payload, _ := json.Marshal(envelope)
	reader := &fakeReader{messages: []kafkago.Message{{Value: payload}}}
	consumer := newConsumer(reader)

	_, err := consumer.NextScript(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF for done message, got %v", err)
	}
}

func TestConsumerCloseProxiesUnderlyingReader(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{}
	consumer := newConsumer(reader)

	if err := consumer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !reader.closed {
		t.Fatalf("expected reader to be closed")
	}
}

func TestPublisherValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewPublisher(PublisherConfig{}); err == nil {
		t.Fatalf("expected error when brokers missing")
	}
	if _, err := NewPublisher(PublisherConfig{Brokers: []string{"localhost:9092"}}); err == nil {
		t.Fatalf("expected error when topic missing")
	}
}

func TestNewPublisherValidConfig(t *testing.T) {
	t.Parallel()

	publisher, err := NewPublisher(PublisherConfig{Brokers: []string{"localhost:9092"}, Topic: "script-results"})
	if err != nil {
		t.Fatalf("NewPublisher returned error: %v", err)
	}
	if err := publisher.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestPublisherPublishesRunReport(t *testing.T) {
	t.Parallel()

	writer := &fakeWriter{}
	publisher := newPublisher(writer)

	report := execution.RunReport{
		Script: execution.Script{ID: "script-42"},
		Result: &execution.Result{
			Status:   execution.StatusWrongAnswer,
			Stdout:   "out",
			Stderr:   "err",
			ExitCode: 7,
			Duration: 1500 * time.Millisecond,
			Tests: []execution.TestResult{
				{
					Case:     execution.TestCase{Number: 3, Input: "2", ExpectedOutput: "4"},
					Status:   execution.StatusWrongAnswer,
					Stdout:   "2",
					Stderr:   "",
					ExitCode: 1,
					Duration: 200 * time.Millisecond,
					Error:    "mismatch",
				},
			},
		},
		Err: errors.New("boom"),
	}

	if err := publisher.PublishRunReport(context.Background(), report); err != nil {
		t.Fatalf("PublishRunReport returned error: %v", err)
	}

	if len(writer.messages) != 1 {
		t.Fatalf("expected one message, got %d", len(writer.messages))
	}

	var envelope resultEnvelope
	if err := json.Unmarshal(writer.messages[0].Value, &envelope); err != nil {
		t.Fatalf("failed to unmarshal result envelope: %v", err)
	}

	if envelope.ID != "script-42" {
		t.Fatalf("unexpected ID in envelope: %q", envelope.ID)
	}
	if envelope.Status != execution.StatusWrongAnswer {
		t.Fatalf("unexpected status: %q", envelope.Status)
	}
	if envelope.Error != "boom" {
		t.Fatalf("expected propagated error, got %q", envelope.Error)
	}
	if envelope.ExitCode == nil || *envelope.ExitCode != 7 {
		t.Fatalf("expected exit code 7")
	}
	if envelope.DurationMs == nil || *envelope.DurationMs != 1500 {
		t.Fatalf("expected duration 1500ms")
	}
	if len(envelope.Tests) != 1 {
		t.Fatalf("expected one test result in envelope")
	}
	if envelope.Tests[0].Status != execution.StatusWrongAnswer {
		t.Fatalf("unexpected test status")
	}
	if envelope.Tests[0].Error != "mismatch" {
		t.Fatalf("expected test error to propagate")
	}

	if err := publisher.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !writer.closed {
		t.Fatalf("expected writer to be closed")
	}
}

func TestPublisherCloseWithNilWriter(t *testing.T) {
	t.Parallel()

	publisher := &Publisher{}
	if err := publisher.Close(); err != nil {
		t.Fatalf("Close should succeed when writer nil, got %v", err)
	}
}

func TestPublisherPublishErrors(t *testing.T) {
	t.Parallel()

	t.Run("writer nil", func(t *testing.T) {
		publisher := &Publisher{}
		err := publisher.PublishRunReport(context.Background(), execution.RunReport{})
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("expected not initialized error, got %v", err)
		}
	})

	t.Run("writer failure", func(t *testing.T) {
		publisher := newPublisher(&fakeWriter{err: errors.New("boom")})
		err := publisher.PublishRunReport(context.Background(), execution.RunReport{Script: execution.Script{ID: "123"}})
		if err == nil || !strings.Contains(err.Error(), "write message") {
			t.Fatalf("expected write failure, got %v", err)
		}
	})
}

type fakeReader struct {
	messages []kafkago.Message
	err      error
	index    int
	closed   bool
}

type fakeWriter struct {
	messages []kafkago.Message
	err      error
	closed   bool
}

func (r *fakeReader) ReadMessage(ctx context.Context) (kafkago.Message, error) {
	if r.index < len(r.messages) {
		msg := r.messages[r.index]
		r.index++
		return msg, nil
	}
	if r.err != nil {
		return kafkago.Message{}, r.err
	}
	return kafkago.Message{}, io.EOF
}

func (r *fakeReader) Close() error {
	r.closed = true
	return nil
}

func (w *fakeWriter) WriteMessages(ctx context.Context, msgs ...kafkago.Message) error {
	if w.err != nil {
		return w.err
	}
	w.messages = append(w.messages, msgs...)
	return nil
}

func (w *fakeWriter) Close() error {
	w.closed = true
	return nil
}
