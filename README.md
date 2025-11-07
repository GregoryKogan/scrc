# scrc

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
publishes sample Python scripts to Kafka, and the runner service that consumes
them and executes each script inside Docker. The runner stops after processing
the number of scripts specified via the `SCRIPT_EXPECTED` environment variable
(defaults to `2`).
