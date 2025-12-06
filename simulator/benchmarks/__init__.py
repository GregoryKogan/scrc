"""
Benchmarking harness for the SCRC simulator.

This package provides facilities to drive randomized submission loads against the
runner service, orchestrate container concurrency sweeps, and render
presentation-ready charts summarising throughput and latency characteristics.
"""

from .main import main

__all__ = ["main"]

