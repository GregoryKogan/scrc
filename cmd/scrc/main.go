package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"scrc/internal/app/executor"
	"scrc/internal/domain/execution"
	kafkainfra "scrc/internal/infra/kafka"
	"scrc/internal/runtime/docker"
)

const (
	pythonDockerImage        = "python:3.12-alpine"
	goDockerImage            = "golang:1.22-alpine"
	cDockerImage             = "gcc:14"
	containerWorkdir         = "/tmp"
	defaultKafkaBrokers      = "kafka:9092"
	defaultKafkaTopic        = "scripts"
	defaultKafkaGroupID      = "scrc-runner"
	defaultKafkaResultsTopic = "script-results"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	appCfg := loadAppConfig()

	runtime, err := docker.New(dockerConfigFromEnv())
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
		Brokers: appCfg.KafkaBrokers,
		Topic:   appCfg.ScriptsTopic,
		GroupID: appCfg.GroupID,
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
		Brokers: appCfg.KafkaBrokers,
		Topic:   appCfg.ResultsTopic,
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
		appCfg.MaxScripts,
		appCfg.MaxParallel,
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
