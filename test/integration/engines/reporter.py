"""
reporter.py — Thread-safe transaction recorder and summary generator.

Writes one JSON object per line to transactions.jsonl.
Computes latency percentiles and final summary on demand.
"""

from __future__ import annotations

import json
import logging
import os
import statistics
import threading
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

log = logging.getLogger(__name__)


class StressReporter:
    """Records transactions to a JSONL file and accumulates stats.

    Args:
        output_dir: Directory where transactions.jsonl and summary.json are written.
                    Created on first write if it does not exist.
    """

    def __init__(self, output_dir: str = "test/docker/data/stress/logs") -> None:
        self.output_dir = output_dir
        self._lock = threading.Lock()
        self._records: List[Dict[str, Any]] = []
        self._jsonl_path = os.path.join(output_dir, "transactions.jsonl")
        self._file_handle = None  # opened lazily
        # Truncate the JSONL at run start so each run's file holds ONLY that
        # run's transactions. Without this, records append across runs (the
        # file is reused under the bind-mounted output dir) and the log
        # misleadingly mixes a clean run with a prior failed one.
        os.makedirs(self.output_dir, exist_ok=True)
        open(self._jsonl_path, "w", encoding="utf-8").close()

    # ------------------------------------------------------------------
    # Transaction recording
    # ------------------------------------------------------------------

    def record_transaction(self, record: Dict[str, Any]) -> None:
        """Append a transaction record to the in-memory list and JSONL file.

        Expected keys: id, from, to, amount, status, req_id, duration_ms,
                       timestamp, error (optional).
        """
        with self._lock:
            self._records.append(record)
            self._append_to_file(record)

    def _append_to_file(self, record: Dict[str, Any]) -> None:
        """Write one JSON line.  Called under self._lock."""
        os.makedirs(self.output_dir, exist_ok=True)
        with open(self._jsonl_path, "a", encoding="utf-8") as fh:
            fh.write(json.dumps(record, default=str) + "\n")

    # ------------------------------------------------------------------
    # Summary
    # ------------------------------------------------------------------

    def summary(
        self,
        final_balances: Optional[Dict[str, float]] = None,
    ) -> Dict[str, Any]:
        """Compute and return the full run summary dict.

        Args:
            final_balances: {"nodeA": float, "nodeB": float, "quorum": float}
        """
        with self._lock:
            records = list(self._records)

        total = len(records)
        successes = [r for r in records if r.get("status") == "SUCCESS"]
        failures = [r for r in records if r.get("status") == "FAIL"]

        durations = [r["duration_ms"] for r in successes if "duration_ms" in r]

        latency: Dict[str, Any] = {}
        if durations:
            durations_sorted = sorted(durations)
            n = len(durations_sorted)
            latency = {
                "min_ms": durations_sorted[0],
                "max_ms": durations_sorted[-1],
                "mean_ms": round(statistics.mean(durations_sorted), 1),
                "p50_ms": durations_sorted[n // 2],
                "p95_ms": durations_sorted[int(n * 0.95)],
            }

        # Per-direction breakdown
        directions: Dict[str, Dict[str, int]] = {}
        for r in records:
            key = f"{r.get('from','?')}->{r.get('to','?')}"
            if key not in directions:
                directions[key] = {"total": 0, "success": 0, "fail": 0}
            directions[key]["total"] += 1
            if r.get("status") == "SUCCESS":
                directions[key]["success"] += 1
            else:
                directions[key]["fail"] += 1

        return {
            "generated_at": datetime.now(tz=timezone.utc).isoformat(),
            "total": total,
            "success": len(successes),
            "fail": len(failures),
            "latency": latency,
            "directions": directions,
            "final_balances": final_balances or {},
        }

    def write_summary(
        self,
        final_balances: Optional[Dict[str, float]] = None,
    ) -> str:
        """Write summary.json to output_dir and return the path."""
        result = self.summary(final_balances)
        path = os.path.join(self.output_dir, "summary.json")
        os.makedirs(self.output_dir, exist_ok=True)
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(result, fh, indent=2, default=str)
        log.info("Summary written to %s", path)
        return path

    def print_summary(self, final_balances: Optional[Dict[str, float]] = None) -> None:
        """Print a human-readable summary to the log."""
        s = self.summary(final_balances)
        log.info("=" * 60)
        log.info("STRESS RUN COMPLETE")
        log.info("  total transactions : %d", s["total"])
        log.info("  success            : %d", s["success"])
        log.info("  fail               : %d", s["fail"])
        if s["latency"]:
            lat = s["latency"]
            log.info(
                "  latency (success)  : min=%dms  p50=%dms  p95=%dms  max=%dms",
                lat["min_ms"], lat["p50_ms"], lat["p95_ms"], lat["max_ms"],
            )
        for direction, counts in s["directions"].items():
            log.info(
                "  %-12s         total=%d  ok=%d  fail=%d",
                direction, counts["total"], counts["success"], counts["fail"],
            )
        if s["final_balances"]:
            for node, bal in s["final_balances"].items():
                log.info("  balance %-10s : %.0f RBT", node, bal)
        log.info("=" * 60)
