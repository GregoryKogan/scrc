//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	kafkatc "github.com/testcontainers/testcontainers-go/modules/kafka"

	"scrc/internal/app/executor"
	"scrc/internal/domain/execution"
	kafkainfra "scrc/internal/infra/kafka"
	"scrc/internal/runtime/docker"
	"scrc/internal/testhelpers"
)

func TestPipelineEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pipeline integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	kafkaContainer, err := kafkatc.Run(ctx, "confluentinc/confluent-local:7.7.0")
	if err != nil {
		t.Skipf("kafka container unavailable: %v", err)
	}
	defer kafkaContainer.Terminate(context.Background())

	brokers, err := kafkaContainer.Brokers(ctx)
	if err != nil {
		t.Fatalf("failed to obtain broker addresses: %v", err)
	}
	if len(brokers) == 0 {
		t.Fatal("no brokers returned by kafka container")
	}
	broker := brokers[0]

	const (
		scriptsTopic = "integration-scripts"
		resultsTopic = "integration-results"
	)

	if err := testhelpers.WaitForKafkaBroker(ctx, broker); err != nil {
		t.Fatalf("wait for kafka broker: %v", err)
	}
	if err := testhelpers.EnsureKafkaTopic(ctx, broker, scriptsTopic); err != nil {
		t.Fatalf("ensure scripts topic: %v", err)
	}
	if err := testhelpers.EnsureKafkaTopic(ctx, broker, resultsTopic); err != nil {
		t.Fatalf("ensure results topic: %v", err)
	}

	runner, err := docker.New(docker.Config{
		Languages: map[execution.Language]docker.LanguageConfig{
			execution.LanguagePython: {
				Image:   "python:3.12-alpine",
				Workdir: "/workspace",
			},
		},
		DefaultLimits: execution.RunLimits{
			TimeLimit: 15 * time.Second,
		},
	})
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	defer runner.Close()

	service := executor.NewService(runner)
	defer service.Close()

	consumer, err := kafkainfra.NewConsumer(kafkainfra.Config{
		Brokers: []string{broker},
		Topic:   scriptsTopic,
		GroupID: "pipeline-integration-consumer",
	})
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}
	defer consumer.Close()

	publisher, err := kafkainfra.NewPublisher(kafkainfra.PublisherConfig{
		Brokers: []string{broker},
		Topic:   resultsTopic,
	})
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	defer publisher.Close()

	execCtx, execCancel := context.WithCancel(ctx)
	defer execCancel()

	errCh := make(chan error, 1)
	sendErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	go func() {
		defer execCancel()
		err := service.ExecuteFromProducer(execCtx, consumer, 1, 1, func(report execution.RunReport) {
			if pubErr := publisher.PublishRunReport(execCtx, report); pubErr != nil {
				sendErr(fmt.Errorf("publish run report: %w", pubErr))
				execCancel()
			}
		})
		sendErr(err)
	}()

	scriptID := "pipeline-script"
	writer := &kafkago.Writer{
		Addr:                   kafkago.TCP(broker),
		Topic:                  scriptsTopic,
		AllowAutoTopicCreation: false,
		Balancer:               &kafkago.LeastBytes{},
	}
	defer writer.Close()

	scriptPayload, err := json.Marshal(map[string]any{
		"type":     "script",
		"id":       scriptID,
		"language": string(execution.LanguagePython),
		"source": `
import sys
data = sys.stdin.read().strip()
n = int(data)
print(n + 1)
`,
		"tests": []map[string]any{
			{"number": 1, "input": "1\n", "expected_output": "2\n"},
			{"number": 2, "input": "3\n", "expected_output": "4\n"},
		},
	})
	if err != nil {
		t.Fatalf("marshal script payload: %v", err)
	}

	if err := writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(scriptID),
		Value: scriptPayload,
	}); err != nil {
		t.Fatalf("write script message: %v", err)
	}

	resultsReader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: []string{broker},
		Topic:   resultsTopic,
		GroupID: "pipeline-integration-results",
	})
	defer resultsReader.Close()

	msgCtx, msgCancel := context.WithTimeout(ctx, time.Minute)
	defer msgCancel()

	msg, err := resultsReader.ReadMessage(msgCtx)
	if err != nil {
		t.Fatalf("read result message: %v", err)
	}

	var envelope struct {
		ID        string               `json:"id"`
		Status    execution.Status     `json:"status"`
		ExitCode  *int64               `json:"exit_code"`
		Tests     []testResultEnvelope `json:"tests"`
		Timestamp time.Time            `json:"timestamp"`
	}
	if err := json.Unmarshal(msg.Value, &envelope); err != nil {
		t.Fatalf("decode result message: %v", err)
	}

	if envelope.ID != scriptID {
		t.Fatalf("expected result for %q, got %q", scriptID, envelope.ID)
	}
	if envelope.Status != execution.StatusOK {
		t.Fatalf("expected overall status OK, got %q", envelope.Status)
	}
	if envelope.ExitCode == nil || *envelope.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %v", envelope.ExitCode)
	}
	if len(envelope.Tests) != 2 {
		t.Fatalf("expected 2 test results, got %d", len(envelope.Tests))
	}
	for i, test := range envelope.Tests {
		if test.Status != execution.StatusOK {
			t.Fatalf("expected test %d status OK, got %q", i+1, test.Status)
		}
	}

	if err := <-errCh; err != nil {
		t.Fatalf("pipeline execution error: %v", err)
	}
}

type testResultEnvelope struct {
	Number int              `json:"number"`
	Status execution.Status `json:"status"`
}
