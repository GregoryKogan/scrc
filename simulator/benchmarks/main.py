from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable

import pandas as pd

from ..consumer import DEFAULT_LOG_PATH as SIMULATOR_LOG_PATH
from .charts import render_matrix_charts
from .collector import BenchmarkResultCollector
from .config import BenchmarkPlan, default_benchmark_plan
from .docker_control import RunnerConfig, RunnerManager
from .load import BenchmarkLoadGenerator

LOGGER = logging.getLogger("scrc_simulator.benchmark")


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="SCRC Benchmark Harness")
    parser.add_argument(
        "--broker", default=os.environ.get("KAFKA_BROKER", "kafka:9092")
    )
    parser.add_argument(
        "--script-topic", default=os.environ.get("KAFKA_TOPIC", "scripts")
    )
    parser.add_argument(
        "--results-topic",
        default=os.environ.get("KAFKA_RESULTS_TOPIC", "script-results"),
    )
    parser.add_argument(
        "--group-id", default=os.environ.get("KAFKA_CONSUMER_GROUP", "scrc-benchmark")
    )
    parser.add_argument(
        "--runner-image",
        default=os.environ.get("SCRC_RUNNER_IMAGE", "scrc:latest"),
        help="Docker image for starting runner containers",
    )
    parser.add_argument(
        "--base-env",
        default=os.environ.get("SCRC_RUNNER_ENV", "{}"),
        help="JSON encoded dict of base environment variables for runner containers",
    )
    parser.add_argument(
        "--networks",
        default=os.environ.get("SCRC_RUNNER_NETWORKS"),
        help="Comma-separated list of docker network names for runner containers",
    )
    parser.add_argument(
        "--output-dir",
        default=os.environ.get("BENCHMARK_OUTPUT_DIR", "/app/logs/benchmarks"),
        help="Directory to store benchmark artefacts (charts and CSV files)",
    )
    parser.add_argument(
        "--plan-path",
        default=os.environ.get("BENCHMARK_PLAN_PATH"),
        help="Optional JSON file describing a custom benchmark plan",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Only print the planned benchmark cases without executing them",
    )
    parser.add_argument(
        "--log-level",
        default=os.environ.get("BENCHMARK_LOG_LEVEL", "INFO"),
        help="Logging level",
    )
    return parser.parse_args(argv)


def load_plan(path: str | None) -> BenchmarkPlan:
    if not path:
        return default_benchmark_plan()
    raise NotImplementedError("Custom benchmark plans via JSON are not yet supported")


def discover_networks(env_override: str | None) -> list[str]:
    if env_override:
        return [item.strip() for item in env_override.split(",") if item.strip()]

    networks: list[str] = []
    try:
        import docker

        client = docker.from_env()
        container_id = os.environ.get("HOSTNAME")
        if not container_id:
            container_id = Path("/proc/self/cgroup").read_text().split("/")[-1].strip()
        if not container_id:
            raise RuntimeError("Unable to determine container id from cgroup")
        container = client.containers.get(container_id)
        networks = list(
            container.attrs.get("NetworkSettings", {}).get("Networks", {}).keys()
        )
        if networks:
            return networks
    except Exception:
        pass
    project = os.environ.get("COMPOSE_PROJECT_NAME")
    if project:
        return [f"{project}_default"]
    return []


def setup_logging(level: str) -> None:
    logging.basicConfig(
        level=getattr(logging, level.upper(), logging.INFO),
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    )


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    setup_logging(args.log_level)

    plan = load_plan(args.plan_path)
    base_env = json.loads(args.base_env)
    networks = discover_networks(args.networks)
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    LOGGER.info("Benchmark output directory: %s", output_dir)
    LOGGER.info("Runner image: %s", args.runner_image)
    LOGGER.info("Networks: %s", ", ".join(networks) or "<none>")

    if args.dry_run:
        _print_plan(plan)
        return 0

    runner_manager = RunnerManager(
        image=args.runner_image,
        base_environment=base_env,
        network_names=networks,
    )

    suite_results = {}
    for matrix in plan:
        LOGGER.info("Executing benchmark matrix: %s", matrix.label)
        matrix_results: dict[str, pd.DataFrame] = {}
        throughput_summary: dict[str, float] = {}
        for case in matrix.cases:
            if matrix.chart_type in {"line", "heatmap"}:
                case_key = case.scale_key()
            else:
                case_key = case.name
            LOGGER.info(
                "Running case %s (load=%s, replicas=%d, parallel=%d, runs=%d)",
                case.name,
                case.load_profile.name,
                case.runner_replicas,
                case.runner_parallelism,
                case.num_runs,
            )
            with runner_manager.run(
                RunnerConfig(
                    replicas=case.runner_replicas,
                    parallelism=case.runner_parallelism,
                    environment={
                        "KAFKA_BROKERS": args.broker,
                        "KAFKA_TOPIC": args.script_topic,
                        "KAFKA_RESULTS_TOPIC": args.results_topic,
                        "KAFKA_GROUP_ID": args.group_id,
                    },
                )
            ):
                run_dataframes: list[pd.DataFrame] = []
                run_throughputs: list[float] = []

                for run_num in range(1, case.num_runs + 1):
                    LOGGER.info(
                        "  Run %d/%d for case %s",
                        run_num,
                        case.num_runs,
                        case.name,
                    )
                    collector = BenchmarkResultCollector(
                        broker=args.broker,
                        results_topic=args.results_topic,
                        group_id=f"{args.group_id}-{case.name}-run{run_num}",
                        log_path=SIMULATOR_LOG_PATH,
                    )
                    collector.start()
                    load_generator = BenchmarkLoadGenerator(
                        broker=args.broker,
                        topic=args.script_topic,
                        profile=case.load_profile,
                        collector_callback=collector.register_submission,
                    )

                    # Warmup period
                    time.sleep(case.warmup_seconds)

                    # Record benchmark start time
                    benchmark_start_ts = time.time()

                    # Run load generation for the specified duration
                    stats = load_generator.run(case.duration_seconds)

                    # Record benchmark end time
                    benchmark_end_ts = time.time()

                    # Set the measurement window for filtering
                    collector.set_benchmark_window(benchmark_start_ts, benchmark_end_ts)

                    # Cooldown period
                    time.sleep(case.cooldown_seconds)

                    # Wait for pending results with extended timeout
                    collector.wait_for_pending(timeout_s=case.cooldown_seconds + 30.0)
                    collector.stop()

                    # Build dataframe with filtering enabled
                    df = collector.build_dataframe(filter_window=True)
                    run_dataframes.append(df)
                    run_throughputs.append(stats.throughput_per_minute)

                    # Save individual run CSV
                    df_path = (
                        output_dir / f"{matrix.label}__{case.name}__run-{run_num}.csv"
                    )
                    df.to_csv(df_path, index=False)
                    LOGGER.info(
                        "  Saved run %d results to %s (%d rows, throughput %.2f submissions/min)",
                        run_num,
                        df_path,
                        len(df),
                        stats.throughput_per_minute,
                    )

                # Aggregate results across all runs
                if run_dataframes:
                    aggregated_df = pd.concat(run_dataframes, ignore_index=True)
                    avg_throughput = sum(run_throughputs) / len(run_throughputs)

                    matrix_results[case_key] = aggregated_df
                    throughput_summary[case_key] = avg_throughput

                    # Save aggregated CSV
                    aggregated_path = (
                        output_dir / f"{matrix.label}__{case.name}__aggregated.csv"
                    )
                    aggregated_df.to_csv(aggregated_path, index=False)
                    LOGGER.info(
                        "Saved aggregated results to %s (%d rows, avg throughput %.2f submissions/min)",
                        aggregated_path,
                        len(aggregated_df),
                        avg_throughput,
                    )

        chart_path = render_matrix_charts(
            matrix, matrix_results, throughput_summary, output_dir
        )
        suite_results[matrix.label] = {
            "chart": str(chart_path),
            "cases": list(throughput_summary.items()),
        }

    suite_manifest_path = output_dir / "benchmark_manifest.json"
    with open(suite_manifest_path, "w", encoding="utf-8") as f:
        json.dump(suite_results, f, indent=2)
    LOGGER.info("Benchmark manifest written to %s", suite_manifest_path)
    return 0


def _print_plan(plan: BenchmarkPlan) -> None:
    for matrix in plan:
        print(f"Matrix: {matrix.label} ({matrix.description})")
        for case in matrix.cases:
            print(
                f"  - {case.name}: load={case.load_profile.name}, "
                f"replicas={case.runner_replicas}, parallelism={case.runner_parallelism}, "
                f"duration={case.duration_seconds}s warmup={case.warmup_seconds}s "
                f"cooldown={case.cooldown_seconds}s runs={case.num_runs}"
            )


if __name__ == "__main__":
    sys.exit(main())
