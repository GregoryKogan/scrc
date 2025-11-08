package kafka

import (
	"context"
	"fmt"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"scrc/internal/domain/execution"
	"scrc/internal/ports"
)

// Config describes how to connect to a Kafka cluster for consuming scripts.
type Config struct {
	Brokers  []string
	Topic    string
	GroupID  string
	MinBytes int
	MaxBytes int
	MaxWait  time.Duration
}

var _ ports.ScriptProducer = (*Consumer)(nil)

// Consumer wraps a kafka-go reader to implement ports.ScriptProducer.
type Consumer struct {
	reader messageReader
}

type messageReader interface {
	ReadMessage(ctx context.Context) (kafkago.Message, error)
	Close() error
}

// NewConsumer builds a new Consumer from the provided configuration.
func NewConsumer(cfg Config) (*Consumer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("at least one broker must be provided")
	}
	if cfg.Topic == "" {
		return nil, fmt.Errorf("topic must be provided")
	}
	if cfg.GroupID == "" {
		cfg.GroupID = "scrc-runner"
	}

	readerConfig := kafkago.ReaderConfig{
		Brokers:  cfg.Brokers,
		Topic:    cfg.Topic,
		GroupID:  cfg.GroupID,
		MinBytes: cfg.MinBytes,
		MaxBytes: cfg.MaxBytes,
		MaxWait:  cfg.MaxWait,
	}

	if readerConfig.MinBytes == 0 {
		readerConfig.MinBytes = 1
	}
	if readerConfig.MaxBytes == 0 {
		readerConfig.MaxBytes = 10 * 1024 * 1024
	}
	if readerConfig.MaxWait == 0 {
		readerConfig.MaxWait = time.Second
	}

	return newConsumer(kafkago.NewReader(readerConfig)), nil
}

func newConsumer(reader messageReader) *Consumer {
	return &Consumer{reader: reader}
}

// NextScript blocks until the next script message is available in Kafka or the context is cancelled.
func (c *Consumer) NextScript(ctx context.Context) (execution.Script, error) {
	msg, err := c.reader.ReadMessage(ctx)
	if err != nil {
		return execution.Script{}, err
	}

	return decodeScriptMessage(msg)
}

// Close releases the underlying Kafka reader.
func (c *Consumer) Close() error {
	return c.reader.Close()
}
