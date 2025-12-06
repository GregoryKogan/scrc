from __future__ import annotations

import itertools
import math
import random
import threading
import time
from dataclasses import dataclass
from typing import Iterable

from kafka import KafkaProducer

from ..producer import ScriptScenario, build_scenarios, create_producer
from .collector import SubmissionRecord
from .config import LoadProfile


@dataclass
class LoadStatistics:
    produced: int
    started_at: float
    finished_at: float

    @property
    def duration_s(self) -> float:
        return max(self.finished_at - self.started_at, 0.0)

    @property
    def throughput_per_minute(self) -> float:
        if self.duration_s == 0:
            return 0.0
        return self.produced / self.duration_s * 60.0


class BenchmarkLoadGenerator:
    """Randomised load generator that samples languages/outcomes via weighted draws."""

    def __init__(
        self,
        broker: str,
        topic: str,
        profile: LoadProfile,
        collector_callback,
    ) -> None:
        self._broker = broker
        self._topic = topic
        self._profile = profile
        self._collector_callback = collector_callback

        self._language_weights = profile.normalised_language_weights()
        self._outcome_weights = profile.normalised_outcome_weights()
        self._scenarios = build_scenarios()
        self._scenario_lookup = {(s.language, s.outcome): s for s in self._scenarios}
        self._iteration_counter = itertools.count(start=1)
        self._stop_event = threading.Event()

    def run(self, duration_seconds: float) -> LoadStatistics:
        producer = self._create_producer()
        produced = 0
        started_at = time.time()
        deadline = started_at + duration_seconds
        lambda_per_sec = self._profile.submissions_per_minute / 60.0
        if lambda_per_sec <= 0:
            raise ValueError("LoadProfile submissions_per_minute must be > 0")

        try:
            while time.time() < deadline and not self._stop_event.is_set():
                scenario = self._pick_scenario()
                iteration = next(self._iteration_counter)
                payload = scenario.to_payload(iteration=iteration)
                script_id = payload["id"]
                sent_ts = time.time()
                self._collector_callback(
                    SubmissionRecord(
                        script_id=script_id,
                        language=scenario.language,
                        outcome=scenario.outcome,
                        sent_ts=sent_ts,
                    )
                )
                future = producer.send(self._topic, key=script_id, value=payload)
                future.get(timeout=30)
                produced += 1
                self._sleep_for_next(lambda_per_sec)
        finally:
            producer.flush()
            producer.close()

        finished_at = time.time()
        return LoadStatistics(produced=produced, started_at=started_at, finished_at=finished_at)

    def stop(self) -> None:
        self._stop_event.set()

    def _pick_scenario(self) -> ScriptScenario:
        language = _weighted_choice(self._language_weights)
        outcome = _weighted_choice(self._outcome_weights)
        scenario = self._scenario_lookup.get((language, outcome))
        if scenario is None:
            # Fallback to OK outcome if specific combo missing
            scenario = self._scenario_lookup[(language, "ok")]
        return scenario

    def _create_producer(self) -> KafkaProducer:
        return create_producer(self._broker)

    def _sleep_for_next(self, lambda_per_sec: float) -> None:
        u = random.random()
        delay = -math.log(1.0 - u) / lambda_per_sec
        remaining = max(delay, 0.0)
        if self._stop_event.wait(timeout=remaining):
            return


def _weighted_choice(weights: dict[str, float]) -> str:
    total = sum(weights.values())
    if total <= 0:
        raise ValueError("Weights must sum to > 0")
    r = random.random() * total
    upto = 0.0
    for key, weight in weights.items():
        upto += weight
        if upto >= r:
            return key
    # Float errors fallback
    return next(iter(weights))

