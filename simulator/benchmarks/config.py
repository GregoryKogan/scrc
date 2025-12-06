from __future__ import annotations

import dataclasses
from dataclasses import dataclass, field
from typing import Iterable, Sequence

from ..producer import OUTCOMES, LANGUAGE_EXTENSIONS


@dataclass(frozen=True)
class LoadProfile:
    """Statistical distribution for randomized submission workloads."""

    name: str
    submissions_per_minute: float
    language_weights: dict[str, float]
    outcome_weights: dict[str, float]
    tl_multiplier: float = 1.0

    def normalised_language_weights(self) -> dict[str, float]:
        weights = {lang: self.language_weights.get(lang, 0.0) for lang in LANGUAGE_EXTENSIONS}
        return _normalise(weights)

    def normalised_outcome_weights(self) -> dict[str, float]:
        weights = {outcome: self.outcome_weights.get(outcome, 0.0) for outcome in OUTCOMES}
        if self.tl_multiplier != 1.0:
            weights["tl"] = weights.get("tl", 0.0) * self.tl_multiplier
        return _normalise(weights)


@dataclass(frozen=True)
class BenchmarkCase:
    """Single execution of the benchmark load against a runner configuration."""

    name: str
    load_profile: LoadProfile
    duration_seconds: int
    warmup_seconds: int
    runner_parallelism: int
    runner_replicas: int = 1
    notes: str | None = None

    def scale_key(self) -> str:
        return f"{self.runner_replicas}x{self.runner_parallelism}"


@dataclass(frozen=True)
class BenchmarkMatrix:
    """Group of cases sharing the same base workload but sweeping parameters."""

    label: str
    cases: Sequence[BenchmarkCase]
    chart_type: str
    chart_title: str
    chart_filename: str
    description: str | None = None


@dataclass
class BenchmarkPlan:
    """Complete set of benchmark matrices the harness will execute."""

    matrices: list[BenchmarkMatrix] = field(default_factory=list)

    def __iter__(self) -> Iterable[BenchmarkMatrix]:
        return iter(self.matrices)


def default_benchmark_plan() -> BenchmarkPlan:
    """Return the default suite of benchmark matrices."""

    light = LoadProfile(
        name="light",
        submissions_per_minute=40,
        language_weights={lang: 1.0 for lang in LANGUAGE_EXTENSIONS},
        outcome_weights={outcome: (1.0 if outcome != "tl" else 0.3) for outcome in OUTCOMES},
    )
    medium = LoadProfile(
        name="medium",
        submissions_per_minute=75,
        language_weights={lang: 1.0 for lang in LANGUAGE_EXTENSIONS},
        outcome_weights={outcome: 1.0 for outcome in OUTCOMES},
    )
    heavy = LoadProfile(
        name="heavy",
        submissions_per_minute=120,
        language_weights={
            "python": 1.2,
            "go": 1.0,
            "c": 0.9,
            "cpp": 0.9,
            "java": 1.1,
        },
        outcome_weights={
            "ok": 1.0,
            "wa": 0.8,
            "ml": 0.5,
            "bf": 0.6,
            "tl": 0.9,
        },
    )
    timeout_heavy = dataclasses.replace(heavy, name="timeout-heavy", tl_multiplier=2.5)

    matrices = [
        BenchmarkMatrix(
            label="single-runner-sweep",
            description=(
                "Single runner throughput while sweeping RUNNER_MAX_PARALLEL under the medium random load."
            ),
            chart_type="line",
            chart_title="Single Runner Throughput vs RUNNER_MAX_PARALLEL",
            chart_filename="single_runner_throughput.png",
            cases=[
                BenchmarkCase(
                    name=f"single-runner-parallel-{parallel}",
                    load_profile=medium,
                    duration_seconds=180,
                    warmup_seconds=20,
                    runner_parallelism=parallel,
                )
                for parallel in (1, 2, 4, 6, 8, 12, 16)
            ],
        ),
        BenchmarkMatrix(
            label="language-latency",
            description="Latency distribution per language/outcome under the light random load.",
            chart_type="violin",
            chart_title="Result Latency Distribution by Language & Outcome",
            chart_filename="language_latency.png",
            cases=[
                BenchmarkCase(
                    name="language-latency-baseline",
                    load_profile=light,
                    duration_seconds=180,
                    warmup_seconds=20,
                    runner_parallelism=8,
                )
            ],
        ),
        BenchmarkMatrix(
            label="scaling-heatmap",
            description="Heatmap of throughput for runner replica and parallelism combinations.",
            chart_type="heatmap",
            chart_title="Throughput Heatmap (Replicas x Parallelism)",
            chart_filename="scaling_heatmap.png",
            cases=[
                BenchmarkCase(
                    name=f"scale-{replicas}x{parallel}",
                    load_profile=medium,
                    duration_seconds=160,
                    warmup_seconds=20,
                    runner_parallelism=parallel,
                    runner_replicas=replicas,
                )
                for replicas in (1, 2, 3)
                for parallel in (4, 6, 8, 12)
            ],
        ),
        BenchmarkMatrix(
            label="timeout-impact",
            description="Effect of timeout-heavy submissions on overall throughput.",
            chart_type="bar",
            chart_title="Throughput Impact of Timeout-Heavy Workloads",
            chart_filename="timeout_impact.png",
            cases=[
                BenchmarkCase(
                    name="timeout-baseline",
                    load_profile=medium,
                    duration_seconds=150,
                    warmup_seconds=20,
                    runner_parallelism=8,
                ),
                BenchmarkCase(
                    name="timeout-heavy",
                    load_profile=timeout_heavy,
                    duration_seconds=150,
                    warmup_seconds=20,
                    runner_parallelism=8,
                    notes="TL-weighted workload",
                ),
            ],
        ),
        BenchmarkMatrix(
            label="language-throughput",
            description="Per-language throughput share under heavy load.",
            chart_type="stacked",
            chart_title="Per-Language Throughput Share (Heavy Load)",
            chart_filename="language_throughput.png",
            cases=[
                BenchmarkCase(
                    name="language-throughput-heavy",
                    load_profile=heavy,
                    duration_seconds=180,
                    warmup_seconds=20,
                    runner_parallelism=12,
                    runner_replicas=2,
                )
            ],
        ),
    ]

    return BenchmarkPlan(matrices=list(matrices))


def _normalise(weights: dict[str, float]) -> dict[str, float]:
    total = sum(value for value in weights.values() if value > 0)
    if total <= 0:
        raise ValueError("LoadProfile weights must sum to > 0")
    return {key: max(value, 0.0) / total for key, value in weights.items()}

