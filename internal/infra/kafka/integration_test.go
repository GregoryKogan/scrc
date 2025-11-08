//go:build integration

package kafka

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	kafkatc "github.com/testcontainers/testcontainers-go/modules/kafka"

	"scrc/internal/domain/execution"
	"scrc/internal/testhelpers"
)

func TestPublisherPublishesToKafka(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping Kafka integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	kafkaContainer, err := kafkatc.Run(ctx, "confluentinc/confluent-local:7.7.0")
	if err != nil {
		t.Skipf("skipping Kafka integration test (requires Docker): %v", err)
	}
	t.Cleanup(func() {
		_ = kafkaContainer.Terminate(context.Background())
	})

	brokers, err := kafkaContainer.Brokers(ctx)
	if err != nil {
		t.Fatalf("failed to obtain bootstrap servers: %v", err)
	}
	if len(brokers) == 0 {
		t.Fatal("kafka provided zero bootstrap servers")
	}

	broker := brokers[0]
	topic := "script-results"

	if err := testhelpers.WaitForKafkaBroker(ctx, broker); err != nil {
		t.Fatalf("wait for broker: %v", err)
	}
	if err := testhelpers.EnsureKafkaTopic(ctx, broker, topic); err != nil {
		t.Fatalf("ensure topic: %v", err)
	}

	publisher, err := NewPublisher(PublisherConfig{
		Brokers: []string{broker},
		Topic:   topic,
	})
	if err != nil {
		t.Fatalf("NewPublisher failed: %v", err)
	}
	defer publisher.Close()

	report := sampleRunReport()
	if err := publisher.PublishRunReport(ctx, report); err != nil {
		t.Fatalf("PublishRunReport returned error: %v", err)
	}

	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: []string{broker},
		Topic:   topic,
		GroupID: "integration-test",
	})
	t.Cleanup(func() {
		_ = reader.Close()
	})

	msgCtx, cancelRead := context.WithTimeout(ctx, 20*time.Second)
	defer cancelRead()

	msg, err := reader.ReadMessage(msgCtx)
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}

	var envelope resultEnvelope
	if err := json.Unmarshal(msg.Value, &envelope); err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}

	if envelope.ID != report.Script.ID {
		t.Fatalf("expected envelope ID %q, got %q", report.Script.ID, envelope.ID)
	}
	if envelope.Status != report.Result.Status {
		t.Fatalf("expected status %q, got %q", report.Result.Status, envelope.Status)
	}
	if envelope.ExitCode == nil || *envelope.ExitCode != report.Result.ExitCode {
		t.Fatalf("expected exit code %d, got %v", report.Result.ExitCode, envelope.ExitCode)
	}
}

func sampleRunReport() execution.RunReport {
	return execution.RunReport{
		Script: execution.Script{
			ID: "script-123",
		},
		Result: &execution.Result{
			Status:   execution.StatusOK,
			ExitCode: 0,
			Stdout:   "ok\n",
			Stderr:   "",
			Duration: 1500 * time.Millisecond,
		},
	}
}
