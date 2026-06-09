"""
native_env.py — Environment backed by native (no-Docker) host processes.

The no-Docker counterpart to DockerEnvironment. Instead of a compose stack, it
stands up three local Postgres instances (NativePostgresManager) and three native
rubixgoplatform processes (NativeClusterManager), then exposes the same
Environment contract (setup/teardown/node_ports) so the test suites run unchanged.

Used on macOS / Windows where GitHub-hosted runners can't run Linux node
containers. Selected via `runner.py --native`.

Ports are the node_index-derived defaults the binary uses natively (no Docker
port remapping):
    nodeA  index 0 -> API 20000, DB 5433, db rubix_a
    nodeB  index 1 -> API 20001, DB 5434, db rubix_b
    quorum index 2 -> API 20002, DB 5435, db rubix_q
These match test/integration/config.native.json — pass it with
`--config test/integration/config.native.json`.
"""

from __future__ import annotations

import logging
import os
from typing import List, Optional

from test.integration.env.base import Environment
from test.integration.env.native_cluster import NativeClusterManager, NativeNode
from test.integration.env.native_postgres import NativePostgresManager, PgInstance

log = logging.getLogger(__name__)

# Native node API ports (constants.RubixServerPort + node_index). These override
# the Docker NODE_PORTS (20010/11/12) because native mode does no port remapping.
NODE_PORTS = [20000, 20001, 20002]

# Workspace for native node data dirs + Postgres data dirs (gitignored, like
# test/docker/data/stress). Relative to repo root (where the runner runs).
_NATIVE_ROOT = os.path.join("test", "native", "data")

# (name, node_index, db_name, db_port) for the 3-node localnet cluster.
_NODE_SPECS = [
    ("nodeA", 0, "rubix_a", 5433),
    ("nodeB", 1, "rubix_b", 5434),
    ("quorum", 2, "rubix_q", 5435),
]


class NativeEnvironment(Environment):
    """Environment backed by native Postgres + rubixgoplatform processes.

    Args:
        binary: path to the rubixgoplatform binary. Defaults to the make
                compile output for the host OS (mac/ on darwin, linux/ elsewhere,
                windows/ on Windows).
        network_mode: rubix network (localnet for the integration suite).
    """

    NODE_PORTS: List[int] = NODE_PORTS

    def __init__(
        self,
        binary: Optional[str] = None,
        network_mode: str = "localnet",
    ) -> None:
        self._binary = binary or self._default_binary()

        nodes = [
            NativeNode(
                name=name,
                node_index=idx,
                db_name=db_name,
                db_port=db_port,
                work_dir=os.path.join(_NATIVE_ROOT, name, "app"),
            )
            for (name, idx, db_name, db_port) in _NODE_SPECS
        ]
        pg_instances = [
            PgInstance(
                name=name,
                port=db_port,
                db_name=db_name,
                data_dir=os.path.join(_NATIVE_ROOT, name, "pgdata"),
            )
            for (name, _idx, db_name, db_port) in _NODE_SPECS
        ]

        self._pg = NativePostgresManager(pg_instances)
        self._cluster = NativeClusterManager(
            nodes, binary=self._binary, network_mode=network_mode
        )

    def setup(self) -> None:
        # Postgres first — nodes fail to start without their DB reachable.
        self._pg.up()
        self._cluster.up()

    def teardown(self) -> None:
        # Stop nodes before Postgres so a node doesn't error on a vanishing DB.
        # Best-effort: both swallow their own errors so one failure doesn't
        # leak the other's resources.
        try:
            self._cluster.down()
        finally:
            self._pg.down()

    @staticmethod
    def _default_binary() -> str:
        """Path to the host's `make compile-*` binary output."""
        if os.name == "nt":
            return os.path.join("windows", "rubixgoplatform.exe")
        import platform
        if platform.system() == "Darwin":
            return os.path.join("mac", "rubixgoplatform")
        return os.path.join("linux", "rubixgoplatform")
