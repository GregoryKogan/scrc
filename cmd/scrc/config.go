package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	"scrc/internal/domain/execution"
	"scrc/internal/runtime/docker"
)

type appConfig struct {
	KafkaBrokers []string
	ScriptsTopic string
	ResultsTopic string
	GroupID      string
	MaxScripts   int
	MaxParallel  int
}

func loadAppConfig() appConfig {
	return appConfig{
		KafkaBrokers: parseBrokerList(envOrDefault("KAFKA_BROKERS", defaultKafkaBrokers)),
		ScriptsTopic: envOrDefault("KAFKA_TOPIC", defaultKafkaTopic),
		ResultsTopic: envOrDefault("KAFKA_RESULTS_TOPIC", defaultKafkaResultsTopic),
		GroupID:      envOrDefault("KAFKA_GROUP_ID", defaultKafkaGroupID),
		MaxScripts:   parseMaxScripts(os.Getenv("SCRIPT_EXPECTED")),
		MaxParallel:  parseMaxParallel(os.Getenv("RUNNER_MAX_PARALLEL")),
	}
}

func parseBrokerList(raw string) []string {
	fields := strings.Split(raw, ",")
	brokers := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			brokers = append(brokers, trimmed)
		}
	}
	return brokers
}

func parseMaxScripts(raw string) int {
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	if value < 0 {
		return 0
	}
	return value
}

func parseMaxParallel(raw string) int {
	if raw == "" {
		return 1
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 1
	}
	return value
}

func dockerConfigFromEnv() docker.Config {
	return docker.Config{
		Languages: map[execution.Language]docker.LanguageConfig{
			execution.LanguagePython: {
				Image:   envOrDefault("PYTHON_IMAGE", pythonDockerImage),
				Workdir: envOrDefault("PYTHON_WORKDIR", containerWorkdir),
			},
			execution.LanguageGo: {
				Image:   envOrDefault("GO_IMAGE", goDockerImage),
				Workdir: envOrDefault("GO_WORKDIR", containerWorkdir),
			},
		},
		DefaultLimits: execution.RunLimits{
			TimeLimit:        parseDuration(os.Getenv("RUNNER_TIME_LIMIT"), 0),
			MemoryLimitBytes: parseBytes(os.Getenv("RUNNER_MEMORY_LIMIT")),
		},
	}
}

func parseDuration(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func parseBytes(raw string) int64 {
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}
