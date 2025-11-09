from __future__ import annotations

import collections
import copy
import json
import threading
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Iterator

import six

# The kafka python client imports six from its own vendor path; preload it
# so our standalone environment works without extra dependencies.
sys_modules = __import__("sys").modules
sys_modules.setdefault("kafka.vendor.six", six)
sys_modules.setdefault("kafka.vendor.six.moves", six.moves)

from kafka import KafkaProducer
from kafka.errors import NoBrokersAvailable

LANGUAGE_EXTENSIONS: dict[str, str] = {
    "python": ".py",
    "go": ".go",
    "c": ".c",
    "cpp": ".cpp",
    "java": ".java",
}

OUTCOMES: tuple[str, ...] = ("ok", "wa", "tl", "ml", "bf")

BASE_DIR = Path(__file__).resolve().parent
SCRIPTS_DIR = BASE_DIR / "scripts"

SUM_TESTS: list[dict[str, object]] = [
    {"number": 1, "input": "1 2 3\n", "expected_output": "6\n"},
    {"number": 2, "input": "10 -5 7\n", "expected_output": "12\n"},
]

SCENARIO_CONFIG: dict[str, dict[str, object]] = {
    "ok": {
        "limits": {
            "time_limit_ms": 2_000,
            "memory_limit_bytes": 256 * 1024 * 1024,
        },
        "tests": SUM_TESTS,
    },
    "wa": {
        "limits": {
            "time_limit_ms": 2_000,
            "memory_limit_bytes": 256 * 1024 * 1024,
        },
        "tests": SUM_TESTS,
    },
    "tl": {
        "limits": {
            "time_limit_ms": 1_000,
            "memory_limit_bytes": 256 * 1024 * 1024,
        },
        "tests": [
            {"number": 1, "input": "", "expected_output": ""},
        ],
    },
    "ml": {
        "limits": {
            "time_limit_ms": 5_000,
            "memory_limit_bytes": 16 * 1024 * 1024,
        },
        "tests": [
            {"number": 1, "input": "", "expected_output": ""},
        ],
    },
    "bf": {
        "limits": {
            "time_limit_ms": 2_000,
            "memory_limit_bytes": 256 * 1024 * 1024,
        },
        "tests": [],
    },
}


@dataclass(frozen=True)
class ScriptScenario:
    language: str
    outcome: str
    source: str
    limits: dict[str, object]
    tests: list[dict[str, object]]

    @property
    def type(self) -> str:
        return f"{self.language}-{self.outcome}"

    def to_payload(self, iteration: int) -> dict[str, object]:
        return {
            "type": "script",
            "id": f"{self.type}-{iteration}",
            "language": self.language,
            "source": self.source,
            "limits": self.limits,
            "tests": self.tests,
        }


def create_producer(broker: str) -> KafkaProducer:
    backoff = 1.0
    max_backoff = 10.0
    deadline = time.time() + 60

    while True:
        try:
            return KafkaProducer(
                bootstrap_servers=broker,
                key_serializer=lambda v: v.encode("utf-8") if v else None,
                value_serializer=lambda v: json.dumps(v).encode("utf-8"),
            )
        except NoBrokersAvailable as exc:
            if time.time() >= deadline:
                raise RuntimeError(
                    "failed to connect to Kafka broker within 60 seconds"
                ) from exc

            time.sleep(backoff)
            backoff = min(backoff * 1.5, max_backoff)


def load_script_source(language: str, outcome: str) -> str:
    extension = LANGUAGE_EXTENSIONS[language]
    path = SCRIPTS_DIR / language / f"{outcome}{extension}"
    return path.read_text(encoding="utf-8")


def build_scenarios() -> list[ScriptScenario]:
    scenarios: list[ScriptScenario] = []
    for language in LANGUAGE_EXTENSIONS:
        for outcome in OUTCOMES:
            source = load_script_source(language, outcome)
            config = SCENARIO_CONFIG[outcome]
            limits = copy.deepcopy(config["limits"])
            tests = copy.deepcopy(config["tests"])
            scenarios.append(
                ScriptScenario(
                    language=language,
                    outcome=outcome,
                    source=source,
                    limits=limits,
                    tests=tests,
                )
            )
    return scenarios


def stream_scripts(
    scenarios: list[ScriptScenario],
    interval_s: float,
    max_iterations: int | None = None,
    stop_event: threading.Event | None = None,
) -> Iterator[tuple[ScriptScenario, dict[str, object]]]:
    iteration = 1
    while True:
        for scenario in scenarios:
            if stop_event is not None and stop_event.is_set():
                return
            yield scenario, scenario.to_payload(iteration)
        iteration += 1
        if max_iterations is not None and iteration > max_iterations:
            return
        if interval_s <= 0:
            continue
        if stop_event is not None:
            if stop_event.wait(interval_s):
                return
        else:
            time.sleep(interval_s)


class ScriptProducerService:
    def __init__(
        self,
        broker: str,
        topic: str,
        interval_s: float,
        max_iterations: int | None = None,
        scenarios: list[ScriptScenario] | None = None,
    ) -> None:
        self._broker = broker
        self._topic = topic
        self._interval_s = interval_s
        self._max_iterations = max_iterations
        self._scenarios = scenarios or build_scenarios()
        self.counters: collections.Counter[str] = collections.Counter()

    def run(self, stop_event: threading.Event | None = None) -> collections.Counter[str]:
        producer = create_producer(self._broker)
        try:
            for scenario, payload in stream_scripts(
                self._scenarios,
                self._interval_s,
                self._max_iterations,
                stop_event,
            ):
                future = producer.send(self._topic, key=payload["id"], value=payload)
                future.get(timeout=30)
                self.counters[scenario.type] += 1
        finally:
            producer.flush()
            producer.close()
        return self.counters


__all__ = [
    "LANGUAGE_EXTENSIONS",
    "OUTCOMES",
    "ScriptScenario",
    "build_scenarios",
    "ScriptProducerService",
]
