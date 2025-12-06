from __future__ import annotations

import collections
import threading
import time
from dataclasses import dataclass
from typing import Any

import pandas as pd

from ..consumer import ResultConsumerService, configure_logger, create_consumer
from ..consumer import DEFAULT_LOG_PATH as SIMULATOR_LOG_PATH
from ..consumer import POLL_TIMEOUT_MS_DEFAULT


@dataclass
class SubmissionRecord:
    script_id: str
    language: str
    outcome: str
    sent_ts: float


class BenchmarkResultCollector:
    """Coordinates producer send-time bookkeeping with the consumer snapshots."""

    def __init__(
        self,
        broker: str,
        results_topic: str,
        group_id: str,
        poll_timeout_ms: int = POLL_TIMEOUT_MS_DEFAULT,
        log_path=SIMULATOR_LOG_PATH,
    ) -> None:
        self._broker = broker
        self._topic = results_topic
        self._group_id = group_id
        self._poll_timeout_ms = poll_timeout_ms
        self._log_path = log_path

        self._records_lock = threading.Lock()
        self._pending: dict[str, SubmissionRecord] = {}
        self._completed: list[dict[str, Any]] = []
        self._consumer_thread: threading.Thread | None = None
        self._stop_event = threading.Event()
        self._service: ResultConsumerService | None = None
        self._benchmark_start_ts: float | None = None
        self._benchmark_end_ts: float | None = None

    def start(self) -> None:
        logger = configure_logger(self._log_path)
        consumer = create_consumer(self._broker, self._topic, self._group_id)
        service = ResultConsumerService(
            consumer=consumer,
            logger=logger,
            poll_timeout_ms=self._poll_timeout_ms,
        )
        self._service = service

        def runner() -> None:
            while not self._stop_event.is_set():
                records = consumer.poll(timeout_ms=self._poll_timeout_ms)
                if not records:
                    continue
                for batch in records.values():
                    for message in batch:
                        payload = message.value
                        script_id = payload.get("id") or message.key
                        if not script_id:
                            continue
                        self._handle_result(script_id, payload, time.time())

        thread = threading.Thread(target=runner, name="benchmark-consumer", daemon=True)
        thread.start()
        self._consumer_thread = thread

    def stop(self) -> None:
        self._stop_event.set()
        if self._consumer_thread:
            self._consumer_thread.join(timeout=5.0)

        if self._service:
            self._service.close()

    def register_submission(self, record: SubmissionRecord) -> None:
        with self._records_lock:
            self._pending[record.script_id] = record

    def set_benchmark_window(self, start_ts: float, end_ts: float) -> None:
        """Set the measurement window for filtering warmup and cooldown periods."""
        with self._records_lock:
            self._benchmark_start_ts = start_ts
            self._benchmark_end_ts = end_ts

    def wait_for_pending(self, timeout_s: float = 60.0) -> None:
        deadline = time.time() + timeout_s
        while time.time() < deadline:
            with self._records_lock:
                if not self._pending:
                    return
            time.sleep(0.2)

    def build_dataframe(self, filter_window: bool = True) -> pd.DataFrame:
        with self._records_lock:
            rows = list(self._completed)
        
        if not rows:
            return pd.DataFrame(
                columns=[
                    "script_id",
                    "language",
                    "outcome",
                    "status",
                    "duration_ms",
                    "sent_ts",
                    "completed_ts",
                    "latency_s",
                ]
            )
        
        # Filter out warmup and cooldown periods if window is set
        if filter_window and self._benchmark_start_ts is not None and self._benchmark_end_ts is not None:
            filtered_rows = []
            for row in rows:
                sent_ts = row.get("sent_ts", 0.0)
                # Include results that started during the measurement window
                # (even if they completed during cooldown)
                if self._benchmark_start_ts <= sent_ts < self._benchmark_end_ts:
                    filtered_rows.append(row)
            rows = filtered_rows
        
        return pd.DataFrame(rows)

    def summaries(self) -> dict[str, int]:
        with self._records_lock:
            counter = collections.Counter(record["status"] for record in self._completed)
        return dict(counter)

    def _handle_result(self, script_id: str, payload: dict[str, Any], completed_ts: float) -> None:
        with self._records_lock:
            record = self._pending.pop(script_id, None)
        if record is None:
            return

        duration_ms = payload.get("duration_ms")
        status = payload.get("status")

        row = {
            "script_id": script_id,
            "language": record.language,
            "outcome": record.outcome,
            "status": status,
            "duration_ms": duration_ms,
            "sent_ts": record.sent_ts,
            "completed_ts": completed_ts,
            "latency_s": max(completed_ts - record.sent_ts, 0.0),
        }
        with self._records_lock:
            self._completed.append(row)

