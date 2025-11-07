import json
import os
import sys
import time

import six

sys.modules.setdefault("kafka.vendor.six", six)
sys.modules.setdefault("kafka.vendor.six.moves", six.moves)

from kafka import KafkaConsumer
from kafka.errors import NoBrokersAvailable


def create_consumer(broker: str, topic: str, group_id: str) -> KafkaConsumer:
    backoff = 1.0
    max_backoff = 10.0
    deadline = time.time() + 60

    while True:
        try:
            return KafkaConsumer(
                topic,
                bootstrap_servers=broker,
                group_id=group_id,
                value_deserializer=lambda v: json.loads(v.decode("utf-8")),
                key_deserializer=lambda v: v.decode("utf-8") if v else None,
                enable_auto_commit=True,
                auto_offset_reset="latest",
            )
        except NoBrokersAvailable as exc:
            if time.time() >= deadline:
                raise RuntimeError(
                    "failed to connect to Kafka broker within 60 seconds"
                ) from exc

            time.sleep(backoff)
            backoff = min(backoff * 1.5, max_backoff)


def format_result(record_key: str | None, payload: dict) -> str:
    script_id = payload.get("id") or record_key or "<unknown>"
    status = payload.get("status")
    exit_code = payload.get("exit_code")
    duration_ms = payload.get("duration_ms")
    stdout = payload.get("stdout", "").rstrip()
    stderr = payload.get("stderr", "").rstrip()
    error = payload.get("error")
    timestamp = payload.get("timestamp")
    tests = payload.get("tests") or []

    lines = [f"[{timestamp}] script {script_id!r}"]

    if status:
        lines.append(f"  status: {status}")
    if exit_code is not None:
        lines.append(f"  exit_code: {exit_code}")
    if duration_ms is not None:
        lines.append(f"  duration_ms: {duration_ms}")
    if error:
        lines.append(f"  error: {error}")
    if stdout:
        lines.append("  stdout:")
        lines.extend(f"    {line}" for line in stdout.splitlines())
    if stderr:
        lines.append("  stderr:")
        lines.extend(f"    {line}" for line in stderr.splitlines())
    if tests:
        lines.append("  tests:")
        for test in tests:
            number = test.get("number")
            test_status = test.get("status")
            test_exit = test.get("exit_code")
            test_duration = test.get("duration_ms")
            test_input = (test.get("input") or "").rstrip("\n")
            test_expected = (test.get("expected_output") or "").rstrip("\n")
            test_stdout = (test.get("stdout") or "").rstrip("\n")
            test_stderr = (test.get("stderr") or "").rstrip("\n")
            test_error = test.get("error")

            header_parts = []
            if number is not None:
                header_parts.append(f"test {number}")
            else:
                header_parts.append("test")
            if test_status:
                header_parts.append(f"status={test_status}")
            if test_duration is not None:
                header_parts.append(f"duration_ms={test_duration}")
            if test_exit is not None:
                header_parts.append(f"exit_code={test_exit}")

            lines.append(f"    {' '.join(header_parts)}")

            if test_input:
                lines.append(f"      input: {test_input!r}")
            if test_expected:
                lines.append(f"      expected: {test_expected!r}")
            if test_stdout:
                lines.append(f"      stdout: {test_stdout!r}")
            if test_stderr:
                lines.append(f"      stderr: {test_stderr!r}")
            if test_error:
                lines.append(f"      error: {test_error}")

    return "\n".join(lines)


def main() -> None:
    broker = os.environ.get("KAFKA_BROKER", "kafka:9092")
    topic = os.environ.get("KAFKA_RESULTS_TOPIC", "script-results")
    group_id = os.environ.get("KAFKA_CONSUMER_GROUP", "scrc-results-consumer")

    consumer = create_consumer(broker, topic, group_id)

    try:
        for message in consumer:
            try:
                print(format_result(message.key, message.value), flush=True)
            except Exception as exc:  # noqa: BLE001
                print(f"failed to format message: {exc!r}", file=sys.stderr, flush=True)
    except KeyboardInterrupt:
        print("stopping results consumer", file=sys.stderr, flush=True)
    finally:
        consumer.close()


if __name__ == "__main__":
    main()
