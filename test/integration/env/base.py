"""
base.py — Environment abstraction for the integration test harness.

An Environment is responsible ONLY for the lifecycle of the node cluster the
tests run against — bringing nodes up, exposing where to reach them (API +
DB ports), and tearing them down. Test logic (engines/, tests/) is deliberately
unaware of *how* the cluster came to exist.

This seam is what lets the same suites run against either:
  - DockerEnvironment  -> compose up/down a fresh 3-node stack (docker_env.py)
  - ExternalEnvironment -> connect to already-running nodes (added with the
                           non-Docker workflow; not yet implemented)

Concrete envs subclass Environment and implement setup()/teardown().
"""

from __future__ import annotations

import logging
from typing import List

log = logging.getLogger(__name__)


class Environment:
    """Lifecycle contract for a node cluster under test.

    Subclasses must implement setup() and teardown(). node_ports() returns the
    host-mapped API ports the tests should target, in [nodeA, nodeB, quorum]
    order.
    """

    #: host-mapped node API ports, in [nodeA, nodeB, quorum] order
    NODE_PORTS: List[int] = [20010, 20011, 20012]

    def setup(self) -> None:
        """Bring the cluster up and block until all nodes are reachable."""
        raise NotImplementedError

    def teardown(self) -> None:
        """Tear the cluster down (best-effort; safe to call on failure)."""
        raise NotImplementedError

    def node_ports(self) -> List[int]:
        return list(self.NODE_PORTS)

    # Context-manager sugar so callers can do `with env: ...` and get
    # guaranteed teardown even when a test raises.
    def __enter__(self) -> "Environment":
        self.setup()
        return self

    def __exit__(self, exc_type, exc, tb) -> bool:
        self.teardown()
        return False  # never swallow exceptions
