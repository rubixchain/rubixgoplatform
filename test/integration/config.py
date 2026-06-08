"""
config.py — StressConfig: load, validate, and expose all stress-test parameters.

Load priority:
  1. JSON file passed via --config (or the default config.json)
  2. --small-test flag applies overrides from "small_test_overrides" key
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional


@dataclass
class MintConfig:
    start_index: int
    end_index: int       # exclusive: tokens minted in [start_index, end_index)
    batch_size: int = 10_000

    @property
    def total_tokens(self) -> int:
        return self.end_index - self.start_index

    @property
    def num_batches(self) -> int:
        return (self.total_tokens + self.batch_size - 1) // self.batch_size

    @classmethod
    def from_dict(cls, d: Dict[str, Any]) -> "MintConfig":
        return cls(
            start_index=int(d["start_index"]),
            end_index=int(d["end_index"]),
            batch_size=int(d.get("batch_size", 10_000)),
        )


@dataclass
class PhaseConfig:
    name: str
    concurrency: int   # 1 = sequential, >1 = parallel
    tx_count: int      # transactions to attempt in this phase

    @classmethod
    def from_dict(cls, d: Dict[str, Any]) -> "PhaseConfig":
        return cls(
            name=str(d["name"]),
            concurrency=int(d["concurrency"]),
            tx_count=int(d["tx_count"]),
        )


@dataclass
class StressConfig:
    # Node API ports (host-mapped)
    node_a_port: int = 20010
    node_b_port: int = 20011
    quorum_port: int = 20012

    # DB ports (host-mapped)
    db_a_port: int = 5436
    db_b_port: int = 5437
    db_q_port: int = 5438

    password: str = "mypassword"
    output_dir: str = "test/docker/data/stress/logs"

    # Minting configuration
    node_a_mint: MintConfig = field(
        default_factory=lambda: MintConfig(100_000, 200_000, 10_000)
    )
    quorum_mint: MintConfig = field(
        default_factory=lambda: MintConfig(500_000, 1_000_000, 10_000)
    )

    # Shuttle parameters
    min_balance_buffer: int = 500   # RBT kept in reserve — never transferred
    quorum_free_threshold: int = 10  # stop when quorum free < this
    max_transactions: int = 3000     # hard cap regardless of quorum balance

    # Manual-mode: warn (don't gate) when any node's RBT balance drops below this.
    # Used to warn the operator when a node's balance runs low (manual runs).
    low_balance_threshold: float = 100.0

    # Per-transfer RBT cap for the shuttle. Default 1000 (matches the original
    # _MAX_SINGLE_TX constant). Lower it (e.g. to 1) to keep a fractional-only
    # run within a tiny token budget.
    max_single_tx: float = 1000.0

    # How the SC callback receiver URL is reachable FROM the node:
    #   "docker" -> http://host.docker.internal:<port>  (node runs in a container)
    #   "host"   -> http://127.0.0.1:<port>             (node is a native process)
    callback_url_mode: str = "docker"

    # Phase definitions
    phases: List[PhaseConfig] = field(default_factory=list)

    # -------------------------------------------------------------------------
    # Factory
    # -------------------------------------------------------------------------

    # v1 signature — preserved for reference
    # @classmethod
    # def load(
    #     cls,
    #     config_path: Optional[str] = None,
    #     small_test: bool = False,
    # ) -> "StressConfig":

    @classmethod
    def load(
        cls,
        config_path: Optional[str] = None,
        small_test: bool = False,
        micro_test: bool = False,  # 1 nodeA token, 5 quorum tokens, 5 sequential txns
    ) -> "StressConfig":
        """Load config from JSON file.  Apply overrides if requested.

        Override precedence (highest wins): micro_test > small_test > base config.
        """
        if config_path is None:
            config_path = os.path.join(os.path.dirname(__file__), "config.json")

        with open(config_path, encoding="utf-8") as fh:
            raw: Dict[str, Any] = json.load(fh)

        # Strip comment-only keys
        raw.pop("_comment", None)
        raw.pop("_comment_micro", None)

        # Apply overrides before parsing — micro_test takes priority over small_test
        if micro_test:
            overrides: Dict[str, Any] = raw.pop("micro_test_overrides", {})
            raw.pop("small_test_overrides", None)
            raw.update(overrides)
        elif small_test:
            overrides = raw.pop("small_test_overrides", {})
            raw.pop("micro_test_overrides", None)
            raw.update(overrides)
        else:
            raw.pop("small_test_overrides", None)
            raw.pop("micro_test_overrides", None)

        return cls._from_dict(raw)

    @classmethod
    def _from_dict(cls, d: Dict[str, Any]) -> "StressConfig":
        phases = [PhaseConfig.from_dict(p) for p in d.get("phases", [])]
        return cls(
            node_a_port=int(d.get("node_a_port", 20010)),
            node_b_port=int(d.get("node_b_port", 20011)),
            quorum_port=int(d.get("quorum_port", 20012)),
            db_a_port=int(d.get("db_a_port", 5436)),
            db_b_port=int(d.get("db_b_port", 5437)),
            db_q_port=int(d.get("db_q_port", 5438)),
            password=str(d.get("password", "mypassword")),
            output_dir=str(d.get("output_dir", "test/docker/data/stress/logs")),
            node_a_mint=MintConfig.from_dict(d["node_a_mint"]),
            quorum_mint=MintConfig.from_dict(d["quorum_mint"]),
            min_balance_buffer=int(d.get("min_balance_buffer", 500)),
            quorum_free_threshold=int(d.get("quorum_free_threshold", 10)),
            max_transactions=int(d.get("max_transactions", 3000)),
            low_balance_threshold=float(d.get("low_balance_threshold", 100.0)),
            max_single_tx=float(d.get("max_single_tx", 1000.0)),
            callback_url_mode=str(d.get("callback_url_mode", "docker")),
            phases=phases,
        )

    def summary(self) -> str:
        lines = [
            f"nodeA mint : {self.node_a_mint.start_index}–{self.node_a_mint.end_index}"
            f" ({self.node_a_mint.total_tokens} tokens, "
            f"{self.node_a_mint.num_batches} batches × {self.node_a_mint.batch_size})",
            f"quorum mint: {self.quorum_mint.start_index}–{self.quorum_mint.end_index}"
            f" ({self.quorum_mint.total_tokens} tokens, "
            f"{self.quorum_mint.num_batches} batches × {self.quorum_mint.batch_size})",
            f"buffer={self.min_balance_buffer}  q_threshold={self.quorum_free_threshold}"
            f"  max_tx={self.max_transactions}",
        ]
        for ph in self.phases:
            lines.append(
                f"  phase={ph.name}  concurrency={ph.concurrency}  tx_count={ph.tx_count}"
            )
        return "\n".join(lines)
