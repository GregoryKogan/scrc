# SCRC - System for Compiling and Running Code

## Table of Contents

- [Introduction](#introduction)
  - [What is SCRC?](#what-is-scrc)
  - [Use Cases](#use-cases)
  - [Features](#features)
  - [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
  - [Installation](#installation)
  - [Running Locally](#running-locally)
  - [Running with Docker](#running-with-docker)
  - [Quick Test with Docker Compose](#quick-test-with-docker-compose)
- [Architecture](#architecture)
  - [System Overview](#system-overview)
  - [Component Diagram](#component-diagram)
  - [Data Flow](#data-flow)
  - [Concurrency Model](#concurrency-model)
  - [Codebase Structure](#codebase-structure)
- [Configuration](#configuration)
  - [Environment Variables Reference](#environment-variables-reference)
    - [Kafka Configuration](#kafka-configuration)
    - [Runtime Configuration](#runtime-configuration)
    - [Language Image Configuration](#language-image-configuration)
    - [Docker Build Configuration](#docker-build-configuration)
  - [Default Values](#default-values)
  - [Configuration Examples](#configuration-examples)
    - [High-Throughput Configuration](#high-throughput-configuration)
    - [Resource-Limited Environment](#resource-limited-environment)
    - [Custom Language Images](#custom-language-images)
    - [Development Configuration](#development-configuration)
- [Message Formats](#message-formats)
  - [Script Message Format](#script-message-format)
    - [Field Descriptions](#field-descriptions)
    - [Validation Rules](#validation-rules)
  - [Result Message Format](#result-message-format)
    - [Field Descriptions](#field-descriptions-1)
    - [Status Codes](#status-codes)
  - [Examples](#examples)
    - [Minimal Script Message](#minimal-script-message)
    - [Script with Resource Limits](#script-with-resource-limits)
    - [Script with Test Cases](#script-with-test-cases)
    - [Result Message Example](#result-message-example)
- [Language Support](#language-support)
  - [Supported Languages](#supported-languages)
    - [Language-Specific Behaviors](#language-specific-behaviors)
  - [Adding New Languages](#adding-new-languages)
    - [Example: Adding Rust Support](#example-adding-rust-support)
  - [Language-Specific Configuration](#language-specific-configuration)
    - [Python Configuration](#python-configuration)
    - [Go Configuration](#go-configuration)
    - [C/C++ Configuration](#cc-configuration)
    - [Java Configuration](#java-configuration)
- [Running the Service](#running-the-service)
  - [Local Execution](#local-execution)
  - [Docker Deployment](#docker-deployment)
    - [Custom Go Version](#custom-go-version)
  - [Docker Compose Setups](#docker-compose-setups)
    - [Single Runner (Simulator Demo)](#single-runner-simulator-demo)
    - [Horizontal Scaling Demo](#horizontal-scaling-demo)
    - [Multi-Runner Without Simulator](#multi-runner-without-simulator)
  - [Production Deployment](#production-deployment)
    - [Prerequisites](#prerequisites-1)
    - [Deployment Considerations](#deployment-considerations)
    - [Example Production Configuration](#example-production-configuration)
    - [Resource Requirements](#resource-requirements)
- [Simulator](#simulator)
  - [Simulator Overview](#simulator-overview)
  - [Simulator Usage](#simulator-usage)
  - [Simulator Configuration](#simulator-configuration)
    - [Example: Extended Test Run](#example-extended-test-run)
- [Benchmarking](#benchmarking)
  - [Benchmark Overview](#benchmark-overview)
  - [Benchmarking Strategy](#benchmarking-strategy)
    - [Multiple Runs with Aggregation](#multiple-runs-with-aggregation)
    - [Warmup Period](#warmup-period)
    - [Measurement Window](#measurement-window)
    - [Cooldown Period](#cooldown-period)
    - [Data Filtering](#data-filtering)
    - [Timing Configuration](#timing-configuration)
    - [Statistical Reliability](#statistical-reliability)
    - [Output Files](#output-files)
  - [Running Benchmarks](#running-benchmarks)
  - [Benchmark Results](#benchmark-results)
- [Development](#development)
  - [Development Setup](#development-setup)
  - [Code Structure](#code-structure)
    - [Key Design Patterns](#key-design-patterns)
  - [Testing](#testing)
    - [Unit Tests](#unit-tests)
    - [Integration Tests](#integration-tests)
    - [Test Coverage](#test-coverage)
  - [Contributing](#contributing)
    - [Code Style](#code-style)
    - [Adding Features](#adding-features)
- [Troubleshooting](#troubleshooting)
  - [Common Issues](#common-issues)
    - [Issue: Cannot connect to Kafka](#issue-cannot-connect-to-kafka)
    - [Issue: Docker socket permission denied](#issue-docker-socket-permission-denied)
    - [Issue: Scripts not being processed](#issue-scripts-not-being-processed)
    - [Issue: High memory usage](#issue-high-memory-usage)
    - [Issue: Slow execution times](#issue-slow-execution-times)
  - [Debugging](#debugging)
    - [Enable Verbose Logging](#enable-verbose-logging)
    - [Check Kafka Messages](#check-kafka-messages)
    - [Monitor Docker Containers](#monitor-docker-containers)
    - [Check Resource Usage](#check-resource-usage)
  - [Performance Tuning](#performance-tuning)
    - [Optimize Parallelism](#optimize-parallelism)
    - [Optimize Kafka](#optimize-kafka)
    - [Optimize Docker](#optimize-docker)
    - [Language-Specific Optimizations](#language-specific-optimizations)
    - [Horizontal Scaling](#horizontal-scaling)
- [License](#license)

---

## Introduction

### What is SCRC?

SCRC (System for Compiling and Running Code) is a production-ready worker service designed for secure, isolated code execution. It consumes program submissions from Kafka, enforces resource limits, executes programs inside Docker containers, and reports results back to Kafka. The service is built with extensibility in mind, allowing new programming languages to be added over time while maintaining strict isolation and security boundaries.

SCRC follows a hexagonal architecture pattern, separating domain logic from infrastructure concerns, making it easy to test, maintain, and extend. The system is designed to handle high-throughput workloads with configurable parallelism and horizontal scaling capabilities.

### Use Cases

- **Online Judge Systems**: Power competitive programming platforms that need to evaluate code submissions safely and efficiently
- **Code Evaluation Services**: Provide secure code execution for educational platforms, coding interview platforms, or automated code review systems
- **Sandboxed Execution**: Run untrusted code in isolated environments with strict resource limits
- **Multi-language Support**: Support multiple programming languages with a unified execution interface
- **High-throughput Processing**: Handle large volumes of code submissions with horizontal scaling

### Features

- **Multi-language Support**: Currently supports Python, Go, C, C++, and Java with an extensible architecture for adding more languages
- **Container Isolation**: All code execution happens in ephemeral Docker containers for complete isolation
- **Resource Limits**: Configurable time and memory limits per execution
- **Test Case Execution**: Support for multiple test cases per submission with detailed per-test results
- **Horizontal Scaling**: Kafka-based architecture enables horizontal scaling across multiple runner instances
- **Build/Run Separation**: Compiled languages use separate build and run images for optimal performance
- **Comprehensive Benchmarking**: Built-in benchmarking suite for performance analysis
- **Production Ready**: Includes integration tests, error handling, and graceful shutdown

### Prerequisites

- **Go 1.25.3 or later**: Required for building and running the service
- **Docker**: Required for containerized execution (Docker daemon must be accessible)
- **Kafka**: Required for message queue (can use Docker Compose setup for local development)
- **Python 3.x**: Optional, required only for running the simulator and benchmark suite

For local development:

- Docker Desktop or Docker Engine with socket access
- Kafka cluster (or use the provided Docker Compose setup)

---

## Quick Start

### Installation

Clone the repository:

```bash
git clone <repository-url>
cd scrc
```

### Running Locally

Run the Go program directly (requires Kafka to be running):

```bash
go run ./cmd/scrc
```

The service will connect to Kafka at `kafka:9092` by default. Ensure Kafka is accessible or configure `KAFKA_BROKERS` environment variable.

### Running with Docker

Build the image and run the container:

```bash
docker build -t scrc .
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  -e KAFKA_BROKERS=localhost:9092 \
  scrc
```

The container must have access to the Docker socket to create execution containers.

### Quick Test with Docker Compose

Start a complete demo environment with Kafka and a single runner:

```bash
docker compose --profile single-runner --profile load-generator up --build
```

This starts Kafka, ZooKeeper, the runner service, and a simulator that generates test submissions.

---

## Architecture

### System Overview

SCRC follows a hexagonal (ports and adapters) architecture pattern, separating the core domain logic from infrastructure concerns. The system consists of:

1. **Domain Layer** (`internal/domain/execution`): Core business entities (Script, Result, Status, Limits, TestCase)
2. **Application Layer** (`internal/app/executor`): Orchestration logic that coordinates script execution
3. **Infrastructure Layer** (`internal/infra/kafka`): Kafka adapters for consuming scripts and publishing results
4. **Runtime Layer** (`internal/runtime`): Language-agnostic execution engine with Docker implementation
5. **Ports** (`internal/ports`): Interfaces that define contracts between layers

### Component Diagram

```plaintext
┌──────────────────────────────────────────────────────────────┐
│                        Kafka Topics                          │
│  ┌──────────────┐                    ┌──────────────┐        │
│  │   scripts    │                    │script-results│        │
│  └──────┬───────┘                    └────────▲─────┘        │
└─────────┼─────────────────────────────────────┼──────────────┘
          │                                     │
          │                                     │
┌─────────▼─────────────────────────────────────┴──────────────┐
│                    SCRC Runner Service                       │
│  ┌───────────────────────────────────────────────────────┐   │
│  │              Kafka Consumer (Adapter)                 │   │
│  └──────────────────────┬────────────────────────────────┘   │
│                         │                                    │
│  ┌──────────────────────▼────────────────────────────────┐   │
│  │         Executor Service (Application)                │   │
│  │  - Manages concurrency (semaphore-based)              │   │
│  │  - Coordinates script execution                       │   │
│  │  - Aggregates test results                            │   │
│  └──────────────────────┬────────────────────────────────┘   │
│                         │                                    │
│  ┌──────────────────────▼────────────────────────────────┐   │
│  │         Runtime Engine (Domain Interface)             │   │
│  │  - Language-agnostic execution interface              │   │
│  │  - Registry for language modules                      │   │
│  └──────────────────────┬────────────────────────────────┘   │
│                         │                                    │
│  ┌──────────────────────▼────────────────────────────────┐   │
│  │      Docker Runtime (Infrastructure)                  │   │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐       │   │
│  │  │  Python    │  │    Go      │  │     C      │  ...  │   │
│  │  │  Module    │  │  Module    │  │  Module    │       │   │
│  │  └────────────┘  └────────────┘  └────────────┘       │   │
│  └──────────────────────┬────────────────────────────────┘   │
│                         │                                    │
│  ┌──────────────────────▼────────────────────────────────┐   │
│  │              Docker Engine                            │   │
│  │  - Container creation and management                  │   │
│  │  - Resource limit enforcement                         │   │
│  │  - Ephemeral container lifecycle                      │   │
│  └───────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌───────────────────────────────────────────────────────┐   │
│  │         Kafka Publisher (Adapter)                     │   │
│  └──────────────────────┬────────────────────────────────┘   │
└─────────────────────────┼────────────────────────────────────┘
                          │
                          ▼
                  Results Published
```

### Data Flow

1. **Script Submission**: External producer publishes a script message to the `scripts` Kafka topic
2. **Consumption**: Kafka consumer reads the message and deserializes it into a `Script` domain object
3. **Preparation**: Executor service calls the runtime engine's `Prepare` method:
   - Runtime registry selects the appropriate language module
   - For compiled languages: Module creates a build container, compiles the source, and extracts the binary
   - For interpreted languages: Module prepares the source code for direct execution
4. **Execution**: For each test case (or single execution if no tests):
   - A run container is created with the prepared artifact
   - Resource limits (time, memory) are enforced via Docker
   - Program executes with test input provided via stdin
   - Output is captured (stdout, stderr, exit code, duration)
   - Container is destroyed
5. **Result Aggregation**: Test results are aggregated into a `RunReport`
6. **Publishing**: Run report is serialized and published to the `script-results` Kafka topic

### Concurrency Model

The executor service uses a semaphore-based concurrency control mechanism:

- **Bounded Parallelism**: The `RUNNER_MAX_PARALLEL` environment variable controls the maximum number of scripts that can be processed concurrently
- **Per-script Concurrency**: Each script can have multiple test cases, which are executed sequentially within that script
- **Horizontal Scaling**: Multiple runner instances can share the same Kafka consumer group, with Kafka distributing partitions across instances
- **Graceful Shutdown**: The service waits for in-flight executions to complete before shutting down

The concurrency model ensures:

- Resource limits are respected (CPU, memory, Docker containers)
- No single script can monopolize resources
- System can scale horizontally by adding more runner instances
- Kafka partition distribution ensures load balancing

### Codebase Structure

```plaintext
cmd/
  scrc/             # Application entry point and configuration wiring
internal/
  app/
    executor/       # Orchestration service pulling scripts and running suites
  domain/
    execution/      # Core domain types (scripts, limits, results, statuses)
  infra/
    kafka/          # Kafka adapters for script consumption and result publishing
  runtime/
    docker/         # Docker-based language modules and container orchestration
    interfaces.go   # Runtime engine interfaces and registry
  ports/            # Hexagonal interfaces exposed to other layers
integration/        # End-to-end tests (requires Docker & Kafka via Testcontainers)
simulator/          # Python-based testing and benchmarking tools
```

- **`internal/runtime`** hosts the runtime engine abstraction plus the Docker implementation. Language modules live under `docker/lang_*` and are responsible for preparing and running programs in their language.
- **`internal/app/executor`** coordinates pulling submissions, managing concurrency, and producing aggregated run reports using the runtime engine.
- **`internal/infra/kafka`** defines the Kafka consumer/publisher and shared message envelopes used to communicate with external systems.
- **`cmd/scrc`** wires configuration from environment variables, constructs the runtime registry, and starts the executor loop.

---

## Configuration

### Environment Variables Reference

SCRC is configured entirely through environment variables. All variables have sensible defaults for local development.

#### Kafka Configuration

| Variable              | Description                                    | Default          | Example                         |
| --------------------- | ---------------------------------------------- | ---------------- | ------------------------------- |
| `KAFKA_BROKERS`       | Comma-separated list of Kafka broker addresses | `kafka:9092`     | `localhost:9092,localhost:9093` |
| `KAFKA_TOPIC`         | Kafka topic name for consuming scripts         | `scripts`        | `code-submissions`              |
| `KAFKA_GROUP_ID`      | Kafka consumer group ID                        | `scrc-runner`    | `runner-group-1`                |
| `KAFKA_RESULTS_TOPIC` | Kafka topic name for publishing results        | `script-results` | `execution-results`             |

#### Runtime Configuration

| Variable              | Description                                                         | Default        | Example             |
| --------------------- | ------------------------------------------------------------------- | -------------- | ------------------- |
| `RUNNER_MAX_PARALLEL` | Maximum number of scripts to process concurrently                   | `1`            | `8`                 |
| `RUNNER_TIME_LIMIT`   | Default time limit for script execution (duration string)           | `0` (no limit) | `5s`, `2m30s`       |
| `RUNNER_MEMORY_LIMIT` | Default memory limit in bytes                                       | `0` (no limit) | `134217728` (128MB) |
| `SCRIPT_EXPECTED`     | Maximum number of scripts to process before exiting (0 = unlimited) | `0`            | `100`               |

#### Language Image Configuration

Each language can be configured with custom Docker images and working directories:

| Variable         | Description                                    | Default                         |
| ---------------- | ---------------------------------------------- | ------------------------------- |
| `PYTHON_IMAGE`   | Python Docker image for execution              | `python:3.12-alpine`            |
| `PYTHON_WORKDIR` | Working directory in Python containers         | `/tmp`                          |
| `GO_IMAGE`       | Go Docker image for building                   | `golang:1.22-alpine`            |
| `GO_RUN_IMAGE`   | Go Docker image for running compiled binaries  | `alpine:3.20`                   |
| `GO_WORKDIR`     | Working directory in Go containers             | `/tmp`                          |
| `C_IMAGE`        | C Docker image for building                    | `gcc:14`                        |
| `C_RUN_IMAGE`    | C Docker image for running compiled binaries   | `alpine:3.20`                   |
| `C_WORKDIR`      | Working directory in C containers              | `/tmp`                          |
| `CPP_IMAGE`      | C++ Docker image for building                  | `gcc:14`                        |
| `CPP_RUN_IMAGE`  | C++ Docker image for running compiled binaries | `alpine:3.20`                   |
| `CPP_WORKDIR`    | Working directory in C++ containers            | `/tmp`                          |
| `JAVA_IMAGE`     | Java Docker image for building                 | `eclipse-temurin:21-jdk-alpine` |
| `JAVA_RUN_IMAGE` | Java Docker image for running compiled classes | `eclipse-temurin:21-jre-alpine` |
| `JAVA_WORKDIR`   | Working directory in Java containers           | `/tmp`                          |

#### Docker Build Configuration

| Variable     | Description                                        | Default  |
| ------------ | -------------------------------------------------- | -------- |
| `GO_VERSION` | Go version for building the SCRC image (build arg) | `1.25.3` |

### Default Values

When environment variables are not set, SCRC uses the following defaults:

- **Kafka**: Connects to `kafka:9092`, consumes from `scripts` topic, publishes to `script-results` topic
- **Concurrency**: Processes 1 script at a time (set `RUNNER_MAX_PARALLEL` for parallelism)
- **Resource Limits**: No default limits (scripts can specify limits in their message)
- **Language Images**: Uses standard official images (see table above)

### Configuration Examples

#### High-Throughput Configuration

```bash
export RUNNER_MAX_PARALLEL=12
export KAFKA_BROKERS=kafka1:9092,kafka2:9092,kafka3:9092
export KAFKA_GROUP_ID=high-throughput-runners
```

#### Resource-Limited Environment

```bash
export RUNNER_MAX_PARALLEL=4
export RUNNER_TIME_LIMIT=10s
export RUNNER_MEMORY_LIMIT=268435456  # 256MB
```

#### Custom Language Images

```bash
export PYTHON_IMAGE=python:3.11-slim
export GO_IMAGE=golang:1.21-alpine
export GO_RUN_IMAGE=debian:bookworm-slim
```

#### Development Configuration

```bash
export KAFKA_BROKERS=localhost:9092
export RUNNER_MAX_PARALLEL=2
export SCRIPT_EXPECTED=10  # Process 10 scripts then exit
```

---

## Message Formats

### Script Message Format

Scripts are published to Kafka as JSON messages. The message envelope structure is:

```json
{
  "type": "script",
  "id": "unique-submission-id",
  "language": "python",
  "source": "print('Hello, World!')",
  "limits": {
    "time_limit_ms": 5000,
    "memory_limit_bytes": 134217728
  },
  "tests": [
    {
      "number": 1,
      "input": "test input",
      "expected_output": "expected output"
    }
  ]
}
```

#### Field Descriptions

| Field                       | Type    | Required | Description                                                                         |
| --------------------------- | ------- | -------- | ----------------------------------------------------------------------------------- |
| `type`                      | string  | No       | Message type, defaults to `"script"`. Use `"done"` to signal end of stream.         |
| `id`                        | string  | No       | Unique identifier for the submission. If omitted, uses Kafka message key or offset. |
| `language`                  | string  | Yes      | Programming language: `python`, `go`, `c`, `cpp`, or `java`                         |
| `source`                    | string  | Yes      | Source code of the program to execute                                               |
| `limits`                    | object  | No       | Resource limits for execution                                                       |
| `limits.time_limit_ms`      | integer | No       | Maximum execution time in milliseconds (0 = no limit)                               |
| `limits.memory_limit_bytes` | integer | No       | Maximum memory usage in bytes (0 = no limit)                                        |
| `tests`                     | array   | No       | Array of test cases to execute                                                      |
| `tests[].number`            | integer | No       | Test case number (auto-assigned if omitted)                                         |
| `tests[].input`             | string  | No       | Input to provide via stdin                                                          |
| `tests[].expected_output`   | string  | No       | Expected output for validation                                                      |

#### Validation Rules

- `source` must be non-empty
- `language` must be one of the supported languages
- `time_limit_ms` must be non-negative
- `memory_limit_bytes` must be non-negative
- Test case numbers are auto-assigned starting from 1 if not provided

### Result Message Format

Results are published to Kafka as JSON messages with the following structure:

```json
{
  "id": "unique-submission-id",
  "status": "OK",
  "exit_code": 0,
  "stdout": "Hello, World!\n",
  "stderr": "",
  "duration_ms": 1234,
  "error": "",
  "tests": [
    {
      "number": 1,
      "status": "OK",
      "exit_code": 0,
      "duration_ms": 1234,
      "stdout": "Hello, World!\n",
      "stderr": "",
      "input": "test input",
      "expected_output": "expected output",
      "error": ""
    }
  ],
  "timestamp": "2025-01-15T10:30:00Z"
}
```

#### Field Descriptions

| Field                     | Type               | Description                                          |
| ------------------------- | ------------------ | ---------------------------------------------------- |
| `id`                      | string             | Submission ID (matches script message ID)            |
| `status`                  | string             | Overall status: `OK`, `WA`, `TL`, `ML`, `BF`, or `-` |
| `exit_code`               | integer (nullable) | Process exit code                                    |
| `stdout`                  | string             | Standard output (aggregated if multiple tests)       |
| `stderr`                  | string             | Standard error (aggregated if multiple tests)        |
| `duration_ms`             | integer (nullable) | Total execution duration in milliseconds             |
| `error`                   | string             | Error message if execution failed                    |
| `tests`                   | array              | Per-test results (present if script had test cases)  |
| `tests[].number`          | integer            | Test case number                                     |
| `tests[].status`          | string             | Test case status                                     |
| `tests[].exit_code`       | integer (nullable) | Test case exit code                                  |
| `tests[].duration_ms`     | integer (nullable) | Test case duration in milliseconds                   |
| `tests[].stdout`          | string             | Test case standard output                            |
| `tests[].stderr`          | string             | Test case standard error                             |
| `tests[].input`           | string             | Test case input                                      |
| `tests[].expected_output` | string             | Test case expected output                            |
| `tests[].error`           | string             | Test case error message                              |
| `timestamp`               | string (ISO 8601)  | Result publication timestamp                         |

#### Status Codes

- `OK`: Script executed successfully and produced expected output (if tests provided)
- `WA` (Wrong Answer): Script executed but output didn't match expected output
- `TL` (Time Limit): Script exceeded the time limit
- `ML` (Memory Limit): Script exceeded the memory limit
- `BF` (Build Failure): Script failed to compile/build
- `-` (Not Run): Test case was not executed (e.g., due to previous failure)

### Examples

#### Minimal Script Message

```json
{
  "language": "python",
  "source": "print('Hello, World!')"
}
```

#### Script with Resource Limits

```json
{
  "id": "submission-123",
  "language": "go",
  "source": "package main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}",
  "limits": {
    "time_limit_ms": 2000,
    "memory_limit_bytes": 67108864
  }
}
```

#### Script with Test Cases

```json
{
  "id": "submission-456",
  "language": "c",
  "source": "#include <stdio.h>\nint main() { int a, b; scanf(\"%d %d\", &a, &b); printf(\"%d\\n\", a + b); return 0; }",
  "tests": [
    {
      "number": 1,
      "input": "2 3\n",
      "expected_output": "5\n"
    },
    {
      "number": 2,
      "input": "10 20\n",
      "expected_output": "30\n"
    }
  ]
}
```

#### Result Message Example

```json
{
  "id": "submission-456",
  "status": "OK",
  "exit_code": 0,
  "stdout": "",
  "stderr": "",
  "duration_ms": 145,
  "error": "",
  "tests": [
    {
      "number": 1,
      "status": "OK",
      "exit_code": 0,
      "duration_ms": 45,
      "stdout": "5\n",
      "stderr": "",
      "input": "2 3\n",
      "expected_output": "5\n",
      "error": ""
    },
    {
      "number": 2,
      "status": "OK",
      "exit_code": 0,
      "duration_ms": 50,
      "stdout": "30\n",
      "stderr": "",
      "input": "10 20\n",
      "expected_output": "30\n",
      "error": ""
    }
  ],
  "timestamp": "2025-01-15T10:30:00.123Z"
}
```

---

## Language Support

### Supported Languages

SCRC currently supports the following programming languages:

| Language   | Type        | Build Image                     | Run Image                       | Notes                           |
| ---------- | ----------- | ------------------------------- | ------------------------------- | ------------------------------- |
| **Python** | Interpreted | N/A                             | `python:3.12-alpine`            | Direct execution, no build step |
| **Go**     | Compiled    | `golang:1.22-alpine`            | `alpine:3.20`                   | Static binary, CGO disabled     |
| **C**      | Compiled    | `gcc:14`                        | `alpine:3.20`                   | Statically linked binary        |
| **C++**    | Compiled    | `gcc:14`                        | `alpine:3.20`                   | Statically linked binary        |
| **Java**   | Compiled    | `eclipse-temurin:21-jdk-alpine` | `eclipse-temurin:21-jre-alpine` | JVM-based execution             |

#### Language-Specific Behaviors

- **Python**: Source code is written to a file and executed directly with the Python interpreter
- **Go**: Source code is compiled in a build container, binary is extracted and executed in a minimal Alpine container
- **C/C++**: Source code is compiled with static linking (`-static` flag) to enable execution on minimal Alpine images
- **Java**: Source code is compiled to `.class` files in a JDK container, then executed in a JRE container

### Adding New Languages

To add support for a new programming language:

1. **Create a Language Module**: Implement the `runtime.Module` interface in `internal/runtime/docker/lang_<language>.go`

2. **Implement the Strategy Pattern**: Create a strategy struct that implements the `prepareStrategy` interface:

   ```go
   type prepareStrategy interface {
       Prepare(ctx context.Context, lang *languageRuntime, script execution.Script) (runtimex.PreparedScript, *execution.Result, error)
       Close() error
   }
   ```

3. **Implement PreparedScript**: Create a struct that implements `runtime.PreparedScript`:

   ```go
   type PreparedScript interface {
       Run(ctx context.Context, stdin string) (*execution.Result, error)
       Close() error
   }
   ```

4. **Register the Module**: Add the language module to the registry in `internal/runtime/docker/engine.go`:

   ```go
   modules = append(modules, newLanguageModule(cfg, execution.LanguageYourLang))
   ```

5. **Add Configuration**: Add language configuration to `cmd/scrc/config.go`:

   ```go
   execution.LanguageYourLang: {
       Image:    envOrDefault("YOURLANG_IMAGE", defaultImage),
       RunImage: envOrDefault("YOURLANG_RUN_IMAGE", defaultRunImage),
       Workdir:  envOrDefault("YOURLANG_WORKDIR", containerWorkdir),
   },
   ```

6. **Add Language Constant**: Add the language constant to `internal/domain/execution/script.go`:

   ```go
   LanguageYourLang Language = "yourlang"
   ```

7. **Update Message Validation**: Ensure the language string is accepted in `internal/infra/kafka/messages.go`

#### Example: Adding Rust Support

Here's a simplified example structure for adding Rust:

```go
// internal/runtime/docker/lang_rust.go
package docker

type rustStrategy struct{}

func (r *rustStrategy) Prepare(ctx context.Context, lang *languageRuntime, script execution.Script) (runtimex.PreparedScript, *execution.Result, error) {
    // Build the Rust program
    // Extract the binary
    // Return prepared script
}

type rustPreparedScript struct {
    runtime *languageRuntime
    binary  []byte
}

func (r *rustPreparedScript) Run(ctx context.Context, stdin string) (*execution.Result, error) {
    // Execute the binary in a run container
}
```

### Language-Specific Configuration

Each language can be customized via environment variables:

#### Python Configuration

```bash
export PYTHON_IMAGE=python:3.11-slim
export PYTHON_WORKDIR=/app
```

#### Go Configuration

```bash
export GO_IMAGE=golang:1.21-alpine
export GO_RUN_IMAGE=alpine:3.19
export GO_WORKDIR=/tmp
```

#### C/C++ Configuration

```bash
export C_IMAGE=gcc:13
export C_RUN_IMAGE=debian:bookworm-slim
export CPP_IMAGE=gcc:13
export CPP_RUN_IMAGE=debian:bookworm-slim
```

#### Java Configuration

```bash
export JAVA_IMAGE=eclipse-temurin:20-jdk-alpine
export JAVA_RUN_IMAGE=eclipse-temurin:20-jre-alpine
export JAVA_WORKDIR=/app
```

**Note**: For compiled languages (C, C++, Go), binaries are statically linked or built without CGO to enable execution on minimal run images. This significantly reduces container startup time and resource usage.

---

## Running the Service

### Local Execution

Run the service directly with Go:

```bash
go run ./cmd/scrc
```

Ensure Kafka is accessible at the configured broker address (default: `kafka:9092`). For local development, you can use Docker Compose to start Kafka:

```bash
docker compose --profile single-runner up kafka zookeeper
```

Then in another terminal:

```bash
export KAFKA_BROKERS=localhost:9092
go run ./cmd/scrc
```

### Docker Deployment

Build and run the Docker image:

```bash
# Build the image
docker build -t scrc .

# Run with Docker socket access
docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e KAFKA_BROKERS=your-kafka:9092 \
  -e RUNNER_MAX_PARALLEL=8 \
  scrc
```

**Important**: The container must have access to the Docker socket (`/var/run/docker.sock`) to create execution containers.

#### Custom Go Version

Override the Go version used during build:

```bash
docker build --build-arg GO_VERSION=1.26.0 -t scrc .
```

### Docker Compose Setups

The project includes several Docker Compose profiles for different scenarios:

#### Single Runner (Simulator Demo)

Starts Kafka, ZooKeeper, a single runner, and a load generator:

```bash
docker compose --profile single-runner --profile load-generator up --build
```

This setup is ideal for:

- Local development and testing
- Understanding the system workflow
- Testing with a controlled load

The runner processes scripts until stopped. Limit execution with:

```bash
docker compose --profile single-runner --profile load-generator up --build \
  -e SCRIPT_EXPECTED=100
```

#### Horizontal Scaling Demo

Scale to multiple runner instances:

```bash
docker compose --profile multi-runner --profile load-generator up --build --scale scrc=3
```

This demonstrates:

- Horizontal scaling across multiple instances
- Kafka partition distribution
- Load balancing via consumer groups

Each runner shares the same consumer group (`scrc-runner`), so Kafka distributes partitions across instances, ensuring each submission is handled exactly once.

**Tip**: Increase the partition count of the `scripts` topic for higher parallelism:

```bash
kafka-topics --alter --topic scripts --partitions 6 --bootstrap-server localhost:9092
```

#### Multi-Runner Without Simulator

Run multiple runners without the load generator:

```bash
docker compose --profile multi-runner up --build --scale scrc=3
```

Point your own producer at Kafka to drive workloads.

### Production Deployment

#### Prerequisites

- Kafka cluster with appropriate replication and partition configuration
- Docker daemon accessible to runner instances
- Monitoring and logging infrastructure
- Resource limits configured appropriately

#### Deployment Considerations

1. **Kafka Configuration**:
   - Configure appropriate replication factors for production
   - Set partition counts based on expected parallelism
   - Configure retention policies for scripts and results topics
   - Enable compression for message efficiency

2. **Scaling Strategy**:
   - Start with `RUNNER_MAX_PARALLEL` set to 2-4x CPU cores per instance
   - Scale horizontally by adding more runner instances
   - Monitor CPU, memory, and Docker container usage
   - Adjust parallelism based on resource utilization

3. **Resource Limits**:
   - Set default `RUNNER_TIME_LIMIT` and `RUNNER_MEMORY_LIMIT` to prevent resource exhaustion
   - Consider per-language limits if needed
   - Monitor for scripts that consistently hit limits

4. **Monitoring**:
   - Monitor Kafka consumer lag
   - Track execution success/failure rates
   - Monitor Docker container creation/destruction rates
   - Alert on resource exhaustion or error spikes

5. **Security**:
   - Run runners with non-root users where possible
   - Use Docker security options (read-only filesystems, no-new-privileges)
   - Isolate runner network access
   - Implement rate limiting at the Kafka level

6. **High Availability**:
   - Deploy multiple runner instances across availability zones
   - Use Kafka consumer groups for automatic failover
   - Implement health checks and automatic restart

#### Example Production Configuration

```bash
# High-throughput production setup
export KAFKA_BROKERS=kafka1:9092,kafka2:9092,kafka3:9092
export KAFKA_GROUP_ID=production-runners
export RUNNER_MAX_PARALLEL=8
export RUNNER_TIME_LIMIT=30s
export RUNNER_MEMORY_LIMIT=536870912  # 512MB
```

#### Resource Requirements

- **CPU**: 2-4 cores per runner instance (depending on parallelism)
- **Memory**: 512MB-2GB per runner instance (plus Docker overhead)
- **Disk**: Minimal (ephemeral containers), but ensure Docker has sufficient space
- **Network**: Low latency connection to Kafka cluster

---

## Simulator

### Simulator Overview

The simulator is a Python-based tool that generates test submissions and validates results. It's useful for:

- Testing the runner service
- Generating load for performance testing
- Validating end-to-end workflows
- Development and debugging

The simulator consists of:

- **Producer**: Publishes script submissions to Kafka
- **Consumer**: Consumes results from Kafka and validates them
- **Script Library**: Pre-written scripts in `simulator/scripts/<language>/<outcome>` that produce specific outcomes (OK, WA, TL, ML, BF)

### Simulator Usage

Run the simulator standalone:

```bash
python -m simulator.main
```

Or use Docker Compose:

```bash
docker compose --profile single-runner --profile load-generator up --build
```

The simulator will:

1. Load scripts from `simulator/scripts/<language>/<outcome>`
2. Publish each language/outcome combination for the configured number of iterations
3. Consume results from the results topic
4. Compare produced vs consumed counts
5. Exit with non-zero status if discrepancies are detected

### Simulator Configuration

| Variable                      | Description                                    | Default          |
| ----------------------------- | ---------------------------------------------- | ---------------- |
| `SCRIPT_ITERATIONS`           | Number of iterations to emit (≤0 for infinite) | `1`              |
| `SCRIPT_INTERVAL_SECONDS`     | Delay between iteration batches                | `1.0`            |
| `RESULT_WAIT_TIMEOUT_SECONDS` | Timeout for waiting for results                | `60`             |
| `RESULT_CONSUMER_LOG_PATH`    | Path to consumer log file                      | `consumer.log`   |
| `KAFKA_BROKER`                | Kafka broker address                           | `localhost:9092` |
| `KAFKA_TOPIC`                 | Scripts topic name                             | `scripts`        |
| `KAFKA_RESULTS_TOPIC`         | Results topic name                             | `script-results` |
| `KAFKA_CONSUMER_GROUP`        | Consumer group ID                              | `scrc-simulator` |
| `CONSUMER_POLL_TIMEOUT_MS`    | Kafka poll timeout in milliseconds             | `1000`           |

#### Example: Extended Test Run

```bash
export SCRIPT_ITERATIONS=10
export SCRIPT_INTERVAL_SECONDS=0.5
export RESULT_WAIT_TIMEOUT_SECONDS=120
python -m simulator.main
```

---

## Benchmarking

### Benchmark Overview

The benchmark suite provides comprehensive performance analysis of the SCRC system. It measures:

- Throughput (submissions per minute)
- Latency (execution time by language and outcome)
- Scaling characteristics (horizontal and vertical)
- Impact of different workload profiles

### Benchmarking Strategy

The benchmark suite employs a rigorous methodology designed to minimize noise and produce statistically reliable results.

#### Multiple Runs with Aggregation

Each benchmark case executes 3 times by default (`num_runs=3`):

- Results from all runs are aggregated to reduce variance
- Throughput metrics are averaged across runs
- Individual run data is preserved in `{case}__run-{N}.csv` files
- Aggregated results are stored in `{case}__aggregated.csv` for analysis

#### Warmup Period

A 60-second warmup period precedes each measurement window:

- Allows the system to reach steady state:
  - Docker image layers are cached
  - JVM warmup completes (for Java)
  - Container pool stabilizes
  - System resources reach equilibrium
- Data collected during warmup is excluded from results

#### Measurement Window

Active measurement occurs for 240-300 seconds (depending on benchmark type):

- Language latency benchmarks use 300s for larger sample sizes
- Throughput benchmarks use 240s for balance between accuracy and time
- Only submissions that start during the measurement window are included
- Submissions that complete during cooldown are still counted if they started during measurement

#### Cooldown Period

A 60-second cooldown period follows each measurement window:

- Allows in-flight submissions to complete naturally
- Prevents premature termination from skewing results
- Data from submissions that start during cooldown is excluded

#### Data Filtering

The collector tracks precise timestamps for the measurement window:

- Results are filtered to exclude:
  - Submissions sent during warmup (`sent_ts < benchmark_start_ts`)
  - Submissions sent during cooldown (`sent_ts >= benchmark_end_ts`)
- This ensures only steady-state measurements are included in analysis

#### Timing Configuration

- Warmup: 60 seconds (allows system stabilization)
- Duration: 240-300 seconds (provides sufficient sample size)
- Cooldown: 60 seconds (allows in-flight work to complete)
- Runs per case: 3 (reduces variance through aggregation)
- Total time per case: ~6.5-7 minutes (including overhead)
- Full suite: ~7-7.5 hours (23 cases × 3 runs)

#### Statistical Reliability

- Multiple runs reduce the impact of transient system conditions
- Longer durations provide larger sample sizes for more accurate statistics
- Filtering eliminates startup/shutdown artifacts
- Aggregated results represent true steady-state performance

#### Output Files

- Individual run CSVs: `{matrix}__{case}__run-{N}.csv` - raw data per run
- Aggregated CSV: `{matrix}__{case}__aggregated.csv` - combined data from all runs
- Charts: Generated from aggregated data for visualization
- Manifest: `benchmark_manifest.json` - summary of all generated artifacts

This strategy ensures benchmarks produce consistent, reliable results that accurately reflect system performance under steady-state conditions.

### Running Benchmarks

1. **Build the runner image** (needed so the harness can launch on-demand runners):

   ```bash
   docker compose build scrc
   ```

2. **Launch the benchmark profile** (this will start Kafka/ZooKeeper plus the harness container; it will terminate automatically once all matrices finish):

   ```bash
   docker compose --profile benchmark up --build scrc-benchmark
   ```

3. **Inspect the outputs** in `logs/benchmarks/`:

   - `single_runner_throughput.png` — throughput vs `RUNNER_MAX_PARALLEL`
   - `scaling_heatmap.png` — throughput heatmap across runner replicas × parallelism
   - `language_latency_heatmap.png` — median latency summary by language & outcome
   - `language_latency_boxplot.png` — detailed latency distributions per language/outcome
   - `timeout_impact.png` — effect of timeout-heavy submissions on throughput
   - `language_throughput.png` — share of throughput per language/outcome
   - `benchmark_manifest.json` — manifest summarizing generated artifacts
   - CSV snapshots for each benchmark case

Use the environment variables documented in `simulator/benchmarks/main.py` to customize broker endpoints, output paths, or override the benchmark plan.

### Benchmark Results

The benchmark suite generates several charts that provide insights into system performance, scalability, and language-specific characteristics:

**Single Runner Throughput** (`single_runner_throughput.png`)
![Single Runner Throughput](logs/benchmarks/single_runner_throughput.png)

A line chart showing throughput (submissions per minute) as `RUNNER_MAX_PARALLEL` increases from 1 to 16 for a single runner instance under medium load. Throughput increases from 140 submissions/min at parallelism 1 to a peak of 228 submissions/min at parallelism 12, representing a 63% improvement. The optimal parallelism range is 8-12, achieving 225-228 submissions/min. Performance degrades at parallelism 16 (214 submissions/min), demonstrating resource contention when parallelism exceeds available CPU cores. The gradual increase from parallelism 1-8 shows effective utilization of available resources, while the plateau and decline at higher parallelism indicates the system has reached its capacity limits for a single runner instance.

**Scaling Heatmap** (`scaling_heatmap.png`)
![Scaling Heatmap](logs/benchmarks/scaling_heatmap.png)

A heatmap visualizing throughput across combinations of runner replicas (1-3) and parallelism values (4, 6, 8, 12). Darker colors indicate higher throughput. The best performance is achieved with 3 replicas × 6 parallelism at 277.5 submissions/min. Increasing to 3 replicas × 8 parallelism yields 264.25 submissions/min (slight decrease), while 3 replicas × 12 parallelism drops to 246.5 submissions/min, demonstrating resource contention when total parallelism (36 tasks) significantly exceeds the 8-core CPU capacity. Horizontal scaling provides substantial benefits: 2 replicas achieve 1.3-1.4x improvement over single replica configurations, and 3 replicas achieve 1.4-1.5x improvement. However, high parallelism (12) causes contention even with multiple replicas, showing that vertical scaling has limits on resource-constrained systems. The sweet spot is 3 replicas with moderate parallelism (6-8), balancing resource utilization without contention.

**Language Latency Heatmap** (`language_latency_heatmap.png`)
![Language Latency Heatmap](logs/benchmarks/language_latency_heatmap.png)

A color-coded grid showing median latency (in seconds) for each language-outcome combination, with darker reds indicating higher latencies. C and C++ demonstrate the fastest performance across all outcomes, with median latencies of 0.8-1.5 seconds due to compiled native code execution. Go shows moderate latency of 1.5-2.5 seconds, including compilation overhead from the build step. Python exhibits higher latency of 2.5-3.5 seconds due to interpretation overhead. Java shows the highest latency at 2.5-4.5 seconds, primarily due to JVM startup and warmup overhead. Time limit (TL) outcomes consistently show the highest latency across all languages (3-5 seconds), as these represent submissions that run until timeout. Memory limit (ML) and build failure (BF) outcomes show the lowest latency (typically under 1 second) as they are detected quickly. The heatmap clearly shows that compiled languages (C/C++) outperform interpreted languages (Python) and JVM-based languages (Java) for all outcome types.

**Language Latency Distribution** (`language_latency_boxplot.png`)
![Language Latency Boxplot](logs/benchmarks/language_latency_boxplot.png)

Box plots showing the full latency distribution for each language-outcome combination. Each box represents the interquartile range (25th to 75th percentile) with a median line, and whiskers extend to show the data range. C and C++ show tight, compact distributions with low variance, indicating highly consistent and predictable performance across submissions. Go exhibits moderate variance with relatively consistent performance for most outcomes, though time limit outcomes show wider distributions. Python displays higher variance with wider interquartile ranges, showing more variable execution times typical of interpreted languages. Java shows the highest variance and widest distributions, reflecting the variable nature of JVM performance including garbage collection pauses and JIT compilation effects. Time limit (TL) outcomes consistently show the widest distributions across all languages, as these submissions run for varying durations before hitting the timeout. Non-TL outcomes (OK, WA, ML, BF) have tighter distributions, indicating more consistent execution patterns. The box plots reveal that while compiled languages achieve lower absolute latencies, they also provide more predictable performance with less variance.

**Timeout Impact** (`timeout_impact.png`)
![Timeout Impact](logs/benchmarks/timeout_impact.png)

A bar chart comparing throughput between a baseline workload (balanced outcome distribution) and a timeout-heavy workload (TL submissions weighted 2.5x higher). The baseline workload achieves 220.75 submissions/min, while the timeout-heavy workload achieves 173.0 submissions/min, representing a 22% reduction in throughput. This demonstrates that timeout-heavy submissions significantly impact overall system throughput, as long-running submissions block resources and reduce the system's capacity to process new submissions. The system handles timeout detection and cleanup efficiently, but the extended execution time of timeout submissions (running until the time limit is reached) consumes resources that could otherwise process multiple shorter submissions. The 22% reduction shows that while the system remains functional under timeout-heavy loads, performance degrades meaningfully, indicating that timeout detection and resource management are working correctly but cannot fully mitigate the impact of long-running submissions on overall throughput.

**Language Throughput Share** (`language_throughput.png`)
![Language Throughput](logs/benchmarks/language_throughput.png)

A stacked bar chart showing the percentage distribution of submissions across languages and outcomes under heavy load, with each bar totaling 100% of submissions. The total system throughput is approximately 347 submissions/min. The distribution reflects the load profile weights: Python (1.2x weight) and Java (1.1x weight) dominate the workload, while Go (1.0x) and C/C++ (0.9x) have lower representation. Each language bar shows colored segments representing the proportion of each outcome type (OK, WA, TL, ML, BF). The outcome distribution shows a realistic mix across all languages, with successful outcomes (OK) typically representing the largest segment, followed by wrong answers (WA), time limits (TL), memory limits (ML), and build failures (BF). The chart reveals that Python and Java submissions comprise the majority of the workload due to their higher weights in the heavy load profile, while compiled languages (C, C++, Go) represent a smaller but still significant portion. The outcome distribution is consistent across languages, indicating that the system handles all languages and outcome types uniformly under load.

---

## Development

### Development Setup

1. **Clone the repository**:

   ```bash
   git clone <repository-url>
   cd scrc
   ```

2. **Install dependencies**:

   ```bash
   go mod download
   ```

3. **Start local infrastructure** (Kafka, ZooKeeper):

   ```bash
   docker compose --profile single-runner up kafka zookeeper
   ```

4. **Run the service**:

   ```bash
   export KAFKA_BROKERS=localhost:9092
   go run ./cmd/scrc
   ```

### Code Structure

The codebase follows Go best practices and hexagonal architecture:

- **`cmd/scrc`**: Application entry point, configuration loading, dependency injection
- **`internal/domain/execution`**: Core domain models (Script, Result, Status, Limits, TestCase)
- **`internal/app/executor`**: Application orchestration logic
- **`internal/infra/kafka`**: Kafka infrastructure adapters
- **`internal/runtime`**: Runtime engine abstraction and Docker implementation
- **`internal/ports`**: Interface definitions (ports in hexagonal architecture)
- **`integration`**: End-to-end integration tests

#### Key Design Patterns

- **Hexagonal Architecture**: Domain logic is isolated from infrastructure
- **Strategy Pattern**: Language modules implement a common interface
- **Registry Pattern**: Language modules are registered in a central registry
- **Adapter Pattern**: Kafka adapters bridge external systems to domain interfaces

### Testing

#### Unit Tests

Run the fast unit test suite:

```bash
go test ./...
```

Run tests for a specific package:

```bash
go test ./internal/runtime/docker
```

#### Integration Tests

Integration tests exercise the full stack with Docker and Kafka:

```bash
go test -tags=integration ./...
```

Integration tests use Testcontainers to spin up ephemeral Kafka and Docker environments. They require:

- Docker daemon accessible
- Network access for pulling container images

#### Test Coverage

Generate coverage report:

```bash
go test -cover ./...
```

Generate HTML coverage report:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Contributing

1. **Fork the repository** and create a feature branch
2. **Write tests** for new functionality
3. **Follow Go conventions**: Use `gofmt`, follow standard project structure
4. **Update documentation** for any new features or configuration options
5. **Run all tests** before submitting:

   ```bash
   go test ./...
   go test -tags=integration ./...
   ```

6. **Submit a pull request** with a clear description of changes

#### Code Style

- Use `gofmt` for formatting
- Follow Go naming conventions
- Add comments for exported functions and types
- Keep functions focused and small
- Prefer composition over inheritance

#### Adding Features

When adding new features:

- Update relevant documentation sections
- Add examples if applicable
- Update configuration reference if new environment variables are added
- Add integration tests for critical paths

---

## Troubleshooting

### Common Issues

#### Issue: Cannot connect to Kafka

**Symptoms**: Service fails to start with connection errors

**Solutions**:

- Verify Kafka is running and accessible: `docker ps | grep kafka`
- Check `KAFKA_BROKERS` environment variable is correct
- Ensure network connectivity to Kafka brokers
- Check Kafka logs for errors: `docker logs kafka`

#### Issue: Docker socket permission denied

**Symptoms**: `permission denied while trying to connect to the Docker daemon socket`

**Solutions**:

- Ensure user has access to Docker socket: `sudo usermod -aG docker $USER` (then log out/in)
- When using Docker, ensure socket is mounted: `-v /var/run/docker.sock:/var/run/docker.sock`
- Check Docker daemon is running: `docker ps`

#### Issue: Scripts not being processed

**Symptoms**: Scripts published to Kafka but no results appear

**Solutions**:

- Check runner logs for errors
- Verify consumer group is correct
- Check Kafka consumer lag: `kafka-consumer-groups --bootstrap-server localhost:9092 --group scrc-runner --describe`
- Ensure runner has sufficient resources (CPU, memory)
- Verify `RUNNER_MAX_PARALLEL` is set appropriately

#### Issue: High memory usage

**Symptoms**: System running out of memory

**Solutions**:

- Reduce `RUNNER_MAX_PARALLEL` to limit concurrent executions
- Set `RUNNER_MEMORY_LIMIT` to cap per-script memory
- Monitor Docker container count: `docker ps -a | wc -l`
- Ensure containers are being cleaned up (check for zombie containers)
- Consider horizontal scaling instead of increasing parallelism

#### Issue: Slow execution times

**Symptoms**: Scripts taking longer than expected

**Solutions**:

- Check Docker image pull times (first run pulls images)
- Verify system resources (CPU, memory, disk I/O)
- Check for Docker daemon issues: `docker info`
- Consider using pre-pulled images or image caching
- Review language-specific optimizations (e.g., JVM warmup for Java)

### Debugging

#### Enable Verbose Logging

The service uses Go's standard `log` package. To enable more verbose logging, you may need to modify the code or use a logging framework.

#### Check Kafka Messages

Inspect messages in Kafka topics:

```bash
# Consume from scripts topic
kafka-console-consumer --bootstrap-server localhost:9092 --topic scripts --from-beginning

# Consume from results topic
kafka-console-consumer --bootstrap-server localhost:9092 --topic script-results --from-beginning
```

#### Monitor Docker Containers

Watch container creation and destruction:

```bash
# Watch container events
docker events

# List running containers
docker ps

# List all containers (including stopped)
docker ps -a

# Inspect a specific container
docker inspect <container-id>
```

#### Check Resource Usage

Monitor system resources:

```bash
# CPU and memory usage
top
# or
htop

# Docker stats
docker stats

# Disk usage
df -h
docker system df
```

### Performance Tuning

#### Optimize Parallelism

- Start with `RUNNER_MAX_PARALLEL = 2 × CPU cores`
- Monitor CPU utilization (target 70-80%)
- Increase gradually and measure throughput
- Watch for diminishing returns or resource contention

#### Optimize Kafka

- Increase partition count for higher parallelism
- Tune Kafka consumer settings (batch size, fetch size)
- Enable compression for message efficiency
- Configure appropriate retention policies

#### Optimize Docker

- Use minimal base images (Alpine Linux)
- Pre-pull frequently used images
- Enable Docker build cache
- Monitor and clean up unused images: `docker image prune`

#### Language-Specific Optimizations

- **Python**: Use slim images, consider PyPy for performance-critical workloads
- **Go**: Pre-compile common dependencies if possible
- **Java**: Tune JVM options for faster startup (consider GraalVM for native images)
- **C/C++**: Use optimized compiler flags in production

#### Horizontal Scaling

- Add more runner instances rather than increasing parallelism per instance
- Ensure Kafka has sufficient partitions (at least one per runner instance)
- Use load balancers or service discovery for Kafka broker addresses
- Monitor consumer group rebalancing

---

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
