package kafka

import (
	"context"
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
	writer messageWriter
}

type messageWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafkago.Message) error
	Close() error
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

	return newPublisher(writer), nil
}

func newPublisher(writer messageWriter) *Publisher {
	return &Publisher{writer: writer}
}

// PublishRunReport serializes and writes the supplied report to Kafka.
func (p *Publisher) PublishRunReport(ctx context.Context, report execution.RunReport) error {
	if p.writer == nil {
		return fmt.Errorf("publisher is not initialized")
	}

	payload, err := encodeRunReport(report)
	if err != nil {
		return err
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
