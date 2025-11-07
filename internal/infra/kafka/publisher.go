package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"scrc/internal/domain/execution"
	"scrc/internal/ports"
)

// Ensure Publisher implements ports.RunReportPublisher.
var _ ports.RunReportPublisher = (*Publisher)(nil)

// PublisherConfig configures the Kafka-based run report publisher.
type PublisherConfig struct {
	Brokers []string
	Topic   string
}

// Publisher publishes run reports to Kafka.
type Publisher struct {
	writer *kafkago.Writer
}

// NewPublisher constructs a Publisher using the supplied configuration.
func NewPublisher(cfg PublisherConfig) (*Publisher, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("at least one broker must be provided")
	}
	if cfg.Topic == "" {
		return nil, fmt.Errorf("topic must be provided")
	}

	writer := &kafkago.Writer{
		Addr:                   kafkago.TCP(cfg.Brokers...),
		Topic:                  cfg.Topic,
		AllowAutoTopicCreation: true,
		Balancer:               &kafkago.LeastBytes{},
		RequiredAcks:           kafkago.RequireAll,
		BatchTimeout:           10 * time.Millisecond,
	}

	return &Publisher{writer: writer}, nil
}

// PublishRunReport serializes and writes the supplied report to Kafka.
func (p *Publisher) PublishRunReport(ctx context.Context, report execution.RunReport) error {
	if p.writer == nil {
		return fmt.Errorf("publisher is not initialized")
	}

	payload, err := json.Marshal(makeResultEnvelope(report))
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	msg := kafkago.Message{
		Key:   []byte(report.Script.ID),
		Value: payload,
		Time:  time.Now(),
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

// Close releases the underlying Kafka writer.
func (p *Publisher) Close() error {
	if p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

type resultEnvelope struct {
	ID         string               `json:"id"`
	Status     execution.Status     `json:"status,omitempty"`
	ExitCode   *int64               `json:"exit_code,omitempty"`
	Stdout     string               `json:"stdout,omitempty"`
	Stderr     string               `json:"stderr,omitempty"`
	DurationMs *int64               `json:"duration_ms,omitempty"`
	Error      string               `json:"error,omitempty"`
	Tests      []testResultEnvelope `json:"tests,omitempty"`
	Timestamp  time.Time            `json:"timestamp"`
}

type testResultEnvelope struct {
	Number         int              `json:"number"`
	Status         execution.Status `json:"status,omitempty"`
	ExitCode       *int64           `json:"exit_code,omitempty"`
	DurationMs     *int64           `json:"duration_ms,omitempty"`
	Stdout         string           `json:"stdout,omitempty"`
	Stderr         string           `json:"stderr,omitempty"`
	Input          string           `json:"input,omitempty"`
	ExpectedOutput string           `json:"expected_output,omitempty"`
	Error          string           `json:"error,omitempty"`
}

func makeResultEnvelope(report execution.RunReport) resultEnvelope {
	var exitCode *int64
	var durationMs *int64
	var stdout string
	var stderr string
	var status execution.Status
	var tests []testResultEnvelope

	if report.Result != nil {
		exit := report.Result.ExitCode
		exitCode = &exit

		dur := report.Result.Duration.Milliseconds()
		durationMs = &dur

		stdout = report.Result.Stdout
		stderr = report.Result.Stderr
		status = report.Result.Status

		if len(report.Result.Tests) > 0 {
			tests = make([]testResultEnvelope, 0, len(report.Result.Tests))
			for _, test := range report.Result.Tests {
				testExit := test.ExitCode
				testDuration := test.Duration.Milliseconds()
				tests = append(tests, testResultEnvelope{
					Number:         test.Case.Number,
					Status:         test.Status,
					ExitCode:       &testExit,
					DurationMs:     &testDuration,
					Stdout:         test.Stdout,
					Stderr:         test.Stderr,
					Input:          test.Case.Input,
					ExpectedOutput: test.Case.ExpectedOutput,
					Error:          test.Error,
				})
			}
		}
	}

	errMsg := ""
	if report.Err != nil {
		errMsg = report.Err.Error()
	}

	return resultEnvelope{
		ID:         report.Script.ID,
		Status:     status,
		ExitCode:   exitCode,
		Stdout:     stdout,
		Stderr:     stderr,
		DurationMs: durationMs,
		Error:      errMsg,
		Tests:      tests,
		Timestamp:  time.Now().UTC(),
	}
}
