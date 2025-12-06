from __future__ import annotations

import contextlib
import logging
import os
import time
from dataclasses import dataclass
from typing import Dict, Iterable, Iterator, List

import docker
from docker.models.containers import Container


LOGGER = logging.getLogger("scrc_simulator.benchmark.docker")


@dataclass
class RunnerConfig:
    replicas: int
    parallelism: int
    environment: Dict[str, str]


class RunnerManager:
    """Provision scrc runner containers for the benchmark using the Docker API."""

    def __init__(
        self,
        image: str,
        base_environment: Dict[str, str],
        network_names: Iterable[str],
        log_wait_seconds: float = 5.0,
        startup_grace_seconds: float = 20.0,
    ) -> None:
        self._image = image
        self._base_env = base_environment
        self._network_names = list(network_names)
        self._client = docker.from_env()
        self._containers: List[Container] = []
        self._log_wait_seconds = log_wait_seconds
        self._startup_grace_seconds = startup_grace_seconds

    def run(self, config: RunnerConfig) -> contextlib.AbstractContextManager[None]:
        return _RunnerContext(self, config)

    def _start(self, config: RunnerConfig) -> None:
        LOGGER.info(
            "Starting %d runner container(s) with parallelism=%d",
            config.replicas,
            config.parallelism,
        )
        env = dict(self._base_env)
        env.update(config.environment)
        env["RUNNER_MAX_PARALLEL"] = str(config.parallelism)

        for idx in range(config.replicas):
            name = f"scrc-benchmark-runner-{int(time.time())}-{idx}"
            container = self._client.containers.run(
                self._image,
                name=name,
                detach=True,
                environment=env,
                volumes=self._runner_volumes(),
                network=self._primary_network(),
                network_mode=None,
            )
            self._attach_additional_networks(container)
            self._containers.append(container)

        self._wait_for_startup()

    def _stop(self) -> None:
        LOGGER.info("Stopping %d runner container(s)", len(self._containers))
        for container in self._containers:
            with contextlib.suppress(Exception):
                container.stop(timeout=10)
            with contextlib.suppress(Exception):
                container.remove(force=True)
        self._containers.clear()

    def _wait_for_startup(self) -> None:
        deadline = time.time() + self._startup_grace_seconds
        while time.time() < deadline:
            if all(self._is_container_healthy(container) for container in self._containers):
                time.sleep(self._log_wait_seconds)
                return
            time.sleep(1.0)
        LOGGER.warning("Runner containers may not be fully ready before load starts")

    def _is_container_healthy(self, container: Container) -> bool:
        with contextlib.suppress(Exception):
            container.reload()
            status = container.attrs.get("State", {})
            if status.get("Health"):
                return status["Health"]["Status"] == "healthy"
            return status.get("Running", False)
        return False

    def _runner_volumes(self) -> Dict[str, dict]:
        docker_host = os.environ.get("DOCKER_HOST")
        if docker_host and docker_host.startswith("tcp://"):
            LOGGER.debug("Runner using remote Docker host via %s", docker_host)
            return {}
        return {"/var/run/docker.sock": {"bind": "/var/run/docker.sock", "mode": "rw"}}

    def _primary_network(self) -> str | None:
        return self._network_names[0] if self._network_names else None

    def _attach_additional_networks(self, container: Container) -> None:
        for network in self._network_names[1:]:
            with contextlib.suppress(Exception):
                self._client.networks.get(network).connect(container)


class _RunnerContext(contextlib.AbstractContextManager[None]):
    def __init__(self, manager: RunnerManager, config: RunnerConfig) -> None:
        self._manager = manager
        self._config = config

    def __enter__(self) -> None:
        self._manager._start(self._config)

    def __exit__(self, exc_type, exc, tb) -> None:
        self._manager._stop()

