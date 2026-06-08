"""
shuttle.py — Token shuttle engine: drives bidirectional A↔B transfers.

Transfer pattern:
  Tokens always move between nodeA and nodeB only.
  nodeA sends to nodeB (A→B), then nodeB sends back to nodeA (B→A), repeating.
  Quorum pledges its tokens for every transaction and unpledges after commit.

Phases:
  1. sequential_warmup — one transaction at a time, alternating A→B / B→A
  2. parallel_N        — batches of N concurrent transfers per direction

Stop conditions (checked before every batch):
  1. quorum FREE token sum (token_status=0) < quorum_free_threshold
  2. total transaction counter >= max_transactions

Balance safety:
  - Every sender balance check is done immediately before the batch.
  - Per-tx amount = floor((available - buffer) / concurrency), capped at 1000 RBT.
  - Minimum transfer is 1 RBT (whole integers only).
  - If a sender cannot safely send even 1 RBT, the direction is skipped for that batch.
"""

from __future__ import annotations

import logging
import random
import threading
import time
from datetime import datetime, timezone
from typing import TYPE_CHECKING, Dict, Optional, Tuple

if TYPE_CHECKING:
    from test.integration.clients.api_client import NodeClient
    from test.integration.clients.db_validator import DBValidator
    from test.integration.config import StressConfig, PhaseConfig
    from test.integration.engines.reporter import StressReporter

log = logging.getLogger(__name__)

_MAX_SINGLE_TX = 1000  # RBT cap per individual transfer


class ShuttleEngine:
    """Runs all configured phases, driving transfers between nodeA and nodeB.

    Args:
        node_a, node_b: NodeClient instances.
        did_a, did_b:   DIDs for nodeA and nodeB respectively.
        db_q:           DBValidator for quorum (used for free-token stop check).
        config:         StressConfig.
        reporter:       StressReporter for recording transactions.
    """

    def __init__(
        self,
        node_a: "NodeClient",
        node_b: "NodeClient",
        did_a: str,
        did_b: str,
        db_q: "DBValidator",
        config: "StressConfig",
        reporter: "StressReporter",
        decimal_transfers: bool = False,
    ) -> None:
        self.node_a = node_a
        self.node_b = node_b
        self.did_a = did_a
        self.did_b = did_b
        self.db_q = db_q
        self.cfg = config
        self.reporter = reporter
        self._decimal_transfers = decimal_transfers
        # Per-tx RBT cap: configurable (falls back to the module default).
        self._max_single_tx = float(getattr(config, "max_single_tx", _MAX_SINGLE_TX))

        self._tx_counter = 0
        self._counter_lock = threading.Lock()
        self._stop_flag = False
        self._stop_reason: Optional[str] = None

    # ------------------------------------------------------------------
    # Entry point
    # ------------------------------------------------------------------

    def run_all_phases(self) -> None:
        """Execute all phases in order.  Returns when done or stop condition hits."""
        for phase in self.cfg.phases:
            if self._check_stop():
                log.info(
                    "Stopping before phase '%s': %s",
                    phase.name, self._stop_reason,
                )
                break
            log.info(
                "=== Phase: %s  concurrency=%d  tx_count=%d ===",
                phase.name, phase.concurrency, phase.tx_count,
            )
            t0 = time.time()
            if phase.concurrency == 1:
                self._run_sequential(phase.tx_count)
            else:
                self._run_parallel(phase.tx_count, phase.concurrency)
            elapsed = time.time() - t0
            log.info(
                "Phase '%s' done in %.1fs.  Total transactions so far: %d",
                phase.name, elapsed, self._tx_counter,
            )

    # ------------------------------------------------------------------
    # Sequential phase
    # ------------------------------------------------------------------

    def _run_sequential(self, tx_count: int) -> None:
        """Alternate A→B / B→A, one at a time."""
        completed = 0
        direction_ab = True  # start with A→B
        consecutive_skips = 0  # break the phase if BOTH directions skip in a row

        while completed < tx_count:
            if self._check_stop():
                break

            if direction_ab:
                sender, sender_did, receiver_did = self.node_a, self.did_a, self.did_b
                label = "A->B"
            else:
                sender, sender_did, receiver_did = self.node_b, self.did_b, self.did_a
                label = "B->A"

            amount = self._safe_amount(sender, sender_did, concurrency=1)
            if amount <= 0:
                # Skipping does NOT advance `completed`, so we must guard against
                # an infinite loop when neither side can transfer above buffer.
                consecutive_skips += 1
                if consecutive_skips >= 2:
                    log.warning(
                        "Both directions below balance buffer — ending shuttle "
                        "phase after %d transfers.", completed,
                    )
                    break
                log.warning("[%s] Skipping — insufficient balance after buffer", label)
                direction_ab = not direction_ab
                continue

            consecutive_skips = 0
            self._fire_transfer(sender, sender_did, receiver_did, amount, label)
            completed += 1
            direction_ab = not direction_ab

            # Wait for token settlement when switching from A->B to B->A
            if not direction_ab and completed < tx_count:
                log.info("Waiting 5 seconds for token settlement before B->A transfer...")
                time.sleep(5)

    # ------------------------------------------------------------------
    # Parallel phase
    # ------------------------------------------------------------------

    def _run_parallel(self, tx_count: int, concurrency: int) -> None:
        """Fire batches of *concurrency* transfers; alternate direction per batch."""
        completed = 0
        direction_ab = True
        consecutive_skips = 0  # break the phase if BOTH directions skip in a row

        while completed < tx_count:
            if self._check_stop():
                break

            batch_size = min(concurrency, tx_count - completed)

            if direction_ab:
                sender, sender_did, receiver_did = self.node_a, self.did_a, self.did_b
                label = "A->B"
            else:
                sender, sender_did, receiver_did = self.node_b, self.did_b, self.did_a
                label = "B->A"

            amount = self._safe_amount(sender, sender_did, concurrency=batch_size)
            if amount <= 0:
                # Skipping does NOT advance `completed`; guard against an
                # infinite loop when neither side can transfer above buffer.
                consecutive_skips += 1
                if consecutive_skips >= 2:
                    log.warning(
                        "Both directions below balance buffer — ending shuttle "
                        "phase after %d batches.", completed,
                    )
                    break
                log.warning("[%s] Skipping batch — insufficient balance after buffer", label)
                direction_ab = not direction_ab
                continue

            consecutive_skips = 0

            log.info("[%s] Launching batch of %d × %.3f RBT", label, batch_size, amount)
            threads = [
                threading.Thread(
                    target=self._fire_transfer,
                    args=(sender, sender_did, receiver_did, amount, label),
                    daemon=True,
                )
                for _ in range(batch_size)
            ]
            for t in threads:
                t.start()
            for t in threads:
                t.join(timeout=180)

            completed += batch_size
            direction_ab = not direction_ab

            # Wait for token settlement when switching from A->B to B->A
            if not direction_ab and completed < tx_count:
                log.info("Waiting 5 seconds for token settlement before B->A batch...")
                time.sleep(5)

    # ------------------------------------------------------------------
    # Core transfer
    # ------------------------------------------------------------------

    def _fire_transfer(
        self,
        sender: "NodeClient",
        sender_did: str,
        receiver_did: str,
        amount: float,
        direction: str,
    ) -> None:
        """Execute one transfer and record it in the reporter."""
        with self._counter_lock:
            self._tx_counter += 1
            tx_id = f"TX-{self._tx_counter:05d}"

        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            result = sender.transfer_rbt(sender_did, receiver_did, amount)
            req_id = result.get("req_id")
            txn_id = sender.extract_txn_id(result)
        except Exception as exc:
            status = "FAIL"
            error = str(exc)

        duration_ms = int((time.time() - t0) * 1000)

        self.reporter.record_transaction(
            {
                "id": tx_id,
                "from": direction.split("->")[0],
                "to": direction.split("->")[1],
                "amount": amount,
                "status": status,
                "req_id": req_id,
                "transaction_id": txn_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
            }
        )

        if status == "SUCCESS":
            log.info(
                "[%s] %s  amount=%.3f  req_id=%s  %dms",
                tx_id, direction, amount, req_id, duration_ms,
            )
        else:
            log.warning("[%s] %s  FAIL  amount=%.3f  %dms  error=%s",
                        tx_id, direction, amount, duration_ms, error)

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------

    def _safe_amount(
        self,
        sender: "NodeClient",
        sender_did: str,
        concurrency: int,
    ) -> float:
        """Return the max safe transfer amount per TX.

        When decimal_transfers is False (default):
            amount = floor((balance - buffer) / concurrency), capped at _MAX_SINGLE_TX.
            Returns 0 if the sender cannot safely send even 1 RBT.

        When decimal_transfers is True:
            Returns a random float in [0.001, per_tx) rounded to 3 decimal places.
        """
        try:
            balance = float(sender.get_balance(sender_did))
        except Exception as exc:
            log.warning("Balance check failed for %s: %s", sender_did[:16], exc)
            return 0.0

        available = balance - self.cfg.min_balance_buffer
        if available <= 0:
            return 0.0

        if self._decimal_transfers:
            per_tx = min(available / max(concurrency, 1), self._max_single_tx)
            if per_tx < 0.001:
                return 0.0
            # Random float in [0.001, per_tx], rounded to 3 decimal places
            amount = round(random.uniform(0.001, per_tx), 3)
            return amount
        else:
            per_tx = min(int(available) // max(concurrency, 1), int(self._max_single_tx))
            if per_tx <= 0:
                return 0.0
            # Vary the transfer amount within [1, per_tx-1] so the sender always
            # retains at least 1 RBT above the buffer after the transfer.
            # When per_tx==1 there is no room to reduce, so send 1 as-is.
            return float(random.randint(1, max(1, per_tx - 1)))

    def _check_stop(self) -> bool:
        """Set and return self._stop_flag if any stop condition is met."""
        if self._stop_flag:
            return True

        with self._counter_lock:
            tx_count = self._tx_counter

        if tx_count >= self.cfg.max_transactions:
            self._stop_flag = True
            self._stop_reason = (
                f"max_transactions ({self.cfg.max_transactions}) reached"
            )
            log.info("Stop: %s", self._stop_reason)
            return True

        try:
            q_free = self.db_q.get_token_sum()
            if q_free < self.cfg.quorum_free_threshold:
                self._stop_flag = True
                self._stop_reason = (
                    f"quorum free tokens {q_free:.0f} < threshold "
                    f"{self.cfg.quorum_free_threshold}"
                )
                log.info("Stop: %s", self._stop_reason)
                return True
        except Exception as exc:
            log.warning("Could not read quorum free balance: %s", exc)

        return False

    @property
    def total_transactions(self) -> int:
        with self._counter_lock:
            return self._tx_counter

    @property
    def stop_reason(self) -> Optional[str]:
        return self._stop_reason
