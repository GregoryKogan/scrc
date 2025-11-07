package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"scrc/internal/app/executor"
	"scrc/internal/domain/execution"
	"scrc/internal/infra/docker"
	kafkainfra "scrc/internal/infra/kafka"
)

const (
	dockerImage         = "python:3.12-alpine"
	containerWorkdir    = "/tmp"
	defaultKafkaBrokers = "kafka:9092"
	defaultKafkaTopic   = "scripts"
	defaultKafkaGroupID = "scrc-runner"
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

	if err := service.ExecuteFromProducer(
		ctx,
		consumer,
		parseMaxScripts(os.Getenv("SCRIPT_EXPECTED")),
		func(report execution.RunReport) {
			if report.Err != nil {
				log.Printf("script %q failed: %v", report.Script.ID, report.Err)
				return
			}

			result := report.Result
			fmt.Printf("script %q exited with status %d after %s\n", report.Script.ID, result.ExitCode, result.Duration.Round(time.Millisecond))
			if result.Stdout != "" {
				fmt.Print(result.Stdout)
			}
			if result.Stderr != "" {
				fmt.Fprint(log.Writer(), result.Stderr)
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
