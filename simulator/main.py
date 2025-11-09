from __future__ import annotations

import argparse
import collections
import os
import sys
import threading
import time
from pathlib import Path
from typing import Any

from .consumer import (
    DEFAULT_LOG_PATH,
    POLL_TIMEOUT_MS_DEFAULT,
    ResultConsumerError,
    ResultConsumerService,
    configure_logger,
    create_consumer,
)
from .producer import ScriptProducerService, build_scenarios


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run the SCRC simulator")
    parser.add_argument("--broker", help="Kafka bootstrap broker")
    parser.add_argument("--script-topic", help="Kafka topic for script submissions")
    parser.add_argument("--results-topic", help="Kafka topic for script results")
    parser.add_argument("--group-id", help="Kafka consumer group id")
    parser.add_argument(
        "--interval",
        type=float,
        help="Seconds between iterations of the scenario set",
    )
    parser.add_argument(
        "--iterations",
        type=int,
        help="Number of iterations to emit (<=0 for infinite)",
    )
    parser.add_argument(
        "--wait-timeout",
        type=float,
        help="Seconds to wait for the consumer to observe all expected results",
    )
    parser.add_argument(
        "--consumer-poll-timeout",
        type=int,
        help="Kafka poll timeout in milliseconds",
    )
    parser.add_argument(
        "--log-path",
        type=str,
        help="Path to the consumer log file",
    )
    return parser.parse_args(argv)


def expected_counts_from_scenarios(scenarios: list[Any], iterations: int) -> collections.Counter[str]:
    if iterations <= 0:
        # No finite expectation; only return zero counts.
        return collections.Counter()
    counts: collections.Counter[str] = collections.Counter()
    for scenario in scenarios:
        counts[scenario.type] = iterations
    return counts


def wait_for_counts(
    service: ResultConsumerService,
    expected: collections.Counter[str],
    timeout_s: float,
    check_interval: float = 0.5,
) -> bool:
    if not expected:
        return True
    deadline = time.time() + timeout_s
    while True:
        snapshot = service.snapshot_counters()
        if all(
            snapshot.get(script_type, 0) >= expected_count
            for script_type, expected_count in expected.items()
        ):
            return True
        now = time.time()
        if now >= deadline:
            return False
        sleep_for = min(check_interval, max(0.05, deadline - now))
        time.sleep(sleep_for)


def format_counter(counter: collections.Counter[str]) -> str:
    if not counter:
        return "<empty>"
    return ", ".join(f"{script_type}={counter[script_type]}" for script_type in sorted(counter))


def run(argv: list[str] | None = None) -> int:
    args = parse_args(argv or sys.argv[1:])

    env = os.environ

    broker = args.broker or env.get("KAFKA_BROKER", "kafka:9092")
    script_topic = args.script_topic or env.get("KAFKA_TOPIC", "scripts")
    results_topic = args.results_topic or env.get("KAFKA_RESULTS_TOPIC", "script-results")
    group_id = args.group_id or env.get("KAFKA_CONSUMER_GROUP", "scrc-simulator")

    interval = args.interval
    if interval is None:
        interval_str = env.get("SCRIPT_INTERVAL_SECONDS", "1.0")
        try:
            interval = max(float(interval_str), 0.0)
        except ValueError:
            print(
                f"invalid SCRIPT_INTERVAL_SECONDS value {interval_str!r}; defaulting to 1.0",
                file=sys.stderr,
            )
            interval = 1.0

    iterations = args.iterations
    if iterations is None:
        iterations_str = env.get("SCRIPT_ITERATIONS", "1")
        try:
            iterations = int(iterations_str)
        except ValueError:
            print(
                f"invalid SCRIPT_ITERATIONS value {iterations_str!r}; defaulting to 1",
                file=sys.stderr,
            )
            iterations = 1

    wait_timeout = args.wait_timeout
    if wait_timeout is None:
        wait_timeout_str = env.get("RESULT_WAIT_TIMEOUT_SECONDS", "60")
        try:
            wait_timeout = float(wait_timeout_str)
        except ValueError:
            print(
                f"invalid RESULT_WAIT_TIMEOUT_SECONDS value {wait_timeout_str!r}; defaulting to 60",
                file=sys.stderr,
            )
            wait_timeout = 60.0

    consumer_poll_timeout = args.consumer_poll_timeout
    if consumer_poll_timeout is None:
        consumer_poll_timeout_str = env.get("CONSUMER_POLL_TIMEOUT_MS", str(POLL_TIMEOUT_MS_DEFAULT))
        try:
            consumer_poll_timeout = int(consumer_poll_timeout_str)
            if consumer_poll_timeout <= 0:
                raise ValueError("non-positive poll timeout")
        except ValueError:
            print(
                f"invalid CONSUMER_POLL_TIMEOUT_MS value {consumer_poll_timeout_str!r}; defaulting to {POLL_TIMEOUT_MS_DEFAULT}",
                file=sys.stderr,
            )
            consumer_poll_timeout = POLL_TIMEOUT_MS_DEFAULT

    log_path_value = args.log_path or env.get("RESULT_CONSUMER_LOG_PATH")
    log_path = Path(log_path_value) if log_path_value else DEFAULT_LOG_PATH

    scenarios = build_scenarios()
    expected_counts = expected_counts_from_scenarios(scenarios, iterations)

    logger = configure_logger(log_path)

    try:
        consumer = create_consumer(broker, results_topic, group_id)
    except ResultConsumerError:
        logger.exception("failed to initialise Kafka consumer")
        return 1

    consumer_service = ResultConsumerService(consumer, logger, consumer_poll_timeout)
    stop_event = threading.Event()
    consumer_errors: list[BaseException] = []

    def consumer_runner() -> None:
        try:
            consumer_service.consume(stop_event)
        except Exception as exc:  # noqa: BLE001
            consumer_errors.append(exc)
            logger.exception("consumer thread failed")
            stop_event.set()

    consumer_thread = threading.Thread(
        target=consumer_runner,
        name="simulator-consumer",
        daemon=True,
    )
    consumer_thread.start()

    max_iterations = None if iterations <= 0 else iterations
    producer_service = ScriptProducerService(
        broker=broker,
        topic=script_topic,
        interval_s=interval,
        max_iterations=max_iterations,
        scenarios=scenarios,
    )

    try:
        producer_service.run(stop_event)
    except KeyboardInterrupt:
        print("stopping simulator", file=sys.stderr)
        stop_event.set()
    except Exception as exc:  # noqa: BLE001
        print(f"producer encountered error: {exc!r}", file=sys.stderr)
        stop_event.set()

    if stop_event.is_set():
        all_consumed = False
    else:
        all_consumed = wait_for_counts(consumer_service, expected_counts, wait_timeout)
        if not all_consumed:
            logger.warning(
                "consumer did not receive all expected results within %.2f seconds",
                wait_timeout,
            )

    stop_event.set()
    consumer_thread.join(timeout=5.0)
    consumer_service.close()

    producer_counts = collections.Counter(producer_service.counters)
    consumer_counts = collections.Counter(consumer_service.snapshot_counters())

    print("Producer counts:")
    for script_type in sorted(producer_counts):
        print(f"  {script_type}: {producer_counts[script_type]}")

    print("Consumer counts:")
    for script_type in sorted(consumer_counts):
        print(f"  {script_type}: {consumer_counts[script_type]}")

    mismatches = {}
    for script_type, expected_count in expected_counts.items():
        produced = producer_counts.get(script_type, 0)
        consumed = consumer_counts.get(script_type, 0)
        if produced != expected_count or consumed != expected_count:
            mismatches[script_type] = {
                "expected": expected_count,
                "produced": produced,
                "consumed": consumed,
            }

    unexpected_consumed = {
        script_type: count
        for script_type, count in consumer_counts.items()
        if script_type not in expected_counts
    }

    exit_code = 0
    if consumer_errors or mismatches or unexpected_consumed or not all_consumed:
        exit_code = 1
        print("\nSimulator status: MISMATCH", file=sys.stderr)
        if mismatches:
            print("  mismatched counts:", file=sys.stderr)
            for script_type, details in sorted(mismatches.items()):
                print(
                    f"    {script_type}: expected={details['expected']} produced={details['produced']} consumed={details['consumed']}",
                    file=sys.stderr,
                )
        if unexpected_consumed:
            print("  unexpected script types consumed:", file=sys.stderr)
            for script_type, count in sorted(unexpected_consumed.items()):
                print(f"    {script_type}: {count}", file=sys.stderr)
        if consumer_errors:
            print(f"  consumer errors: {len(consumer_errors)} (see log)", file=sys.stderr)
        if not all_consumed:
            print("  consumer timed out waiting for results", file=sys.stderr)
    else:
        print("\nSimulator status: OK", file=sys.stderr)

    return exit_code


def main() -> None:
    sys.exit(run())


if __name__ == "__main__":
    main()
