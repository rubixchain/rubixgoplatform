"""
intra_node_engine.py — Intra-node two-DID stress test.

Purpose
-------
Verify that two DIDs that live on the *same* node can exchange every kind of
asset (RBT, FT, NFT execute, SC execute) through the normal quorum protocol.
This is structurally different from the shuttle tests (which swap assets
between nodeA and nodeB): here both the initiator and the owner share a
single node's wallet, so the only party that changes during a transaction
is the quorum.

Workflow
--------
1. Bootstrap: create a brand-new ``did_a2`` on ``nodeA``. Register it, then
   broadcast ``add_peer_details`` to nodeB + quorum so that the quorum
   protocol can resolve the new DID to nodeA's peer.
2. Fund ``did_a2`` with a small RBT allowance from ``did_a`` (intra-node RBT
   transfer via ``POST /rubix/v1/tx``).
3. RBT back-and-forth: alternately send ``did_a``→``did_a2`` and
   ``did_a2``→``did_a`` for N rounds.
4. Fund ``did_a2`` with a slice of one of the existing minted FT batches
   (whose ``creator_did == did_a``).
5. FT back-and-forth: same alternating pattern, preserving ``creator_did``
   across every hop.
6. NFT: ``did_a2`` creates a fresh NFT, self-deploys it (A2→A2), then
   self-executes it. Both calls are routed through ``nodeA``.
7. SC: ``did_a2`` generates a fresh smart contract, deploys it, then
   executes it with ``did_a2`` as the initiator — all on ``nodeA``.
8. Verification: chain reads + balance reads on every asset to confirm the
   secondary DID sees the updates.

Every step is recorded to the shared :class:`StressReporter` under the
``INTRA_NODE_*`` type namespace.
"""

from __future__ import annotations

import logging
import time
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional, TYPE_CHECKING

from test.integration.engines.file_selector import select_nft_files, select_smart_contract_files

if TYPE_CHECKING:
    from test.integration.clients.api_client import NodeClient
    from test.integration.clients.db_validator import DBValidator
    from test.integration.config import StressConfig
    from test.integration.engines.reporter import StressReporter

log = logging.getLogger(__name__)


class IntraNodeEngine:
    """Two-DID, single-node exerciser covering RBT / FT / NFT / SC."""

    # ------------------------------------------------------------------
    # Construction
    # ------------------------------------------------------------------

    def __init__(
        self,
        *,
        node_a: "NodeClient",
        node_b: "NodeClient",
        quorum: "NodeClient",
        primary_did: str,
        peer_b_id: str,
        peer_q_id: str,
        did_b: str,
        did_q: str,
        config: "StressConfig",
        reporter: "StressReporter",
        password: str,
    ) -> None:
        self.node_a = node_a
        self.node_b = node_b
        self.quorum = quorum
        self.primary_did = primary_did
        self.peer_b_id = peer_b_id
        self.peer_q_id = peer_q_id
        self.did_b = did_b
        self.did_q = did_q
        self.cfg = config
        self.reporter = reporter
        self.password = password

        self.secondary_did: Optional[str] = None
        self._secondary_peer_id: Optional[str] = None

        self._counter = 0
        self._deployed_nft_id: Optional[str] = None
        self._deployed_sc_id: Optional[str] = None
        self._funded_ft_batch: Optional[Dict[str, Any]] = None

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------

    def _next_seq(self, prefix: str) -> str:
        self._counter += 1
        return f"INTRA-{prefix}-{self._counter:05d}"

    def _record(
        self,
        *,
        seq_id: str,
        tx_type: str,
        status: str,
        t0: float,
        ts: str,
        req_id: Optional[str] = None,
        transaction_id: Optional[str] = None,
        error: Optional[str] = None,
        **extra: Any,
    ) -> None:
        self.reporter.record_transaction(
            {
                "id": seq_id,
                "type": tx_type,
                "node": "A",
                "status": status,
                "req_id": req_id,
                "transaction_id": transaction_id,
                "duration_ms": int((time.time() - t0) * 1000),
                "timestamp": ts,
                "error": error,
                **extra,
            }
        )

    # ------------------------------------------------------------------
    # Step 1: bootstrap second DID on nodeA
    # ------------------------------------------------------------------

    def setup_secondary_did(self) -> str:
        """Create + register a second DID on nodeA and cross-register it."""
        log.info("=== INTRA-NODE SETUP: creating secondary DID on nodeA ===")

        seq_id = self._next_seq("DID")
        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()

        result = self.node_a.create_did(self.password)
        self.secondary_did = result["did"]
        self._secondary_peer_id = result.get("peer_id")

        log.info(
            "Secondary DID created: did_a2=%s  peer_id=%s",
            self.secondary_did,
            self._secondary_peer_id,
        )

        # Give IPFS a moment to propagate the new DID object before RegisterDID.
        log.info("Waiting 5 seconds for IPFS propagation before RegisterDID...")
        time.sleep(5)
        self.node_a.register_did(self.secondary_did)

        # Cross-register the new DID on the peers so the quorum can resolve
        # it during subsequent token operations. peer_id is nodeA's — the
        # DID lives on nodeA but the other nodes need the did→peerID map.
        peer_a = self.node_a.get_peer_id()
        log.info(
            "Cross-registering did_a2=%s  peer_a=%s on nodeB and quorum",
            self.secondary_did,
            peer_a,
        )
        self.node_b.add_peer_details(peer_a, self.secondary_did)
        self.quorum.add_peer_details(peer_a, self.secondary_did)

        self._record(
            seq_id=seq_id,
            tx_type="INTRA_NODE_SETUP",
            status="SUCCESS",
            t0=t0,
            ts=ts,
            did=self.secondary_did[:20] + "...",
            detail="second DID created + registered + broadcast",
        )

        log.info("=== INTRA-NODE SETUP COMPLETE ===")
        return self.secondary_did

    # ------------------------------------------------------------------
    # Step 2+3: RBT funding and back-and-forth
    # ------------------------------------------------------------------

    def _fire_rbt(
        self,
        *,
        sender_did: str,
        receiver_did: str,
        amount: float,
        direction_label: str,
    ) -> None:
        seq_id = self._next_seq("RBT")
        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            result = self.node_a.transfer_rbt(
                sender_did=sender_did,
                receiver_did=receiver_did,
                amount=amount,
                password=self.password,
            )
            req_id = result.get("req_id") if isinstance(result, dict) else None
            txn_id = self.node_a.extract_txn_id(result)
        except Exception as exc:
            status = "FAIL"
            error = str(exc)
            log.error("[%s] intra-node RBT %s FAILED: %s", seq_id, direction_label, exc)

        self._record(
            seq_id=seq_id,
            tx_type="INTRA_NODE_RBT",
            status=status,
            t0=t0,
            ts=ts,
            req_id=req_id,
            transaction_id=txn_id,
            error=error,
            direction=direction_label,
            amount=amount,
            sender_did=sender_did[:20] + "...",
            receiver_did=receiver_did[:20] + "...",
        )

        if status == "SUCCESS":
            log.info(
                "[%s] INTRA-NODE RBT %s  amount=%.3f  req=%s",
                seq_id,
                direction_label,
                amount,
                req_id,
            )

    def run_rbt_phase(self, *, fund_amount: float = 5.0, rounds: int = 3, per_round: float = 1.0) -> None:
        """Fund did_a2 then bounce RBT back-and-forth for ``rounds`` rounds."""
        assert self.secondary_did, "call setup_secondary_did() first"

        log.info(
            "=== INTRA-NODE RBT: fund=%.3f  rounds=%d  per_round=%.3f ===",
            fund_amount, rounds, per_round,
        )

        # Initial fund — did_a -> did_a2
        self._fire_rbt(
            sender_did=self.primary_did,
            receiver_did=self.secondary_did,
            amount=fund_amount,
            direction_label="A->A2_FUND",
        )

        # Let the funding settle before ping-ponging
        time.sleep(2)

        for rnd in range(1, rounds + 1):
            # A -> A2
            self._fire_rbt(
                sender_did=self.primary_did,
                receiver_did=self.secondary_did,
                amount=per_round,
                direction_label=f"A->A2_R{rnd}",
            )
            time.sleep(1)
            # A2 -> A
            self._fire_rbt(
                sender_did=self.secondary_did,
                receiver_did=self.primary_did,
                amount=per_round,
                direction_label=f"A2->A_R{rnd}",
            )
            time.sleep(1)

        log.info("=== INTRA-NODE RBT COMPLETE ===")

    # ------------------------------------------------------------------
    # Step 4+5: FT funding and back-and-forth
    # ------------------------------------------------------------------

    def _fire_ft(
        self,
        *,
        sender_did: str,
        receiver_did: str,
        ft_name: str,
        creator_did: str,
        ft_count: int,
        direction_label: str,
    ) -> None:
        seq_id = self._next_seq("FT")
        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            result = self.node_a.transfer_ft(
                sender_did=sender_did,
                receiver_did=receiver_did,
                ft_name=ft_name,
                ft_count=ft_count,
                creator_did=creator_did,
                memo=f"intra-node FT {direction_label}",
                password=self.password,
            )
            req_id = result.get("req_id") if isinstance(result, dict) else None
            txn_id = self.node_a.extract_txn_id(result)
        except Exception as exc:
            status = "FAIL"
            error = str(exc)
            log.error("[%s] intra-node FT %s FAILED: %s", seq_id, direction_label, exc)

        self._record(
            seq_id=seq_id,
            tx_type="INTRA_NODE_FT",
            status=status,
            t0=t0,
            ts=ts,
            req_id=req_id,
            transaction_id=txn_id,
            error=error,
            direction=direction_label,
            ft_name=ft_name,
            ft_count=ft_count,
            sender_did=sender_did[:20] + "...",
            receiver_did=receiver_did[:20] + "...",
        )

        if status == "SUCCESS":
            log.info(
                "[%s] INTRA-NODE FT %s  %d x %s  req=%s",
                seq_id,
                direction_label,
                ft_count,
                ft_name,
                req_id,
            )

    def run_ft_phase(
        self,
        *,
        ft_batch: Dict[str, Any],
        fund_count: int = 2,
        rounds: int = 2,
        per_round: int = 1,
        pre_fund_settle: int = 15,
    ) -> None:
        """Fund did_a2 with ``fund_count`` FTs and bounce ``per_round`` tokens
        back-and-forth for ``rounds`` rounds.

        ``ft_batch`` is a dict from :class:`FTEngine._minted_fts` — needs
        ``ft_name`` and ``creator_did``.

        ``pre_fund_settle`` waits before the first A->A2 fund so the quorum's
        synced copy of the FT token chain catches up to the initiator's head.
        Without it, on slow runners the fund transfer fails with
        ``TokenChainIntegrityCheck: ... chain mismatch after sync`` because the
        FT was minted/transferred in the preceding FT phase and the quorum's
        chain head lags. Fast dev machines don't hit this; CI does.
        """
        assert self.secondary_did, "call setup_secondary_did() first"
        if not ft_batch:
            log.info("=== INTRA-NODE FT SKIPPED (no FT batch supplied) ===")
            return
        if ft_batch.get("creator_did") != self.primary_did:
            log.warning(
                "FT batch %s is not minted by did_a (creator=%s). Skipping intra-node FT.",
                ft_batch.get("ft_name"), ft_batch.get("creator_did"),
            )
            return

        ft_name = ft_batch["ft_name"]
        creator_did = ft_batch["creator_did"]

        log.info(
            "=== INTRA-NODE FT: batch=%s  fund=%d  rounds=%d  per_round=%d ===",
            ft_name, fund_count, rounds, per_round,
        )

        # Let the quorum's synced FT token chain catch up to the initiator's
        # head before funding, so the fund transfer's TokenChainIntegrityCheck
        # doesn't fail on a stale chain head (see docstring).
        if pre_fund_settle > 0:
            log.info(
                "Settling %ds for FT '%s' token chain to sync on quorum before fund…",
                pre_fund_settle, ft_name,
            )
            time.sleep(pre_fund_settle)

        # Initial fund — A -> A2
        self._fire_ft(
            sender_did=self.primary_did,
            receiver_did=self.secondary_did,
            ft_name=ft_name,
            creator_did=creator_did,
            ft_count=fund_count,
            direction_label="A->A2_FUND",
        )
        self._funded_ft_batch = {
            "ft_name": ft_name,
            "creator_did": creator_did,
            "funded_count": fund_count,
        }

        time.sleep(3)  # FT mints/transfers need a bit longer to settle

        for rnd in range(1, rounds + 1):
            # A2 -> A (return what we funded, piece by piece)
            self._fire_ft(
                sender_did=self.secondary_did,
                receiver_did=self.primary_did,
                ft_name=ft_name,
                creator_did=creator_did,
                ft_count=per_round,
                direction_label=f"A2->A_R{rnd}",
            )
            time.sleep(2)
            # A -> A2 (top up again for symmetric flow)
            self._fire_ft(
                sender_did=self.primary_did,
                receiver_did=self.secondary_did,
                ft_name=ft_name,
                creator_did=creator_did,
                ft_count=per_round,
                direction_label=f"A->A2_R{rnd}",
            )
            time.sleep(2)

        log.info("=== INTRA-NODE FT COMPLETE ===")

    # ------------------------------------------------------------------
    # Step 6: NFT deploy + self-execute by did_a2
    # ------------------------------------------------------------------

    def run_nft_phase(self) -> None:
        """Let did_a2 create, deploy, and execute an NFT on nodeA."""
        assert self.secondary_did, "call setup_secondary_did() first"
        log.info("=== INTRA-NODE NFT (did_a2 self-deploy + self-execute) ===")

        seq_id = self._next_seq("NFT")
        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        nft_id: Optional[str] = None
        deploy_req_id: Optional[str] = None
        deploy_txn_id: Optional[str] = None
        exec_req_id: Optional[str] = None
        exec_txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            metadata_path, artifact_path = select_nft_files()

            # 1. Create NFT owned by did_a2
            result = self.node_a.create_nft(self.secondary_did, metadata_path, artifact_path)
            nft_id = result.get("nft_id")

            if not nft_id:
                raise RuntimeError("create_nft returned no nft_id")

            # 2. Self-deploy (A2 -> A2)
            deploy_result = self.node_a.transfer_nft(
                sender_did=self.secondary_did,
                receiver_did=self.secondary_did,
                nft_id=nft_id,
                data="intra-node NFT deploy (did_a2)",
            )
            deploy_req_id = deploy_result.get("req_id")
            deploy_txn_id = self.node_a.extract_txn_id(deploy_result)

            # 3. Settle, then self-execute
            time.sleep(3)

            exec_result = self.node_a.execute_nft(
                executor_did=self.secondary_did,
                nft_id=nft_id,
                nft_value=1.0,
                data="intra-node NFT execute (did_a2)",
                password=self.password,
            )
            exec_req_id = exec_result.get("req_id")
            exec_txn_id = self.node_a.extract_txn_id(exec_result)

            self._deployed_nft_id = nft_id
        except Exception as exc:
            status = "FAIL"
            error = str(exc)
            log.error("[%s] intra-node NFT flow FAILED: %s", seq_id, exc)

        self._record(
            seq_id=seq_id,
            tx_type="INTRA_NODE_NFT",
            status=status,
            t0=t0,
            ts=ts,
            req_id=exec_req_id,
            transaction_id=exec_txn_id,
            deploy_transaction_id=deploy_txn_id,
            error=error,
            nft_id=nft_id,
            deploy_req_id=deploy_req_id,
            owner_did=self.secondary_did[:20] + "..." if self.secondary_did else None,
        )

        if status == "SUCCESS":
            log.info(
                "[%s] INTRA-NODE NFT  nft=%s  deploy_req=%s  exec_req=%s",
                seq_id,
                (nft_id or "")[:12] + "...",
                deploy_req_id,
                exec_req_id,
            )

        log.info("=== INTRA-NODE NFT COMPLETE ===")

    # ------------------------------------------------------------------
    # Step 7: SC deploy + self-execute by did_a2
    # ------------------------------------------------------------------

    def run_sc_phase(self) -> None:
        """Let did_a2 generate, deploy, and execute a smart contract on nodeA."""
        assert self.secondary_did, "call setup_secondary_did() first"
        log.info("=== INTRA-NODE SC (did_a2 deploy + self-execute) ===")

        seq_id = self._next_seq("SC")
        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        sc_id: Optional[str] = None
        deploy_req_id: Optional[str] = None
        deploy_txn_id: Optional[str] = None
        exec_req_id: Optional[str] = None
        exec_txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            wasm_path, source_path = select_smart_contract_files()

            # 1. Generate SC (owned by did_a2)
            result = self.node_a.create_smart_contract(self.secondary_did, wasm_path, source_path)
            sc_id = result.get("smartContractId")
            if not sc_id:
                raise RuntimeError("create_smart_contract returned no smartContractId")

            # 2. Deploy (first execute) under did_a2
            deploy_result = self.node_a.deploy_smart_contract(
                initiator_did=self.secondary_did,
                sc_id=sc_id,
                data="intra-node SC deploy (did_a2)",
                password=self.password,
            )
            deploy_req_id = deploy_result.get("req_id")
            deploy_txn_id = self.node_a.extract_txn_id(deploy_result)

            # 3. Settle then execute again as did_a2
            time.sleep(3)

            exec_result = self.node_a.execute_smart_contract(
                executor_did=self.secondary_did,
                sc_id=sc_id,
                data="intra-node SC execute (did_a2)",
                password=self.password,
            )
            exec_req_id = exec_result.get("req_id")
            exec_txn_id = self.node_a.extract_txn_id(exec_result)

            self._deployed_sc_id = sc_id
        except Exception as exc:
            status = "FAIL"
            error = str(exc)
            log.error("[%s] intra-node SC flow FAILED: %s", seq_id, exc)

        self._record(
            seq_id=seq_id,
            tx_type="INTRA_NODE_SC",
            status=status,
            t0=t0,
            ts=ts,
            req_id=exec_req_id,
            transaction_id=exec_txn_id,
            deploy_transaction_id=deploy_txn_id,
            error=error,
            sc_id=sc_id,
            deploy_req_id=deploy_req_id,
            initiator_did=self.secondary_did[:20] + "..." if self.secondary_did else None,
        )

        if status == "SUCCESS":
            log.info(
                "[%s] INTRA-NODE SC  sc=%s  deploy_req=%s  exec_req=%s",
                seq_id,
                (sc_id or "")[:12] + "...",
                deploy_req_id,
                exec_req_id,
            )

        log.info("=== INTRA-NODE SC COMPLETE ===")

    # ------------------------------------------------------------------
    # Verification
    # ------------------------------------------------------------------

    def run_verification(self) -> List[Dict[str, str]]:
        """Collect post-run assertions for the intra-node subsystem."""
        results: List[Dict[str, str]] = []
        if not self.secondary_did:
            results.append({
                "check": "intra_node.secondary_did_created",
                "status": "FAIL",
                "detail": "secondary DID was never created",
            })
            return results

        results.append({
            "check": "intra_node.secondary_did_created",
            "status": "PASS",
            "detail": f"did_a2={self.secondary_did[:20]}...",
        })

        # Primary + secondary RBT balances on nodeA
        try:
            bal_primary = self.node_a.get_balance(self.primary_did)
            bal_secondary = self.node_a.get_balance(self.secondary_did)
            results.append({
                "check": "intra_node.rbt_balances",
                "status": "PASS" if (bal_primary is not None and bal_secondary is not None) else "FAIL",
                "detail": f"did_a={bal_primary:.3f}  did_a2={bal_secondary:.3f}",
            })
        except Exception as exc:
            results.append({
                "check": "intra_node.rbt_balances",
                "status": "FAIL",
                "detail": f"get_balance error: {exc}",
            })

        # NOTE: the intra-node FT balance check is intentionally NOT here.
        # Intra-node (same-node, two-DID) FT settlement to the secondary DID's
        # balance view is slow — the FT does arrive (confirmed via /balances/ft
        # and the fts/ft_tokens DB tables), but crediting did_a2 can lag past a
        # 60s poll. Reading it inline here races the settlement and produces a
        # spurious WARN. Instead it's deferred to verify_ft_balance_deferred(),
        # which the runner calls at the very END of the run so settlement has
        # had the maximum possible elapsed time.

        # NFT chain read on nodeA
        if self._deployed_nft_id:
            try:
                chain = self.node_a.get_nft_chain(self._deployed_nft_id) or []
                results.append({
                    "check": "intra_node.nft_chain",
                    "status": "PASS" if chain else "FAIL",
                    "detail": f"nft_id={self._deployed_nft_id[:12]}...  entries={len(chain)}",
                })
            except Exception as exc:
                results.append({
                    "check": "intra_node.nft_chain",
                    "status": "FAIL",
                    "detail": f"get_nft_chain error: {exc}",
                })

        # SC chain read on nodeA
        if self._deployed_sc_id:
            try:
                chain = self.node_a.get_smart_contract_chain(self._deployed_sc_id) or []
                results.append({
                    "check": "intra_node.sc_chain",
                    "status": "PASS" if chain else "FAIL",
                    "detail": f"sc_id={self._deployed_sc_id[:12]}...  entries={len(chain)}",
                })
            except Exception as exc:
                results.append({
                    "check": "intra_node.sc_chain",
                    "status": "FAIL",
                    "detail": f"get_smart_contract_chain error: {exc}",
                })

        return results

    # FT-balance API response keys. GET /rubix/v1/dids/{did}/balances/ft returns
    # result[] of types.FTBalance, whose JSON tags are name/creator/value/count
    # (see types/balance.go) — NOT ft_name/FTName. Reading the wrong key was the
    # ROOT CAUSE of the long-standing intra-node FT "settlement lag" WARN: the API
    # returned the FT correctly, but the harness matched on a non-existent key and
    # always saw 0. (Confirmed 2026-06-05 against the live API + product struct.)
    _FT_NAME_KEYS = ("name", "ft_name", "FTName")
    _FT_COUNT_KEYS = ("count", "ft_count", "FTCount")

    @staticmethod
    def _ft_row_name(row: Dict[str, Any]) -> Optional[str]:
        for k in IntraNodeEngine._FT_NAME_KEYS:
            if row.get(k):
                return row.get(k)
        return None

    @staticmethod
    def _ft_row_count(row: Dict[str, Any]) -> int:
        for k in IntraNodeEngine._FT_COUNT_KEYS:
            if row.get(k) is not None:
                try:
                    return int(row.get(k))
                except (TypeError, ValueError):
                    return 0
        return 0

    def verify_ft_balance_deferred(self, db_a: Optional["DBValidator"] = None) -> List[Dict[str, str]]:
        """Deferred FT-ownership assertion for did_a2 (call at END of the run).

        Asserts the funded FT batch landed on the secondary DID. The PRIMARY
        source is the balance API (GET /rubix/v1/dids/{did}/balances/ft) read with
        the CORRECT response keys (name/count — see _FT_NAME_KEYS). The API works;
        an earlier version matched on ft_name/FTName (keys that don't exist in the
        response), which always read 0 and was misdiagnosed as "settlement lag".

        When *db_a* is provided, the tokens table (token_type='ft') is used as an
        independent cross-check. PASS if EITHER source shows the FT (they should
        agree); FAIL only if both agree it's absent. Without db_a, falls back to an
        API-only poll.
        """
        results: List[Dict[str, str]] = []
        if not (self.secondary_did and self._funded_ft_batch):
            return results
        ft_name = self._funded_ft_batch["ft_name"]

        # Primary: balance API, read with the correct keys, with a short poll.
        api_count = 0
        try:
            for _attempt in range(6):  # ~30s confirm-poll
                ft_balances = self.node_a.get_ft_balance(self.secondary_did) or []
                api_count = sum(
                    self._ft_row_count(row) for row in ft_balances
                    if self._ft_row_name(row) == ft_name
                )
                if api_count > 0:
                    break
                time.sleep(5)
        except Exception as exc:  # noqa: BLE001
            api_count = -1  # API read failed; rely on DB cross-check below
            log.warning("[intra-node] FT balance API error: %s", exc)

        # Independent cross-check: committed FT ownership in the tokens table.
        db_count: Optional[int] = None
        if db_a is not None:
            try:
                db_count = db_a.get_did_ft_token_count(self.secondary_did, free_only=True)
            except Exception as exc:  # noqa: BLE001
                log.warning("[intra-node] FT DB cross-check error: %s", exc)
                db_count = None

        api_ok = api_count > 0
        db_ok = db_count is not None and db_count > 0
        present = api_ok or db_ok

        detail = f"did_a2 {ft_name}: API count={api_count}"
        if db_count is not None:
            detail += f", DB FT tokens={db_count}"
        if not present:
            detail += " — FT not found on did_a2 (API and DB both 0)"

        results.append({
            "check": f"intra_node.ft_balance[{ft_name}]",
            "status": "PASS" if present else "FAIL",
            "detail": detail,
        })
        return results
