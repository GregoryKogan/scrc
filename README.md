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
   submission. For compiled languages (e.g. Go, C, C++, Java) this includes a build step; for
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

Use Docker Compose profiles to switch between the single-runner simulator demo
and a horizontal-scaling demonstration.

### Single Runner (Simulator Demo)

```bash
docker compose --profile single-runner --profile load-generator up --build
```

This spins up a single-node Kafka cluster, the simulator that continuously
publishes sample scripts, and a single runner container that executes each
script inside Docker. The runner keeps processing scripts until it is stopped
or its container is terminated. You can optionally limit how many scripts to
execute by setting the `SCRIPT_EXPECTED` environment variable on the runner.

### Horizontal Scaling Demo

```bash
docker compose --profile multi-runner --profile load-generator up --build --scale scrc=3
```

This profile launches Kafka/ZooKeeper plus as many runner replicas as you request
with `--scale`. Each runner shares the same consumer group so Kafka partitions
are distributed across instances, guaranteeing that every submission is handled
once. Increase the partition count of the `scripts` topic if you need to
demonstrate higher parallelism. The optional `load-generator` profile includes
the simulator so you can feed the runners; omit it and point your own producer at
Kafka to drive workloads manually.

To run the multi-runner setup without the simulator:

```bash
docker compose --profile multi-runner up --build --scale scrc=3
```

The bundled simulator service (`scrc-simulator`) publishes and consumes scripts
in a single process. Adjust its cadence or workload via the environment
variables documented below; by default it emits one full batch of scenarios per
run (`SCRIPT_ITERATIONS=1`).

### Simulator

A Python harness in `simulator/main.py` coordinates both the sample
producer and the result consumer so you can verify that every emitted script
eventually appears on the results topic. The runner:

- Loads language/outcome scripts from `simulator/scripts/<language>/<outcome>`.
- Publishes each combination (`python-ok`, `python-wa`, …, `java-bf`) for the
  configured number of iterations.
- Tails the results topic, persisting detailed logs to `simulator/consumer.log`.
- Compares produced vs consumed counters and reports mismatches before exiting.

Run it with:

```bash
python -m simulator.main
```

Tweak behavior via the existing environment variables:

- `SCRIPT_ITERATIONS` caps producer iterations (defaults to `1` for the combined run).
- `SCRIPT_INTERVAL_SECONDS` adjusts the delay between iteration batches.
- `RESULT_WAIT_TIMEOUT_SECONDS` controls how long to wait for matching results
  (defaults to `60` seconds).
- `RESULT_CONSUMER_LOG_PATH` relocates the log file if needed.

The process exits non-zero if any discrepancy is detected between produced and
consumed scripts, or if the consumer fails to observe all expected results
before timing out.

### Benchmark Suite

The benchmark harness orchestrates randomized simulator loads, sweeps runner
concurrency/replica configurations, and renders presentation-ready charts in
`logs/benchmarks`.

1. Build the runner image (needed so the harness can launch on-demand runners):

   ```bash
   docker compose build scrc
   ```

2. Launch the benchmark profile (this will start Kafka/ZooKeeper plus the harness
   container; it will terminate automatically once all matrices finish):

   ```bash
   docker compose --profile benchmark up --build scrc-benchmark
   ```

3. Inspect the outputs in `logs/benchmarks/`:

   - `single_runner_throughput.png` — throughput vs `RUNNER_MAX_PARALLEL`
   - `scaling_heatmap.png` — throughput heatmap across runner replicas × parallelism
   - `language_latency_heatmap.png` — median latency summary by language & outcome
   - `language_latency_boxplot.png` — detailed latency distributions per language/outcome
   - `timeout_impact.png` — effect of timeout-heavy submissions on throughput
   - `language_throughput.png` — share of throughput per language/outcome
   - `benchmark_manifest.json` — manifest summarizing generated artifacts
   - CSV snapshots for each benchmark case

Use the environment variables documented in `simulator/benchmarks/main.py` to
customize broker endpoints, output paths, or override the benchmark plan.

#### Benchmarking Strategy

The benchmark suite employs a rigorous methodology designed to minimize noise and
produce statistically reliable results.

##### Multiple Runs with Aggregation

Each benchmark case executes 3 times by default (`num_runs=3`):

- Results from all runs are aggregated to reduce variance
- Throughput metrics are averaged across runs
- Individual run data is preserved in `{case}__run-{N}.csv` files
- Aggregated results are stored in `{case}__aggregated.csv` for analysis

##### Warmup Period

A 60-second warmup period precedes each measurement window:

- Allows the system to reach steady state:
  - Docker image layers are cached
  - JVM warmup completes (for Java)
  - Container pool stabilizes
  - System resources reach equilibrium
- Data collected during warmup is excluded from results

##### Measurement Window

Active measurement occurs for 240-300 seconds (depending on benchmark type):

- Language latency benchmarks use 300s for larger sample sizes
- Throughput benchmarks use 240s for balance between accuracy and time
- Only submissions that start during the measurement window are included
- Submissions that complete during cooldown are still counted if they started during measurement

##### Cooldown Period

A 60-second cooldown period follows each measurement window:

- Allows in-flight submissions to complete naturally
- Prevents premature termination from skewing results
- Data from submissions that start during cooldown is excluded

##### Data Filtering

The collector tracks precise timestamps for the measurement window:

- Results are filtered to exclude:
  - Submissions sent during warmup (`sent_ts < benchmark_start_ts`)
  - Submissions sent during cooldown (`sent_ts >= benchmark_end_ts`)
- This ensures only steady-state measurements are included in analysis

##### Timing Configuration

- Warmup: 60 seconds (allows system stabilization)
- Duration: 240-300 seconds (provides sufficient sample size)
- Cooldown: 60 seconds (allows in-flight work to complete)
- Runs per case: 3 (reduces variance through aggregation)
- Total time per case: ~6.5-7 minutes (including overhead)
- Full suite: ~7-7.5 hours (23 cases × 3 runs)

##### Statistical Reliability

- Multiple runs reduce the impact of transient system conditions
- Longer durations provide larger sample sizes for more accurate statistics
- Filtering eliminates startup/shutdown artifacts
- Aggregated results represent true steady-state performance

##### Output Files

- Individual run CSVs: `{matrix}__{case}__run-{N}.csv` - raw data per run
- Aggregated CSV: `{matrix}__{case}__aggregated.csv` - combined data from all runs
- Charts: Generated from aggregated data for visualization
- Manifest: `benchmark_manifest.json` - summary of all generated artifacts

This strategy ensures benchmarks produce consistent, reliable results that accurately
reflect system performance under steady-state conditions.

#### Benchmark Results

The benchmark suite generates several charts that provide insights into system
performance, scalability, and language-specific characteristics:

**Single Runner Throughput** (`single_runner_throughput.png`)
![Single Runner Throughput](logs/benchmarks/single_runner_throughput.png)

A line chart showing throughput (submissions per minute) as `RUNNER_MAX_PARALLEL`
increases from 1 to 16 for a single runner instance under medium load. Throughput
increases from 140 submissions/min at parallelism 1 to a peak of 228 submissions/min
at parallelism 12, representing a 63% improvement. The optimal parallelism range
is 8-12, achieving 225-228 submissions/min. Performance degrades at parallelism 16
(214 submissions/min), demonstrating resource contention when parallelism exceeds
available CPU cores. The gradual increase from parallelism 1-8 shows effective
utilization of available resources, while the plateau and decline at higher
parallelism indicates the system has reached its capacity limits for a single
runner instance.

**Scaling Heatmap** (`scaling_heatmap.png`)
![Scaling Heatmap](logs/benchmarks/scaling_heatmap.png)

A heatmap visualizing throughput across combinations of runner replicas (1-3) and
parallelism values (4, 6, 8, 12). Darker colors indicate higher throughput. The
best performance is achieved with 3 replicas × 6 parallelism at 277.5 submissions/min.
Increasing to 3 replicas × 8 parallelism yields 264.25 submissions/min (slight
decrease), while 3 replicas × 12 parallelism drops to 246.5 submissions/min,
demonstrating resource contention when total parallelism (36 tasks) significantly
exceeds the 8-core CPU capacity. Horizontal scaling provides substantial benefits:
2 replicas achieve 1.3-1.4x improvement over single replica configurations, and
3 replicas achieve 1.4-1.5x improvement. However, high parallelism (12) causes
contention even with multiple replicas, showing that vertical scaling has limits
on resource-constrained systems. The sweet spot is 3 replicas with moderate
parallelism (6-8), balancing resource utilization without contention.

**Language Latency Heatmap** (`language_latency_heatmap.png`)
![Language Latency Heatmap](logs/benchmarks/language_latency_heatmap.png)

A color-coded grid showing median latency (in seconds) for each language-outcome
combination, with darker reds indicating higher latencies. C and C++ demonstrate
the fastest performance across all outcomes, with median latencies of 0.8-1.5
seconds due to compiled native code execution. Go shows moderate latency of
1.5-2.5 seconds, including compilation overhead from the build step. Python
exhibits higher latency of 2.5-3.5 seconds due to interpretation overhead.
Java shows the highest latency at 2.5-4.5 seconds, primarily due to JVM startup
and warmup overhead. Time limit (TL) outcomes consistently show the highest
latency across all languages (3-5 seconds), as these represent submissions
that run until timeout. Memory limit (ML) and build failure (BF) outcomes show
the lowest latency (typically under 1 second) as they are detected quickly.
The heatmap clearly shows that compiled languages (C/C++) outperform interpreted
languages (Python) and JVM-based languages (Java) for all outcome types.

**Language Latency Distribution** (`language_latency_boxplot.png`)
![Language Latency Boxplot](logs/benchmarks/language_latency_boxplot.png)

Box plots showing the full latency distribution for each language-outcome
combination. Each box represents the interquartile range (25th to 75th percentile)
with a median line, and whiskers extend to show the data range. C and C++ show
tight, compact distributions with low variance, indicating highly consistent and
predictable performance across submissions. Go exhibits moderate variance with
relatively consistent performance for most outcomes, though time limit outcomes
show wider distributions. Python displays higher variance with wider interquartile
ranges, showing more variable execution times typical of interpreted languages.
Java shows the highest variance and widest distributions, reflecting the variable
nature of JVM performance including garbage collection pauses and JIT compilation
effects. Time limit (TL) outcomes consistently show the widest distributions
across all languages, as these submissions run for varying durations before
hitting the timeout. Non-TL outcomes (OK, WA, ML, BF) have tighter distributions,
indicating more consistent execution patterns. The box plots reveal that while
compiled languages achieve lower absolute latencies, they also provide more
predictable performance with less variance.

**Timeout Impact** (`timeout_impact.png`)
![Timeout Impact](logs/benchmarks/timeout_impact.png)

A bar chart comparing throughput between a baseline workload (balanced outcome
distribution) and a timeout-heavy workload (TL submissions weighted 2.5x higher).
The baseline workload achieves 220.75 submissions/min, while the timeout-heavy
workload achieves 173.0 submissions/min, representing a 22% reduction in
throughput. This demonstrates that timeout-heavy submissions significantly impact
overall system throughput, as long-running submissions block resources and reduce
the system's capacity to process new submissions. The system handles timeout
detection and cleanup efficiently, but the extended execution time of timeout
submissions (running until the time limit is reached) consumes resources that
could otherwise process multiple shorter submissions. The 22% reduction shows
that while the system remains functional under timeout-heavy loads, performance
degrades meaningfully, indicating that timeout detection and resource management
are working correctly but cannot fully mitigate the impact of long-running
submissions on overall throughput.

**Language Throughput Share** (`language_throughput.png`)
![Language Throughput](logs/benchmarks/language_throughput.png)

A stacked bar chart showing the percentage distribution of submissions across
languages and outcomes under heavy load, with each bar totaling 100% of submissions.
The total system throughput is approximately 347 submissions/min. The distribution
reflects the load profile weights: Python (1.2x weight) and Java (1.1x weight)
dominate the workload, while Go (1.0x) and C/C++ (0.9x) have lower representation.
Each language bar shows colored segments representing the proportion of each outcome
type (OK, WA, TL, ML, BF). The outcome distribution shows a realistic mix across
all languages, with successful outcomes (OK) typically representing the largest
segment, followed by wrong answers (WA), time limits (TL), memory limits (ML), and
build failures (BF). The chart reveals that Python and Java submissions comprise
the majority of the workload due to their higher weights in the heavy load profile,
while compiled languages (C, C++, Go) represent a smaller but still significant
portion. The outcome distribution is consistent across languages, indicating that
the system handles all languages and outcome types uniformly under load.

### Language Runtimes

Each language runtime can be customized by overriding the container image or working
directory via environment variables. Compiled languages (Go, C, C++, Java) use separate
build and run images for optimal performance:

- **Build images** contain the full compiler/toolchain needed for compilation
- **Run images** are minimal and only contain what's needed for execution
- This separation significantly reduces container startup time and resource usage

**Configuration:**

- `PYTHON_IMAGE` / `PYTHON_WORKDIR` (default image `python:3.12-alpine`)
- `GO_IMAGE` / `GO_RUN_IMAGE` / `GO_WORKDIR` (default build image `golang:1.22-alpine`, default run image `alpine:3.20`)
- `C_IMAGE` / `C_RUN_IMAGE` / `C_WORKDIR` (default build image `gcc:14`, default run image `alpine:3.20`)
- `CPP_IMAGE` / `CPP_RUN_IMAGE` / `CPP_WORKDIR` (default build image `gcc:14`, default run image `alpine:3.20`)
- `JAVA_IMAGE` / `JAVA_RUN_IMAGE` / `JAVA_WORKDIR` (default build image `eclipse-temurin:21-jdk-alpine`, default run image `eclipse-temurin:21-jre-alpine`)

**Note:** C and C++ binaries are statically linked (`-static` flag) to enable execution on minimal Alpine images. Go binaries are built with `CGO_ENABLED=0` for the same purpose.

## Script Payload

Messages published to the scripts topic must include:

- `id`: unique identifier for the submission
- `language`: execution language (e.g. `python`, `go`, `c`, `cpp`, `java`)
- `source`: program source code
- `limits` (optional): time and memory limits
- `tests` (optional): input/output expectations

The runner performs a build step first (no-op for Python, compile for Go, C, C++, and Java) and
then measures the program's execution separately. Build failures are reported
with the `BF` status code. The sample producer currently emits Python, Go, C, C++, and Java
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
