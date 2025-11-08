# scrc

## Overview

SCRC (System for Compiling and Running Code) is a worker service that consumes
program submissions from Kafka, enforces resource limits, executes the compiled
programs inside Docker containers, and reports the results back to Kafka. The
service is intended to power online judge / code-evaluation workloads where new
languages can be added over time, and where the execution environment must be
containerized and isolated.

### High-Level Workflow

1. **Kafka consumer** reads script submissions that contain the source code,
   requested language, optional resource limits, and test cases.
2. **Runtime registry** selects the appropriate language module and prepares the
   submission. For compiled languages (e.g. Go, C) this includes a build step; for
   interpreted languages (e.g. Python) it produces a ready-to-run container
   invocation.
3. **Docker runtime** executes the program inside an ephemeral container,
   respecting time and memory limits for each test case.
4. **Result publisher** serializes the outcome (status, stdout/stderr, test case
   details, error messages) and emits it back to Kafka.

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
```

- **`internal/runtime`** hosts the runtime engine abstraction plus the Docker
  implementation. Language modules live under `docker/lang_*` and are
  responsible for preparing and running programs in their language.
- **`internal/app/executor`** coordinates pulling submissions, managing
  concurrency, and producing aggregated run reports using the runtime engine.
- **`internal/infra/kafka`** defines the Kafka consumer/publisher and shared
  message envelopes used to communicate with external systems.
- **`cmd/scrc`** wires configuration from environment variables, constructs
  the runtime registry, and starts the executor loop.

## Running Locally

Run the Go program directly:

```bash
go run ./cmd/scrc
```

## Running with Docker

Build the image and run the container, mounting the host Docker socket so the service can access the Docker daemon:

```bash
docker build -t scrc .
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock scrc
```

To override the Go toolchain used during the image build, specify the `GO_VERSION` build argument (defaults to `1.25.3`):

```bash
docker build --build-arg GO_VERSION=1.26.0 -t scrc .
```

## Running with Docker Compose

```bash
docker compose up --build
```

This spins up a single-node Kafka cluster, a mock producer container that
continuously streams sample scripts to Kafka, and the runner service that
consumes them and executes each script inside Docker. The runner keeps
processing scripts until it is stopped or its container is terminated. You can
optionally limit how many scripts to execute by setting the `SCRIPT_EXPECTED`
environment variable on the runner service.

The producer emits a new script roughly once per second by default; change the
cadence by setting `SCRIPT_INTERVAL_SECONDS` on the producer container.

### Language Runtimes

Each language runtime can be customized by overriding the container image or working
directory via environment variables:

- `PYTHON_IMAGE` / `PYTHON_WORKDIR` (default image `python:3.12-alpine`)
- `GO_IMAGE` / `GO_WORKDIR` (default image `golang:1.22-alpine`)
- `C_IMAGE` / `C_WORKDIR` (default image `gcc:14`)

## Script Payload

Messages published to the scripts topic must include:

- `id`: unique identifier for the submission
- `language`: execution language (e.g. `python`, `go`, `c`)
- `source`: program source code
- `limits` (optional): time and memory limits
- `tests` (optional): input/output expectations

The runner performs a build step first (no-op for Python, compile for Go and C) and
then measures the program's execution separately. Build failures are reported
with the `BF` status code. The sample producer currently emits Python, Go, and C
programs.

## Testing

Run the fast (unit) suite:

```bash
go test ./...
```

Integration tests exercise the Kafka infrastructure and Docker-backed runner.
They require access to a Docker daemon and will spin up ephemeral containers via
Testcontainers:

```bash
go test -tags=integration ./...
```
