package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"scrc/internal/domain/execution"
	"scrc/internal/ports"
)

const (
	messageTypeScript = "script"
	messageTypeDone   = "done"
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
	reader *kafkago.Reader
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

	return &Consumer{reader: kafkago.NewReader(readerConfig)}, nil
}

// NextScript blocks until the next script message is available in Kafka or the context is cancelled.
func (c *Consumer) NextScript(ctx context.Context) (execution.Script, error) {
	msg, err := c.reader.ReadMessage(ctx)
	if err != nil {
		return execution.Script{}, err
	}

	var envelope scriptEnvelope
	if err := json.Unmarshal(msg.Value, &envelope); err != nil {
		return execution.Script{}, fmt.Errorf("decode message: %w", err)
	}

	msgType := envelope.Type
	if msgType == "" {
		msgType = messageTypeScript
	}

	switch msgType {
	case messageTypeScript:
		if envelope.Source == "" {
			return execution.Script{}, fmt.Errorf("script message missing source")
		}

		scriptID := envelope.ID
		if scriptID == "" {
			scriptID = string(msg.Key)
		}
		if scriptID == "" {
			scriptID = fmt.Sprintf("%s:%d", msg.Topic, msg.Offset)
		}

		limits := execution.RunLimits{}
		if envelope.Limits != nil {
			if envelope.Limits.TimeLimitMs > 0 {
				limits.TimeLimit = time.Duration(envelope.Limits.TimeLimitMs) * time.Millisecond
			}
			if envelope.Limits.MemoryLimitBytes > 0 {
				limits.MemoryLimitBytes = envelope.Limits.MemoryLimitBytes
			}
		}

		return execution.Script{ID: scriptID, Source: envelope.Source, Limits: limits}, nil
	case messageTypeDone:
		return execution.Script{}, io.EOF
	default:
		return execution.Script{}, fmt.Errorf("unknown message type %q", msgType)
	}
}

// Close releases the underlying Kafka reader.
func (c *Consumer) Close() error {
	return c.reader.Close()
}

type scriptEnvelope struct {
	Type   string        `json:"type"`
	ID     string        `json:"id"`
	Source string        `json:"source"`
	Limits *scriptLimits `json:"limits,omitempty"`
}

type scriptLimits struct {
	TimeLimitMs      int64 `json:"time_limit_ms"`
	MemoryLimitBytes int64 `json:"memory_limit_bytes"`
}
