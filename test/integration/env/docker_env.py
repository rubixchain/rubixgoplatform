"""
docker_env.py — Docker Compose lifecycle for the Rubix integration test stack.

Uses test/docker/docker-compose.stress.yml (project rubix-stress).
Container names: rubix-stress-nodeA, rubix-stress-nodeB, rubix-stress-quorum.

StressDockerManager is the low-level compose wrapper (up/down/health).
DockerEnvironment adapts it to the Environment interface (setup/teardown) so
the harness can swap in an external/non-Docker environment later without the
test suites changing.
"""

from __future__ import annotations

import logging
import os
import shutil
import subprocess
import time
from typing import List

from test.integration.env.base import Environment

log = logging.getLogger(__name__)

NODE_PORTS = [20010, 20011, 20012]

# Bind-mount state dir for the stress stack. The compose file mounts
# ./data/stress/<node>/{db,app} into the containers — these are BIND-MOUNTS,
# so `docker compose down -v` does NOT remove them. Stale state here (e.g.
# accumulated quorum DIDs in quorum_manager) causes "Quorum is not setup"
# pledge failures on the next run. We wipe it before each bring-up to guarantee
# isolation. Relative to repo root (where the runner is invoked).
_STATE_DIR = os.path.join("test", "docker", "data", "stress")

_CONTAINER_BY_PORT = {
    20010: "rubix-stress-nodeA",
    20011: "rubix-stress-nodeB",
    20012: "rubix-stress-quorum",
}


class StressDockerManager:
    def __init__(
        self,
        compose_file: str = "test/docker/docker-compose.stress.yml",
        project_name: str = "rubix-stress",
    ) -> None:
        self.compose_file = compose_file
        self.project_name = project_name

    def up(self, build: bool = True, timeout: int = 480) -> None:
        """Start the stack and block until all nodes are healthy.

        Timeout is larger than a standard stack because large token minting
        operations may push container startup longer.
        """
        ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        log.info(
            "[%s] Starting integration compose stack (build=%s, wait-timeout=%ds)…",
            ts, build, timeout,
        )
        self._force_remove_stale()

        # Build the shared image via a SINGLE service, not the whole project.
        # All three node services declare the same image tag (rubix-node:latest).
        # On a cold cache, `docker compose build` (or `up --build`) builds all
        # three in parallel and they race to export to the same tag — one fails
        # with `image "rubix-node:latest": already exists`. Building just one
        # service populates the tag once; `up` then reuses it for all three.
        if build:
            subprocess.run(
                ["docker", "compose", "-f", self.compose_file,
                 "-p", self.project_name, "build", "nodeA"],
                check=True,
            )

        subprocess.run(
            ["docker", "compose", "-f", self.compose_file,
             "-p", self.project_name,
             "up", "-d", "--wait", "--remove-orphans"],
            check=True,
        )
        log.info("All integration nodes healthy.")

    def down(self, volumes: bool = True) -> None:
        ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        log.info("[%s] Tearing down integration stack (volumes=%s)…", ts, volumes)
        cmd = [
            "docker", "compose",
            "-f", self.compose_file,
            "-p", self.project_name,
            "down",
        ]
        if volumes:
            cmd.append("-v")
        subprocess.run(cmd, check=True)
        log.info("Integration stack torn down.")

    def is_healthy(self, port: int) -> bool:
        container = _CONTAINER_BY_PORT.get(port, f"{self.project_name}-nodeA")
        try:
            result = subprocess.run(
                ["docker", "exec", container, "bash", "-c", "</dev/tcp/localhost/20000"],
                capture_output=True,
                timeout=5,
            )
            return result.returncode == 0
        except Exception:
            return False

    def _force_remove_stale(self) -> None:
        result = subprocess.run(
            ["docker", "ps", "-aq", "--filter", f"name={self.project_name}"],
            capture_output=True,
            text=True,
        )
        ids = result.stdout.strip().splitlines()
        if ids:
            log.info("Removing %d stale container(s) before up…", len(ids))
            subprocess.run(["docker", "rm", "-f"] + ids, check=False)


class DockerEnvironment(Environment):
    """Environment backed by a fresh docker-compose stack.

    Args:
        compose_file: path to the compose file (default: stress stack).
        project_name: compose project name (isolates containers/networks).
        build:        rebuild node images on up (default: True).
        teardown_volumes: remove volumes on teardown (default: True — clean DB
                          state every run).
    """

    NODE_PORTS: List[int] = NODE_PORTS

    def __init__(
        self,
        compose_file: str = "test/docker/docker-compose.stress.yml",
        project_name: str = "rubix-stress",
        build: bool = True,
        teardown_volumes: bool = True,
    ) -> None:
        self._dm = StressDockerManager(compose_file, project_name)
        self._build = build
        self._teardown_volumes = teardown_volumes

    def setup(self) -> None:
        self._clean_state()
        self._dm.up(build=self._build)

    def teardown(self) -> None:
        self._dm.down(volumes=self._teardown_volumes)

    @staticmethod
    def _clean_state() -> None:
        """Wipe the bind-mount state dir so every run starts from a clean DB.

        Critical for test isolation: the Postgres data is bind-mounted, so
        `down -v` leaves it behind. Without this, quorum DIDs and tokens from
        prior runs leak in and cause "Quorum is not setup" pledge failures.
        Preserves the logs/ subdir (run artifacts), wipes per-node db/app state.
        """
        if not os.path.isdir(_STATE_DIR):
            return
        for entry in os.listdir(_STATE_DIR):
            if entry == "logs":
                continue  # keep run artifacts (transactions.jsonl, summary.json, …)
            path = os.path.join(_STATE_DIR, entry)
            try:
                if os.path.isdir(path):
                    shutil.rmtree(path)
                else:
                    os.remove(path)
            except OSError as exc:  # noqa: BLE001
                log.warning("Could not remove stale state %s: %s", path, exc)
        log.info("Cleaned stale bind-mount state under %s (kept logs/).", _STATE_DIR)
