from __future__ import annotations

import logging
from pathlib import Path

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
import seaborn as sns

from .config import BenchmarkMatrix

LOGGER = logging.getLogger("scrc_simulator.benchmark.charts")

# Set a modern, professional style
sns.set_style("whitegrid")
plt.rcParams["figure.dpi"] = 100
plt.rcParams["savefig.dpi"] = 300
plt.rcParams["font.size"] = 10
plt.rcParams["axes.labelsize"] = 11
plt.rcParams["axes.titlesize"] = 13
plt.rcParams["xtick.labelsize"] = 10
plt.rcParams["ytick.labelsize"] = 10
plt.rcParams["legend.fontsize"] = 9
plt.rcParams["figure.titlesize"] = 14

# Modern color palette for outcomes
OUTCOME_COLORS = {
    "ok": "#2E86AB",  # Blue
    "wa": "#A23B72",  # Purple
    "tl": "#F18F01",  # Orange
    "ml": "#C73E1D",  # Red
    "bf": "#6A994E",  # Green
}

# Language display names
LANGUAGE_NAMES = {
    "python": "Python",
    "go": "Go",
    "c": "C",
    "cpp": "C++",
    "java": "Java",
}


def render_matrix_charts(
    matrix: BenchmarkMatrix,
    matrix_results: dict[str, pd.DataFrame],
    throughput_summary: dict[str, float],
    output_dir: Path,
) -> Path:
    """Render charts for a benchmark matrix based on its chart_type."""
    chart_path = output_dir / matrix.chart_filename

    if matrix.chart_type == "violin":
        chart_path = _render_violin_chart(matrix, matrix_results, chart_path)
    elif matrix.chart_type == "line":
        _render_line_chart(matrix, matrix_results, throughput_summary, chart_path)
    elif matrix.chart_type == "heatmap":
        _render_heatmap_chart(matrix, matrix_results, throughput_summary, chart_path)
    elif matrix.chart_type == "bar":
        _render_bar_chart(matrix, matrix_results, throughput_summary, chart_path)
    elif matrix.chart_type == "stacked":
        _render_stacked_chart(matrix, matrix_results, chart_path)
    else:
        raise ValueError(f"Unknown chart type: {matrix.chart_type}")

    LOGGER.info("Rendering chart %s", chart_path)
    return chart_path


def _render_violin_chart(
    matrix: BenchmarkMatrix,
    matrix_results: dict[str, pd.DataFrame],
    chart_path: Path,
) -> Path:
    """Render improved latency distribution charts as separate images."""
    # Combine all dataframes
    df = pd.concat(matrix_results.values(), ignore_index=True)
    if df.empty or "latency_s" not in df.columns:
        LOGGER.warning("No latency data available for latency chart")
        return chart_path

    # Filter out invalid data
    df = df[df["latency_s"].notna() & (df["latency_s"] >= 0)].copy()
    if df.empty:
        LOGGER.warning("No valid latency data after filtering")
        return chart_path

    # Map language codes to display names
    df["language_display"] = df["language"].map(
        lambda x: LANGUAGE_NAMES.get(x, x.title())
    )

    language_order = sorted(
        df["language"].unique(), key=lambda x: LANGUAGE_NAMES.get(x, x)
    )
    outcome_order = ["ok", "wa", "tl", "ml", "bf"]

    # Generate two separate chart files
    output_dir = chart_path.parent

    # Chart 1: Statistical Summary Heatmap
    heatmap_path = output_dir / "language_latency_heatmap.png"
    fig1, ax1 = plt.subplots(figsize=(12, 6))
    _render_latency_summary_heatmap(df, language_order, outcome_order, ax1)
    fig1.savefig(
        heatmap_path, bbox_inches="tight", facecolor="white", edgecolor="none", dpi=300
    )
    plt.close(fig1)
    LOGGER.info("Rendering chart %s", heatmap_path)

    # Chart 2: Faceted Box Plots
    boxplot_path = output_dir / "language_latency_boxplot.png"
    fig2, ax2 = plt.subplots(figsize=(14, 8))
    _render_faceted_boxplots(df, language_order, outcome_order, ax2)
    fig2.savefig(
        boxplot_path, bbox_inches="tight", facecolor="white", edgecolor="none", dpi=300
    )
    plt.close(fig2)
    LOGGER.info("Rendering chart %s", boxplot_path)

    # Return the boxplot path (the main detailed chart)
    return boxplot_path


def _render_latency_summary_heatmap(
    df: pd.DataFrame,
    language_order: list[str],
    outcome_order: list[str],
    ax: plt.Axes,
) -> None:
    """Render a heatmap showing key latency statistics."""
    # Calculate statistics for each language-outcome combination
    stats = []
    for lang in language_order:
        row = []
        for outcome in outcome_order:
            subset = df[(df["language"] == lang) & (df["outcome"] == outcome)][
                "latency_s"
            ]
            if len(subset) > 0:
                # Use median as the main metric
                row.append(subset.median())
            else:
                row.append(0)
        stats.append(row)

    # Create heatmap
    stats_array = np.array(stats)
    im = ax.imshow(stats_array, cmap="YlOrRd", aspect="auto", vmin=0)

    # Set ticks and labels
    ax.set_xticks(range(len(outcome_order)))
    ax.set_xticklabels([o.upper() for o in outcome_order])
    ax.set_yticks(range(len(language_order)))
    ax.set_yticklabels(
        [LANGUAGE_NAMES.get(lang, lang.title()) for lang in language_order]
    )

    # Add text annotations
    for i in range(len(language_order)):
        for j in range(len(outcome_order)):
            value = stats[i][j]
            if value > 0:
                max_val = stats_array.max()
                text_color = (
                    "white" if max_val > 0 and value > max_val * 0.6 else "black"
                )
                ax.text(
                    j,
                    i,
                    f"{value:.2f}s",
                    ha="center",
                    va="center",
                    color=text_color,
                    fontweight="bold",
                    fontsize=9,
                )

    # Add colorbar
    cbar = plt.colorbar(im, ax=ax)
    cbar.set_label("Median Latency (seconds)", fontweight="semibold", labelpad=10)

    ax.set_title(
        "Median Latency by Language & Outcome", fontweight="bold", pad=15, fontsize=14
    )
    ax.set_xlabel("Outcome", fontweight="semibold", labelpad=10)
    ax.set_ylabel("Language", fontweight="semibold", labelpad=10)

    # Remove grid lines for clean heatmap appearance
    ax.grid(False)


def _render_faceted_boxplots(
    df: pd.DataFrame,
    language_order: list[str],
    outcome_order: list[str],
    ax: plt.Axes,
) -> None:
    """Render faceted box plots for detailed distribution view."""
    # Create box plots using seaborn
    sns.boxplot(
        data=df,
        x="language_display",
        y="latency_s",
        hue="outcome",
        order=[LANGUAGE_NAMES.get(lang, lang.title()) for lang in language_order],
        hue_order=outcome_order,
        palette=[OUTCOME_COLORS.get(outcome, "#808080") for outcome in outcome_order],
        ax=ax,
        linewidth=1.5,
        width=0.7,
    )

    ax.set_xlabel("Language", fontweight="semibold", labelpad=12, fontsize=12)
    ax.set_ylabel("Latency (seconds)", fontweight="semibold", labelpad=12, fontsize=12)
    ax.set_ylim(bottom=0)

    # Improve legend
    handles, labels = ax.get_legend_handles_labels()
    ax.legend(
        handles=handles,
        labels=[label.upper() for label in labels],
        loc="upper right",
        frameon=True,
        fancybox=True,
        shadow=True,
        title="Outcome",
        title_fontsize=10,
        fontsize=9,
        ncol=len(outcome_order),
    )

    ax.set_title(
        "Result Latency Distribution by Language & Outcome",
        fontweight="bold",
        pad=15,
        fontsize=14,
    )

    # Grid and styling
    ax.grid(True, alpha=0.3, linestyle="--", linewidth=0.5, axis="y")
    ax.set_axisbelow(True)
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)
    ax.spines["left"].set_color("#CCCCCC")
    ax.spines["bottom"].set_color("#CCCCCC")


def _render_line_chart(
    matrix: BenchmarkMatrix,
    matrix_results: dict[str, pd.DataFrame],
    throughput_summary: dict[str, float],
    chart_path: Path,
) -> None:
    """Render a line chart for throughput vs parallelism."""
    fig, ax = plt.subplots(figsize=(10, 6))

    # Extract parallelism values and sort
    def extract_parallelism(key: str) -> int:
        """Extract parallelism value from key."""
        if "x" in key:
            # Format: "1x4" or "scale-1x4"
            parts = key.split("x")
            if len(parts) > 1:
                return int(parts[-1])
        # Format: "single-runner-parallel-8" or similar
        parts = key.split("-")
        for part in reversed(parts):
            if part.isdigit():
                return int(part)
        return 0

    keys = sorted(throughput_summary.keys(), key=extract_parallelism)
    x_values = [extract_parallelism(k) for k in keys]
    y_values = [throughput_summary[k] for k in keys]

    ax.plot(
        x_values, y_values, marker="o", linewidth=2.5, markersize=8, color="#2E86AB"
    )
    ax.set_xlabel("RUNNER_MAX_PARALLEL", fontweight="semibold")
    ax.set_ylabel("Throughput (submissions/min)", fontweight="semibold")
    ax.set_title(matrix.chart_title, fontweight="bold", pad=15)
    ax.grid(True, alpha=0.3, linestyle="--")

    plt.tight_layout()
    fig.savefig(chart_path, bbox_inches="tight", facecolor="white")
    plt.close(fig)


def _render_heatmap_chart(
    matrix: BenchmarkMatrix,
    matrix_results: dict[str, pd.DataFrame],
    throughput_summary: dict[str, float],
    chart_path: Path,
) -> None:
    """Render a heatmap for scaling combinations."""
    fig, ax = plt.subplots(figsize=(10, 6))

    # Parse scale keys (e.g., "1x4" -> replicas=1, parallel=4)
    data = {}
    replicas_set = set()
    parallel_set = set()

    for key, throughput in throughput_summary.items():
        if "x" in key:
            parts = key.split("x")
            replicas = int(parts[0])
            parallel = int(parts[1])
            data[(replicas, parallel)] = throughput
            replicas_set.add(replicas)
            parallel_set.add(parallel)

    # Create matrix
    replicas_list = sorted(replicas_set)
    parallel_list = sorted(parallel_set)
    matrix_data = [[data.get((r, p), 0) for p in parallel_list] for r in replicas_list]

    sns.heatmap(
        matrix_data,
        annot=True,
        fmt=".1f",
        xticklabels=parallel_list,
        yticklabels=[f"{r} replica{'s' if r > 1 else ''}" for r in replicas_list],
        cmap="YlOrRd",
        cbar_kws={"label": "Throughput (submissions/min)"},
        ax=ax,
    )

    ax.set_xlabel("Parallelism", fontweight="semibold")
    ax.set_ylabel("Replicas", fontweight="semibold")
    ax.set_title(matrix.chart_title, fontweight="bold", pad=15)

    plt.tight_layout()
    fig.savefig(chart_path, bbox_inches="tight", facecolor="white")
    plt.close(fig)


def _render_bar_chart(
    matrix: BenchmarkMatrix,
    matrix_results: dict[str, pd.DataFrame],
    throughput_summary: dict[str, float],
    chart_path: Path,
) -> None:
    """Render a bar chart comparing throughput."""
    fig, ax = plt.subplots(figsize=(10, 6))

    labels = list(throughput_summary.keys())
    values = list(throughput_summary.values())

    # Clean up labels for display
    display_labels = [
        label.replace("timeout-", "").replace("-", " ").title() for label in labels
    ]

    bars = ax.bar(
        display_labels,
        values,
        color=["#2E86AB", "#F18F01"],
        alpha=0.8,
        edgecolor="white",
        linewidth=2,
    )
    ax.set_ylabel("Throughput (submissions/min)", fontweight="semibold")
    ax.set_title(matrix.chart_title, fontweight="bold", pad=15)
    ax.grid(True, alpha=0.3, axis="y", linestyle="--")

    # Add value labels on bars
    for bar in bars:
        height = bar.get_height()
        ax.text(
            bar.get_x() + bar.get_width() / 2.0,
            height,
            f"{height:.1f}",
            ha="center",
            va="bottom",
            fontweight="semibold",
        )

    plt.tight_layout()
    fig.savefig(chart_path, bbox_inches="tight", facecolor="white")
    plt.close(fig)


def _render_stacked_chart(
    matrix: BenchmarkMatrix,
    matrix_results: dict[str, pd.DataFrame],
    chart_path: Path,
) -> None:
    """Render a stacked bar chart for language throughput."""
    fig, ax = plt.subplots(figsize=(12, 6))

    # Combine all dataframes
    df = pd.concat(matrix_results.values(), ignore_index=True)

    # Count submissions by language and outcome
    counts = df.groupby(["language", "outcome"]).size().unstack(fill_value=0)

    # Normalize to percentages
    totals = counts.sum(axis=1)
    percentages = counts.div(totals, axis=0) * 100

    # Plot stacked bars
    outcome_order = ["ok", "wa", "tl", "ml", "bf"]
    colors = [OUTCOME_COLORS.get(outcome, "#808080") for outcome in outcome_order]

    bottom = None
    for outcome in outcome_order:
        if outcome in percentages.columns:
            ax.bar(
                [LANGUAGE_NAMES.get(lang, lang.title()) for lang in percentages.index],
                percentages[outcome],
                bottom=bottom,
                label=outcome.upper(),
                color=OUTCOME_COLORS.get(outcome, "#808080"),
                alpha=0.8,
                edgecolor="white",
                linewidth=1.5,
            )
            if bottom is None:
                bottom = percentages[outcome].values
            else:
                bottom += percentages[outcome].values

    ax.set_ylabel("Percentage of Submissions", fontweight="semibold")
    ax.set_xlabel("Language", fontweight="semibold")
    ax.set_title(matrix.chart_title, fontweight="bold", pad=15)
    ax.legend(
        loc="center left",
        bbox_to_anchor=(1.0, 0.5),
        frameon=True,
        fancybox=True,
        shadow=True,
    )
    ax.set_ylim(0, 100)
    ax.grid(True, alpha=0.3, axis="y", linestyle="--")

    plt.tight_layout()
    fig.savefig(chart_path, bbox_inches="tight", facecolor="white")
    plt.close(fig)
