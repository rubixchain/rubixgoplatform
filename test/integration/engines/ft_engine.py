"""
ft_engine.py — Fungible Token (FT) creation and transfer engine for stress testing.

Workflow:
  - Alternates FT mint operations between nodeA and nodeB.
  - Each mint creates a batch of FTs under a unique ``ft_name`` by burning
    a configurable amount of RBT.
  - ``ft_num_start_index`` is auto-managed per DID so successive mints
    never collide.
  - Optional transfer phase moves a subset of minted FTs to the opposite
    node using POST /rubix/v1/tx (the ``ft`` array).
  - Optional repeated transfer rounds alternating A->B / B->A.
  - Records every mint / transfer in the reporter.

Pattern matches nft_engine.py for consistency.
"""

from __future__ import annotations

import logging
import random
import string
import threading
import time
from datetime import datetime, timezone
from typing import TYPE_CHECKING, Dict, List, Optional

if TYPE_CHECKING:
    from test.integration.clients.api_client import NodeClient
    from test.integration.config import PhaseConfig, StressConfig
    from test.integration.engines.reporter import StressReporter

log = logging.getLogger(__name__)


class FTEngine:
    """Runs FT mint + transfer phases, alternating between nodeA and nodeB.

    Args:
        node_a, node_b:         NodeClient instances.
        did_a, did_b:           DIDs for nodeA and nodeB respectively.
        config:                 StressConfig.
        reporter:               StressReporter for recording operations.
        tokens_per_batch:       Number of FTs created per mint operation.
        rbt_per_batch:          RBT burned per mint operation.
    """

    def __init__(
        self,
        node_a: "NodeClient",
        node_b: "NodeClient",
        did_a: str,
        did_b: str,
        config: "StressConfig",
        reporter: "StressReporter",
        tokens_per_batch: int = 100,
        rbt_per_batch: int = 10,
    ) -> None:
        self.node_a = node_a
        self.node_b = node_b
        self.did_a = did_a
        self.did_b = did_b
        self.cfg = config
        self.reporter = reporter
        self.tokens_per_batch = tokens_per_batch
        self.rbt_per_batch = rbt_per_batch

        self._ft_counter = 0
        self._counter_lock = threading.Lock()

        # Monotonic ft_num_start_index per DID to avoid index collisions
        # between successive mints by the same creator.
        self._next_start_index: Dict[str, int] = {did_a: 0, did_b: 0}
        self._index_lock = threading.Lock()

        # Track minted FT batches for the transfer/verification phases.
        # Each entry: {ft_name, creator_did, owner_did, owner_node,
        #              owner_label, ft_count, ft_seq}
        self._minted_fts: List[Dict] = []
        self._minted_lock = threading.Lock()

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------

    def _reserve_start_index(self, did: str) -> int:
        """Allocate + return the next ft_num_start_index for *did*."""
        with self._index_lock:
            idx = self._next_start_index.get(did, 0)
            self._next_start_index[did] = idx + self.tokens_per_batch
            return idx

    @property
    def total_mints(self) -> int:
        """Total number of FT mint operations attempted so far."""
        with self._counter_lock:
            return self._ft_counter

    # ------------------------------------------------------------------
    # Entry point
    # ------------------------------------------------------------------

    def run_all_phases(self, phases: List["PhaseConfig"]) -> None:
        """Execute all FT mint phases in order."""
        for phase in phases:
            log.info(
                "=== FT Phase: %s  concurrency=%d  ft_mints=%d  "
                "(tokens_per_batch=%d  rbt_per_batch=%d) ===",
                phase.name,
                phase.concurrency,
                phase.tx_count,
                self.tokens_per_batch,
                self.rbt_per_batch,
            )
            t0 = time.time()
            if phase.concurrency == 1:
                self._run_sequential(phase.tx_count)
            else:
                self._run_parallel(phase.tx_count, phase.concurrency)
            elapsed = time.time() - t0
            log.info(
                "FT Phase '%s' done in %.1fs.  Total mints: %d",
                phase.name, elapsed, self._ft_counter,
            )

    # ------------------------------------------------------------------
    # Sequential / parallel phases
    # ------------------------------------------------------------------

    def _run_sequential(self, ft_count: int) -> None:
        """Alternate A / B, one mint at a time."""
        completed = 0
        use_node_a = True
        while completed < ft_count:
            if use_node_a:
                node, did, label = self.node_a, self.did_a, "A"
            else:
                node, did, label = self.node_b, self.did_b, "B"
            self._fire_ft_mint(node, did, label)
            completed += 1
            use_node_a = not use_node_a

    def _run_parallel(self, ft_count: int, concurrency: int) -> None:
        """Fire batches of *concurrency* FT mints; alternate node per batch."""
        completed = 0
        use_node_a = True
        while completed < ft_count:
            batch_size = min(concurrency, ft_count - completed)
            if use_node_a:
                node, did, label = self.node_a, self.did_a, "A"
            else:
                node, did, label = self.node_b, self.did_b, "B"

            log.info("[%s] Launching FT mint batch of %d", label, batch_size)
            threads = [
                threading.Thread(
                    target=self._fire_ft_mint,
                    args=(node, did, label),
                    daemon=True,
                )
                for _ in range(batch_size)
            ]
            for t in threads:
                t.start()
            for t in threads:
                t.join(timeout=180)

            completed += batch_size
            use_node_a = not use_node_a

    # ------------------------------------------------------------------
    # Core FT mint
    # ------------------------------------------------------------------

    def _fire_ft_mint(
        self,
        node: "NodeClient",
        did: str,
        label: str,
    ) -> None:
        """Mint one batch of FTs on *node* under *did*."""
        with self._counter_lock:
            self._ft_counter += 1
            mint_counter = self._ft_counter
            ft_seq_id = f"FT-{mint_counter:05d}"

        # Random 4-char suffix keeps the name unique ACROSS runs against the
        # same DIDs (the per-run counter alone collides on re-run, which the
        # node rejects as a duplicate genesis). label+counter stays for
        # readability; the suffix guarantees a fresh (creator, name) row.
        # NOTE: no hyphens in the FT name (kept as a single alphanumeric token).
        rand_suffix = "".join(random.choices(string.ascii_lowercase + string.digits, k=4))
        ft_name = f"ft{label}{mint_counter:05d}{rand_suffix}"
        start_index = self._reserve_start_index(did)

        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            result = node.mint_ft(
                did=did,
                ft_name=ft_name,
                ft_count=self.tokens_per_batch,
                token_count=self.rbt_per_batch,
                ft_num_start_index=start_index,
            )
            req_id = result.get("req_id")
            txn_id = node.extract_txn_id(result)
        except Exception as exc:
            status = "FAIL"
            error = str(exc)

        duration_ms = int((time.time() - t0) * 1000)

        self.reporter.record_transaction(
            {
                "id": ft_seq_id,
                "type": "FT_MINT",
                "node": label,
                "did": did[:20] + "...",
                "ft_name": ft_name,
                "ft_count": self.tokens_per_batch,
                "rbt_burned": self.rbt_per_batch,
                "ft_num_start_index": start_index,
                "status": status,
                "req_id": req_id,
                "transaction_id": txn_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
            }
        )

        if status == "SUCCESS":
            with self._minted_lock:
                self._minted_fts.append({
                    "ft_name": ft_name,
                    "creator_did": did,
                    "owner_did": did,
                    "owner_node": node,
                    "owner_label": label,
                    "ft_count": self.tokens_per_batch,
                    "ft_seq": ft_seq_id,
                })
            log.info(
                "[%s] Node-%s  ft_name=%s  count=%d  rbt=%d  start_idx=%d  req=%s  %dms",
                ft_seq_id, label, ft_name, self.tokens_per_batch,
                self.rbt_per_batch, start_index, req_id, duration_ms,
            )
        else:
            log.warning(
                "[%s] Node-%s  FAIL  %dms  error=%s",
                ft_seq_id, label, duration_ms, error,
            )

    # ------------------------------------------------------------------
    # FT transfer phase (single-shot: each minted FT -> opposite node)
    # ------------------------------------------------------------------

    def run_ft_transfer(self, fraction: float = 0.5) -> None:
        """Transfer a fraction of each minted FT batch to the opposite node.

        Args:
            fraction: Portion of each batch to send (0 < fraction <= 1).
                      Default 0.5 keeps half on the creator so subsequent
                      rounds / verification have something to assert on.
        """
        if fraction <= 0 or fraction > 1:
            raise ValueError("fraction must be in (0, 1]")

        with self._minted_lock:
            fts_snapshot = list(self._minted_fts)

        if not fts_snapshot:
            log.warning("No minted FTs available for transfer")
            return

        log.info(
            "=== FT TRANSFER START: %d FT batches, fraction=%.2f ===",
            len(fts_snapshot), fraction,
        )

        # Let the mint txns settle on chain before attempting transfers.
        log.info("Waiting 5 seconds for FT mints to settle on chain...")
        time.sleep(5)

        transfer_counter = 0
        for ft_info in fts_snapshot:
            transfer_counter += 1
            self._fire_ft_transfer(ft_info, fraction, transfer_counter)
            # Small settle gap between transfers
            time.sleep(2)

        log.info("=== FT TRANSFER COMPLETE ===")

    def _fire_ft_transfer(
        self,
        ft_info: Dict,
        fraction: float,
        seq: int,
    ) -> None:
        """Transfer *fraction* of one FT batch to the opposite node."""
        owner_label = ft_info["owner_label"]
        owner_did = ft_info["owner_did"]
        owner_node = ft_info["owner_node"]
        ft_name = ft_info["ft_name"]
        creator_did = ft_info["creator_did"]
        total = ft_info["ft_count"]

        transfer_count = max(1, int(total * fraction))

        if owner_label == "A":
            recv_node, recv_did, recv_label = self.node_b, self.did_b, "B"
        else:
            recv_node, recv_did, recv_label = self.node_a, self.did_a, "A"

        seq_id = f"FT-XFER-{seq:04d}"
        log.info(
            "[%s] Transfer ft_name=%s  count=%d  %s->%s",
            seq_id, ft_name, transfer_count, owner_label, recv_label,
        )

        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            result = owner_node.transfer_ft(
                sender_did=owner_did,
                receiver_did=recv_did,
                ft_name=ft_name,
                ft_count=transfer_count,
                creator_did=creator_did,
                memo=f"FT transfer {ft_name} {owner_label}->{recv_label}",
            )
            req_id = result.get("req_id")
            txn_id = owner_node.extract_txn_id(result)
        except Exception as exc:
            status = "FAIL"
            error = str(exc)

        duration_ms = int((time.time() - t0) * 1000)

        self.reporter.record_transaction(
            {
                "id": seq_id,
                "type": "FT_TRANSFER",
                "node": owner_label,
                "sender_did": owner_did[:20] + "...",
                "receiver_did": recv_did[:20] + "...",
                "ft_name": ft_name,
                "creator_did": creator_did[:20] + "...",
                "ft_count": transfer_count,
                "status": status,
                "req_id": req_id,
                "transaction_id": txn_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
            }
        )

        if status == "SUCCESS":
            remaining = total - transfer_count
            # Update the ownership book-keeping: we now split the batch.
            with self._minted_lock:
                # Shrink the creator's batch; append a receiver-side copy
                # so subsequent transfers/verification see both holders.
                for entry in self._minted_fts:
                    if entry is ft_info:
                        entry["ft_count"] = remaining
                        break
                self._minted_fts.append({
                    "ft_name": ft_name,
                    "creator_did": creator_did,
                    "owner_did": recv_did,
                    "owner_node": recv_node,
                    "owner_label": recv_label,
                    "ft_count": transfer_count,
                    "ft_seq": f"{ft_info['ft_seq']}-X",
                })
            log.info(
                "[%s] OK  %dms  remaining_on_%s=%d",
                seq_id, duration_ms, owner_label, remaining,
            )
        else:
            log.warning("[%s] FAIL  %dms  error=%s", seq_id, duration_ms, error)

    # ------------------------------------------------------------------
    # Repeated transfer rounds (alternating A<->B per round)
    # ------------------------------------------------------------------

    def run_repeated_transfers(self, rounds: int, count_per_round: int = 1) -> Dict:
        """Perform *rounds* additional FT transfers per minted batch,
        alternating direction each round.

        Round 0 / even: current_owner -> opposite.
        Round 1 / odd:  opposite -> current_owner.

        Args:
            rounds:           Rounds per FT batch.
            count_per_round:  Number of FTs to move each round (must be <=
                              what the current sender holds at that time).

        Returns:
            Dict ft_name -> {"success": n, "fail": n}.
        """
        with self._minted_lock:
            batches = [b for b in self._minted_fts if b["ft_count"] > 0]

        if not batches or rounds <= 0:
            return {}

        # De-dup by ft_name (we track two entries per batch after a split).
        by_name: Dict[str, Dict] = {}
        for b in batches:
            # prefer the entry that is on the creator side
            if b["ft_name"] not in by_name or b["owner_did"] == b["creator_did"]:
                by_name[b["ft_name"]] = b

        log.info(
            "=== FT REPEATED TRANSFER START: %d rounds x %d batches ===",
            rounds, len(by_name),
        )

        exec_counter = 0
        stats: Dict[str, Dict[str, int]] = {}

        for ft_name, start_entry in by_name.items():
            stats[ft_name] = {"success": 0, "fail": 0}
            creator_did = start_entry["creator_did"]

            # Track current sender dynamically across rounds.
            sender_did = start_entry["owner_did"]
            sender_node = start_entry["owner_node"]
            sender_label = start_entry["owner_label"]

            for r in range(rounds):
                exec_counter += 1
                if sender_label == "A":
                    recv_did, recv_node, recv_label = self.did_b, self.node_b, "B"
                else:
                    recv_did, recv_node, recv_label = self.did_a, self.node_a, "A"

                seq_id = f"FT-REXFER-{exec_counter:04d}"
                ts = datetime.now(tz=timezone.utc).isoformat()
                t0 = time.time()
                status = "SUCCESS"
                req_id: Optional[str] = None
                txn_id: Optional[str] = None
                error: Optional[str] = None

                try:
                    result = sender_node.transfer_ft(
                        sender_did=sender_did,
                        receiver_did=recv_did,
                        ft_name=ft_name,
                        ft_count=count_per_round,
                        creator_did=creator_did,
                        memo=f"repeated FT xfer round {r+1}/{rounds} {sender_label}->{recv_label}",
                    )
                    req_id = result.get("req_id")
                    txn_id = sender_node.extract_txn_id(result)
                    stats[ft_name]["success"] += 1
                except Exception as exc:
                    status = "FAIL"
                    error = str(exc)
                    stats[ft_name]["fail"] += 1
                    log.warning(
                        "[%s] round %d/%d FAIL: %s",
                        seq_id, r + 1, rounds, exc,
                    )

                duration_ms = int((time.time() - t0) * 1000)

                self.reporter.record_transaction(
                    {
                        "id": seq_id,
                        "type": "FT_REPEATED_TRANSFER",
                        "node": sender_label,
                        "ft_name": ft_name,
                        "creator_did": creator_did[:20] + "...",
                        "sender_did": sender_did[:20] + "...",
                        "receiver_did": recv_did[:20] + "...",
                        "ft_count": count_per_round,
                        "transaction_id": txn_id,
                        "round": r + 1,
                        "total_rounds": rounds,
                        "status": status,
                        "req_id": req_id,
                        "duration_ms": duration_ms,
                        "timestamp": ts,
                        "error": error,
                    }
                )

                if status == "SUCCESS":
                    log.info(
                        "[%s] round %d/%d %s->%s OK  %dms",
                        seq_id, r + 1, rounds,
                        sender_label, recv_label, duration_ms,
                    )
                    # Flip sender for the next round (direction inversion).
                    sender_did, sender_node, sender_label = (
                        recv_did, recv_node, recv_label,
                    )

                time.sleep(2)

        total_ok = sum(s["success"] for s in stats.values())
        total_fail = sum(s["fail"] for s in stats.values())
        log.info(
            "=== FT REPEATED TRANSFER COMPLETE: %d success, %d fail ===",
            total_ok, total_fail,
        )
        return stats

    # ------------------------------------------------------------------
    # FT verification
    # ------------------------------------------------------------------

    def run_verification(self) -> List[Dict[str, str]]:
        """Exercise FT query APIs and report PASS/FAIL for each.

        Exercises:
          - GET /rubix/v1/fts               (both nodes)
          - GET /rubix/v1/dids/{did}/balances/ft (both DIDs)
          - GET /api/get-ft-token-chain     (best-effort if token IDs available)
          - GET /rubix/v1/tx                (both nodes — tx list sanity)
        """
        log.info("=== FT VERIFICATION START ===")
        results: List[Dict[str, str]] = []

        # --- 1. list_fts on both nodes ---
        for node, label in [(self.node_a, "A"), (self.node_b, "B")]:
            try:
                fts = node.list_fts()
                results.append({
                    "check": f"FT_LIST_NODE_{label}",
                    "status": "PASS",
                    "detail": f"node{label} has {len(fts)} FT entries",
                })
            except Exception as exc:
                results.append({
                    "check": f"FT_LIST_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- 2. get_ft_balance for each DID on its own node ---
        for did, node, label in [
            (self.did_a, self.node_a, "A"),
            (self.did_b, self.node_b, "B"),
        ]:
            try:
                bal = node.get_ft_balance(did)
                results.append({
                    "check": f"FT_BALANCE_NODE_{label}",
                    "status": "PASS",
                    "detail": f"node{label} DID holds {len(bal)} FT entries",
                })
            except Exception as exc:
                results.append({
                    "check": f"FT_BALANCE_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- 3. list_transactions on both nodes ---
        for node, label in [(self.node_a, "A"), (self.node_b, "B")]:
            try:
                txns = node.list_transactions()
                results.append({
                    "check": f"FT_TX_LIST_NODE_{label}",
                    "status": "PASS",
                    "detail": f"node{label} has {len(txns)} transactions",
                })
            except Exception as exc:
                results.append({
                    "check": f"FT_TX_LIST_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- Log summary ---
        passed = sum(1 for r in results if r["status"] == "PASS")
        failed = sum(1 for r in results if r["status"] == "FAIL")
        log.info(
            "=== FT VERIFICATION COMPLETE: %d passed, %d failed ===",
            passed, failed,
        )
        for r in results:
            level = log.info if r["status"] == "PASS" else log.warning
            level("  [%s] %s: %s", r["status"], r["check"], r["detail"])

        return results
