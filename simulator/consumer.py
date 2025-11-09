from __future__ import annotations

import collections
import json
import logging
import threading
import time
from pathlib import Path
from typing import Optional

import six

sys_modules = __import__("sys").modules
sys_modules.setdefault("kafka.vendor.six", six)
sys_modules.setdefault("kafka.vendor.six.moves", six.moves)

from kafka import KafkaConsumer
from kafka.errors import NoBrokersAvailable

BASE_DIR = Path(__file__).resolve().parent
DEFAULT_LOG_PATH = BASE_DIR / "consumer.log"
POLL_TIMEOUT_MS_DEFAULT = 1_000


class ResultConsumerError(Exception):
    """Raised when the result consumer encounters a fatal condition."""


def configure_logger(log_path: Path) -> logging.Logger:
    log_path.parent.mkdir(parents=True, exist_ok=True)
    logger = logging.getLogger("scrc_simulator.consumer")
    logger.setLevel(logging.INFO)
    logger.propagate = False

    handler = logging.FileHandler(log_path, mode="w", encoding="utf-8")
    formatter = logging.Formatter("%(asctime)s %(levelname)s %(message)s")
    handler.setFormatter(formatter)

    logger.handlers.clear()
    logger.addHandler(handler)
    return logger


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
                raise ResultConsumerError(
                    "failed to connect to Kafka broker within 60 seconds"
                ) from exc

            time.sleep(backoff)
            backoff = min(backoff * 1.5, max_backoff)


def format_result(record_key: Optional[str], payload: dict) -> str:
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


def infer_script_type(script_id: Optional[str]) -> Optional[str]:
    if not script_id:
        return None
    if "-" not in script_id:
        return None
    prefix, _, suffix = script_id.rpartition("-")
    if not prefix or not suffix:
        return None
    return prefix


class ResultConsumerService:
    def __init__(
        self,
        consumer: KafkaConsumer,
        logger: logging.Logger,
        poll_timeout_ms: int,
    ) -> None:
        self._consumer = consumer
        self._logger = logger
        self._poll_timeout_ms = poll_timeout_ms
        self.counters: collections.Counter[str] = collections.Counter()
        self._lock = threading.Lock()

    def consume(self, stop_event: threading.Event | None = None) -> None:
        while True:
            if stop_event is not None and stop_event.is_set():
                break

            records = self._consumer.poll(timeout_ms=self._poll_timeout_ms)
            if not records:
                continue

            for batch in records.values():
                for message in batch:
                    try:
                        summary = format_result(message.key, message.value)
                        self._logger.info(summary)
                        script_type = infer_script_type(message.value.get("id") or message.key)
                        if script_type:
                            with self._lock:
                                self.counters[script_type] += 1
                    except Exception:  # noqa: BLE001
                        self._logger.exception("failed to process message")

    def close(self) -> None:
        self._consumer.close()
        for handler in list(self._logger.handlers):
            handler.close()
            self._logger.removeHandler(handler)

    def snapshot_counters(self) -> dict[str, int]:
        with self._lock:
            return dict(self.counters)


__all__ = [
    "DEFAULT_LOG_PATH",
    "POLL_TIMEOUT_MS_DEFAULT",
    "ResultConsumerError",
    "configure_logger",
    "create_consumer",
    "ResultConsumerService",
]
