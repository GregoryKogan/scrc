package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"scrc/internal/app/executor"
	"scrc/internal/app/producer"
	"scrc/internal/infra/docker"
)

const (
	dockerImage      = "python:3.12-alpine"
	containerWorkdir = "/tmp"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

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

	scriptProducer := producer.NewService()

	reports, err := service.ExecuteFromProducer(ctx, scriptProducer)
	if err != nil {
		log.Fatalf("failed to execute scripts: %v", err)
	}

	for _, report := range reports {
		if report.Err != nil {
			log.Printf("script %q failed: %v", report.Script.ID, report.Err)
			continue
		}

		result := report.Result
		fmt.Printf("script %q exited with status %d after %s\n", report.Script.ID, result.ExitCode, result.Duration.Round(time.Millisecond))
		if result.Stdout != "" {
			fmt.Print(result.Stdout)
		}
		if result.Stderr != "" {
			fmt.Fprint(log.Writer(), result.Stderr)
		}
	}
}
