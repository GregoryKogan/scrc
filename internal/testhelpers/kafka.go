//go:build integration

package testhelpers

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

const brokerWaitInterval = 500 * time.Millisecond
const brokerWaitTimeout = 30 * time.Second

// WaitForKafkaBroker blocks until the provided broker address accepts connections or the context ends.
func WaitForKafkaBroker(ctx context.Context, broker string) error {
	deadline := time.Now().Add(brokerWaitTimeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}

	for time.Now().Before(deadline) {
		conn, err := kafkago.Dial("tcp", broker)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		select {
		case <-time.After(brokerWaitInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("kafka broker %q not ready before timeout", broker)
}

// EnsureKafkaTopic creates the provided topic if it doesn't exist.
func EnsureKafkaTopic(ctx context.Context, broker, topic string) error {
	conn, err := kafkago.DialContext(ctx, "tcp", broker)
	if err != nil {
		return fmt.Errorf("dial broker: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("controller: %w", err)
	}

	controllerAddr := net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port))
	ctrlConn, err := kafkago.DialContext(ctx, "tcp", controllerAddr)
	if err != nil {
		return fmt.Errorf("dial controller: %w", err)
	}
	defer ctrlConn.Close()

	return ctrlConn.CreateTopics(kafkago.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
}
