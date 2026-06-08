"""
minter.py — Batch token minting with per-batch timing and a mint_timing.log.

Mints tokens in fixed-size batches, polling balance after each batch.
All timing data is appended to mint_timing.log inside output_dir.
"""

from __future__ import annotations

import logging
import os
import time
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from test.integration.config import MintConfig
    from test.integration.clients.api_client import NodeClient

log = logging.getLogger(__name__)


class BatchMinter:
    """Mints tokens in batches and logs timing per batch + total.

    Args:
        node_client:  NodeClient for the node that will receive tokens.
        did:          DID of the token owner.
        mint_config:  MintConfig (start_index, end_index, batch_size).
        output_dir:   Directory where mint_timing.log is written.
        node_label:   Human-readable label for log lines (e.g. "nodeA", "quorum").
    """

    def __init__(
        self,
        node_client: "NodeClient",
        did: str,
        mint_config: "MintConfig",
        output_dir: str,
        node_label: str,
    ) -> None:
        self.client = node_client
        self.did = did
        self.cfg = mint_config
        self.output_dir = output_dir
        self.label = node_label
        self._log_path = os.path.join(output_dir, "mint_timing.log")

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def run(self) -> float:
        """Run all batches.  Returns total elapsed seconds."""
        os.makedirs(self.output_dir, exist_ok=True)

        total_tokens = self.cfg.total_tokens
        num_batches = self.cfg.num_batches
        log.info(
            "[%s] Starting mint: %d tokens in %d batches of %d (range [%d, %d))",
            self.label, total_tokens, num_batches, self.cfg.batch_size,
            self.cfg.start_index, self.cfg.end_index,
        )
        self._write_log(
            f"START  node={self.label}  total={total_tokens}  batches={num_batches}"
            f"  range=[{self.cfg.start_index},{self.cfg.end_index})"
        )

        overall_start = time.time()
        current_start = self.cfg.start_index
        batch_num = 0

        while current_start < self.cfg.end_index:
            batch_num += 1
            count = min(self.cfg.batch_size, self.cfg.end_index - current_start)
            # Cumulative tokens this DID should hold AFTER this batch — used so the
            # per-batch skip/poll compares against the running total, not the
            # per-batch count (otherwise only the first batch ever mints).
            cumulative = (current_start + count) - self.cfg.start_index
            self._mint_batch(batch_num, num_batches, current_start, count, cumulative)
            current_start += count

        total_elapsed = time.time() - overall_start
        self._write_log(
            f"TOTAL  node={self.label}  tokens={total_tokens}"
            f"  elapsed={total_elapsed:.1f}s  ({_fmt_duration(total_elapsed)})"
        )
        log.info(
            "[%s] Mint complete: %d tokens in %.1fs (%s)",
            self.label, total_tokens, total_elapsed, _fmt_duration(total_elapsed),
        )
        return total_elapsed

    # ------------------------------------------------------------------
    # Internal
    # ------------------------------------------------------------------

    _MAX_RETRIES = 3
    _RETRY_DELAY = 10  # seconds between retries

    def _mint_batch(
        self,
        batch_num: int,
        total_batches: int,
        start_index: int,
        count: int,
        cumulative: int,
    ) -> None:
        log.info(
            "[%s] Batch %d/%d: start_index=%d  count=%d",
            self.label, batch_num, total_batches, start_index, count,
        )
        t0 = time.time()
        last_exc: Exception | None = None

        for attempt in range(1, self._MAX_RETRIES + 1):
            try:
                self.client.generate_local_rbt(
                    self.did, count, start_index, expected_total=cumulative
                )
                elapsed = time.time() - t0
                self._write_log(
                    f"BATCH  node={self.label}  batch={batch_num}/{total_batches}"
                    f"  start_index={start_index}  count={count}"
                    f"  elapsed={elapsed:.1f}s  status=OK  attempt={attempt}"
                )
                log.info(
                    "[%s] Batch %d/%d done in %.1fs (attempt %d)",
                    self.label, batch_num, total_batches, elapsed, attempt,
                )
                return
            except Exception as exc:
                last_exc = exc
                elapsed = time.time() - t0
                log.warning(
                    "[%s] Batch %d/%d attempt %d/%d failed after %.1fs: %s",
                    self.label, batch_num, total_batches, attempt, self._MAX_RETRIES,
                    elapsed, exc,
                )
                if attempt < self._MAX_RETRIES:
                    log.info(
                        "[%s] Retrying batch %d/%d in %ds…",
                        self.label, batch_num, total_batches, self._RETRY_DELAY,
                    )
                    time.sleep(self._RETRY_DELAY)

        elapsed = time.time() - t0
        self._write_log(
            f"BATCH  node={self.label}  batch={batch_num}/{total_batches}"
            f"  start_index={start_index}  count={count}"
            f"  elapsed={elapsed:.1f}s  status=FAIL  error={last_exc}"
        )
        log.error(
            "[%s] Batch %d/%d FAILED after %d attempts (%.1fs): %s",
            self.label, batch_num, total_batches, self._MAX_RETRIES, elapsed, last_exc,
        )
        raise last_exc  # type: ignore[misc]

    def _write_log(self, line: str) -> None:
        ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        entry = f"[{ts}] {line}\n"
        with open(self._log_path, "a", encoding="utf-8") as fh:
            fh.write(entry)


def _fmt_duration(seconds: float) -> str:
    mins, secs = divmod(int(seconds), 60)
    if mins:
        return f"{mins}m {secs}s"
    return f"{secs}s"
