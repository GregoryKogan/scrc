package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"scrc/internal/app/executor"
	"scrc/internal/domain/execution"
	"scrc/internal/infra/docker"
	kafkainfra "scrc/internal/infra/kafka"
)

const (
	dockerImage              = "python:3.12-alpine"
	containerWorkdir         = "/tmp"
	defaultKafkaBrokers      = "kafka:9092"
	defaultKafkaTopic        = "scripts"
	defaultKafkaGroupID      = "scrc-runner"
	defaultKafkaResultsTopic = "script-results"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := docker.New(docker.Config{
		Image:   dockerImage,
		Workdir: containerWorkdir,
	})
	if err != nil {
		log.Fatalf("failed to initialize docker runner: %v", err)
	}

	service := executor.NewService(runtime)
	defer func() {
		if cerr := service.Close(); cerr != nil {
			log.Printf("warning: failed to close runner: %v", cerr)
		}
	}()

	consumer, err := kafkainfra.NewConsumer(kafkainfra.Config{
		Brokers: parseBrokerList(envOrDefault("KAFKA_BROKERS", defaultKafkaBrokers)),
		Topic:   envOrDefault("KAFKA_TOPIC", defaultKafkaTopic),
		GroupID: envOrDefault("KAFKA_GROUP_ID", defaultKafkaGroupID),
	})
	if err != nil {
		log.Fatalf("failed to initialize kafka consumer: %v", err)
	}
	defer func() {
		if cerr := consumer.Close(); cerr != nil {
			log.Printf("warning: failed to close kafka consumer: %v", cerr)
		}
	}()

	publisher, err := kafkainfra.NewPublisher(kafkainfra.PublisherConfig{
		Brokers: parseBrokerList(envOrDefault("KAFKA_BROKERS", defaultKafkaBrokers)),
		Topic:   envOrDefault("KAFKA_RESULTS_TOPIC", defaultKafkaResultsTopic),
	})
	if err != nil {
		log.Fatalf("failed to initialize kafka publisher: %v", err)
	}
	defer func() {
		if cerr := publisher.Close(); cerr != nil {
			log.Printf("warning: failed to close kafka publisher: %v", cerr)
		}
	}()

	if err := service.ExecuteFromProducer(
		ctx,
		consumer,
		parseMaxScripts(os.Getenv("SCRIPT_EXPECTED")),
		func(report execution.RunReport) {
			if err := publisher.PublishRunReport(ctx, report); err != nil {
				log.Printf("failed to publish run report for script %q: %v", report.Script.ID, err)
			}
		},
	); err != nil {
		log.Fatalf("failed to execute scripts: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseBrokerList(raw string) []string {
	fields := strings.Split(raw, ",")
	brokers := make([]string, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed != "" {
			brokers = append(brokers, trimmed)
		}
	}
	return brokers
}

func parseMaxScripts(raw string) int {
	if raw == "" {
		return 0
	}
	maxScripts, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("warning: ignoring invalid SCRIPT_EXPECTED value %q: %v", raw, err)
		return 0
	}
	if maxScripts < 0 {
		return 0
	}
	return maxScripts
}
