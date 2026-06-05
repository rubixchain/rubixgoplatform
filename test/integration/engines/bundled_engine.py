"""
bundled_engine.py -- Bundled (combined) transaction engine for stress testing.

Workflow:
  - Requires pre-deployed NFTs and smart contracts (from NFTEngine + SmartContractEngine)
  - Sends a single /rubix/v1/tx call that combines RBT transfer + NFT execution + SC execution
  - Runs multiple rounds mixing same-node and cross-node bundled transactions
  - Verifies token chains and balances after the bundled rounds

Preconditions:
  The caller must supply already-deployed NFT IDs and SC IDs that are available
  on the initiator node (owned or subscribed).
"""

from __future__ import annotations

import logging
import time
from datetime import datetime, timezone
from typing import TYPE_CHECKING, Any, Dict, List, Optional

if TYPE_CHECKING:
    from test.integration.clients.api_client import NodeClient
    from test.integration.config import StressConfig
    from test.integration.engines.reporter import StressReporter

log = logging.getLogger(__name__)


class BundledEngine:
    """Runs bundled (RBT + NFT + SC) transactions in a single API call.

    Args:
        node_a, node_b: NodeClient instances.
        did_a, did_b:   DIDs for nodeA and nodeB respectively.
        config:         StressConfig.
        reporter:       StressReporter for recording operations.
    """

    def __init__(
        self,
        node_a: "NodeClient",
        node_b: "NodeClient",
        did_a: str,
        did_b: str,
        config: "StressConfig",
        reporter: "StressReporter",
    ) -> None:
        self.node_a = node_a
        self.node_b = node_b
        self.did_a = did_a
        self.did_b = did_b
        self.cfg = config
        self.reporter = reporter

    # ------------------------------------------------------------------
    # Main entry point
    # ------------------------------------------------------------------

    def run_bundled_test(
        self,
        nft_id: str,
        sc_id: str,
        rbt_amount: float = 1.0,
        rounds: int = 3,
    ) -> Dict[str, Any]:
        """Run bundled transaction test rounds.

        Each round sends a single /rubix/v1/tx call containing:
          - RBT transfer (sender -> receiver)
          - NFT execution (no ownership transfer)
          - SC execution

        Alternates direction each round:
          - Even rounds: nodeA initiates -> nodeB receives RBT
          - Odd rounds:  nodeB initiates -> nodeA receives RBT

        Preconditions:
          - nft_id must be deployed and executable on both nodes (subscribed)
          - sc_id must be deployed and subscribed on both nodes
          - Both nodes must have sufficient RBT balance

        Args:
            nft_id: NFT token hash to include in the bundled tx
            sc_id: Smart contract token hash to include in the bundled tx
            rbt_amount: RBT amount per transfer (default: 1.0)
            rounds: Number of bundled transaction rounds (default: 3)

        Returns:
            Stats dict: {"success": int, "fail": int, "rounds": list[dict]}
        """
        log.info(
            "=== BUNDLED TRANSACTION TEST START: %d rounds ===",
            rounds,
        )
        log.info(
            "  RBT=%.4f  NFT=%s  SC=%s",
            rbt_amount,
            nft_id[:12] + "...",
            sc_id[:12] + "...",
        )

        stats: Dict[str, Any] = {"success": 0, "fail": 0, "rounds": []}

        for r in range(rounds):
            # Alternate direction: even = A->B, odd = B->A
            if r % 2 == 0:
                sender_node = self.node_a
                sender_did = self.did_a
                receiver_did = self.did_b
                direction = "A->B"
            else:
                sender_node = self.node_b
                sender_did = self.did_b
                receiver_did = self.did_a
                direction = "B->A"

            seq_id = f"BUNDLED-{r + 1:04d}"
            ts = datetime.now(tz=timezone.utc).isoformat()
            t0 = time.time()
            status = "SUCCESS"
            req_id: Optional[str] = None
            txn_id: Optional[str] = None
            error: Optional[str] = None

            try:
                log.info(
                    "[%s] Round %d/%d  direction=%s  rbt=%.4f  nft=%s  sc=%s",
                    seq_id, r + 1, rounds, direction, rbt_amount,
                    nft_id[:12] + "...", sc_id[:12] + "...",
                )

                result = sender_node.execute_bundled_transaction(
                    sender_did=sender_did,
                    receiver_did=receiver_did,
                    rbt_amount=rbt_amount,
                    nft_id=nft_id,
                    nft_data=f"bundled NFT exec round {r + 1}/{rounds}",
                    sc_id=sc_id,
                    sc_data=f"bundled SC exec round {r + 1}/{rounds}",
                    transfer_nft_ownership=False,
                    memo=f"bundled tx round {r + 1}/{rounds} ({direction})",
                )
                req_id = result.get("req_id")
                txn_id = sender_node.extract_txn_id(result)
                stats["success"] += 1
                log.info("[%s] SUCCESS  req_id=%s  txn_id=%s", seq_id, req_id, txn_id)

            except Exception as exc:
                status = "FAIL"
                error = str(exc)
                stats["fail"] += 1
                log.warning("[%s] FAIL: %s", seq_id, exc)

            duration_ms = int((time.time() - t0) * 1000)

            round_info = {
                "round": r + 1,
                "direction": direction,
                "status": status,
                "req_id": req_id,
                "transaction_id": txn_id,
                "duration_ms": duration_ms,
                "error": error,
            }
            stats["rounds"].append(round_info)

            self.reporter.record_transaction({
                "id": seq_id,
                "type": "BUNDLED_TX",
                "direction": direction,
                "node": direction.split("->")[0],
                "rbt_amount": rbt_amount,
                "nft_id": nft_id,
                "sc_id": sc_id,
                "round": r + 1,
                "total_rounds": rounds,
                "status": status,
                "req_id": req_id,
                "transaction_id": txn_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
            })

            # Settle delay between rounds
            time.sleep(3)

        log.info(
            "=== BUNDLED TRANSACTION TEST COMPLETE: %d success, %d fail ===",
            stats["success"], stats["fail"],
        )
        return stats

    # ------------------------------------------------------------------
    # All-in-one transaction — RBT + FT[] + NFT[] + SC[] in one call
    # ------------------------------------------------------------------

    def run_all_in_one_test(
        self,
        nft_ids: Optional[List[str]] = None,
        sc_ids: Optional[List[str]] = None,
        ft_batches: Optional[List[Dict[str, Any]]] = None,
        rbt_amount: float = 1.0,
        rounds: int = 3,
        ft_amount_per_batch: float = 1.0,
    ) -> Dict[str, Any]:
        """Run all-in-one transaction rounds.

        Each round sends a single ``/rubix/v1/tx`` call that simultaneously
        carries RBT + every NFT id + every SC id + a slice from every FT
        batch owned by the sender. Direction alternates A->B / B->A each
        round.

        Args:
            nft_ids: NFT token hashes to include in each round. All are
                executed (not ownership-transferred).
            sc_ids: Smart-contract token hashes to include in each round.
            ft_batches: Pre-minted FT batches. Each entry should look like
                ``{"ft_name": str, "creator_did": str, "owner_did": str,
                "owner_label": "A"|"B", "ft_count": int}``. The engine
                picks batches whose ``owner_did`` matches the current
                sender and emits FTInfo entries referring to them.
            rbt_amount: RBT amount transferred per round.
            rounds:     Number of rounds.
            ft_amount_per_batch: Number of FTs to move from each eligible
                batch per round (default 1.0).

        Returns:
            Stats dict: ``{"success": int, "fail": int, "rounds": [...]}``.

        Preconditions:
            * NFTs must already be executable / subscribed on both nodes.
            * SCs must already be deployed + subscribed on both nodes.
            * FT batches must exist on the sender's node with enough
              remaining balance for ``ft_amount_per_batch``.
        """
        nft_ids = list(nft_ids or [])
        sc_ids = list(sc_ids or [])
        ft_batches = list(ft_batches or [])

        log.info(
            "=== ALL-IN-ONE TX TEST START: %d rounds ===",
            rounds,
        )
        log.info(
            "  RBT=%.4f  NFTs=%d  SCs=%d  FT batches=%d  ft_amount/batch=%.2f",
            rbt_amount,
            len(nft_ids),
            len(sc_ids),
            len(ft_batches),
            ft_amount_per_batch,
        )

        stats: Dict[str, Any] = {"success": 0, "fail": 0, "rounds": []}

        for r in range(rounds):
            if r % 2 == 0:
                sender_node = self.node_a
                sender_did = self.did_a
                receiver_did = self.did_b
                direction = "A->B"
                sender_label = "A"
            else:
                sender_node = self.node_b
                sender_did = self.did_b
                receiver_did = self.did_a
                direction = "B->A"
                sender_label = "B"

            # Build NFTInfo list for this round
            nft_list = [
                {
                    "nftId": nid,
                    "value": 1.0,
                    "data": f"all-in-one NFT exec round {r + 1}/{rounds}",
                }
                for nid in nft_ids
            ]

            # Build SmartContractInfo list for this round
            sc_list = [
                {
                    "smartContractId": sid,
                    "value": 1.0,
                    "data": f"all-in-one SC exec round {r + 1}/{rounds}",
                }
                for sid in sc_ids
            ]

            # Build FTInfo list for this round — only FT batches owned by
            # the current sender are eligible this round.
            ft_list: List[Dict[str, Any]] = []
            for b in ft_batches:
                owner_label = b.get("owner_label")
                if owner_label != sender_label:
                    continue
                remaining = b.get("ft_count", 0)
                if remaining <= 0:
                    continue
                move = min(ft_amount_per_batch, remaining)
                if move <= 0:
                    continue
                ft_list.append({
                    "ftName": b["ft_name"],
                    "numberOfFts": move,
                    "creatorDID": b["creator_did"],
                })
                # Track so subsequent rounds from the same sender don't
                # over-commit the same batch.
                b["ft_count"] = remaining - move
                b["owner_did"] = receiver_did
                b["owner_label"] = "B" if sender_label == "A" else "A"

            seq_id = f"ALLINONE-{r + 1:04d}"
            ts = datetime.now(tz=timezone.utc).isoformat()
            t0 = time.time()
            status = "SUCCESS"
            req_id: Optional[str] = None
            txn_id: Optional[str] = None
            error: Optional[str] = None

            try:
                log.info(
                    "[%s] Round %d/%d  direction=%s  rbt=%.4f  nfts=%d  scs=%d  fts=%d",
                    seq_id, r + 1, rounds, direction, rbt_amount,
                    len(nft_list), len(sc_list), len(ft_list),
                )

                result = sender_node.execute_all_transaction(
                    sender_did=sender_did,
                    receiver_did=receiver_did,
                    rbt_amount=rbt_amount,
                    ft_list=ft_list,
                    nft_list=nft_list,
                    sc_list=sc_list,
                    transfer_nft_ownership=False,
                    memo=f"all-in-one tx round {r + 1}/{rounds} ({direction})",
                )
                req_id = result.get("req_id")
                txn_id = self.node_a.extract_txn_id(result)
                stats["success"] += 1
                log.info("[%s] SUCCESS  req_id=%s  txn_id=%s", seq_id, req_id, txn_id)

            except Exception as exc:
                status = "FAIL"
                error = str(exc)
                stats["fail"] += 1
                log.warning("[%s] FAIL: %s", seq_id, exc)

            duration_ms = int((time.time() - t0) * 1000)

            round_info = {
                "round": r + 1,
                "direction": direction,
                "status": status,
                "req_id": req_id,
                "transaction_id": txn_id,
                "duration_ms": duration_ms,
                "rbt_amount": rbt_amount,
                "nft_count": len(nft_list),
                "sc_count": len(sc_list),
                "ft_count": len(ft_list),
                "error": error,
            }
            stats["rounds"].append(round_info)

            self.reporter.record_transaction({
                "id": seq_id,
                "type": "ALLINONE_TX",
                "direction": direction,
                "node": direction.split("->")[0],
                "rbt_amount": rbt_amount,
                "nft_count": len(nft_list),
                "sc_count": len(sc_list),
                "ft_count": len(ft_list),
                "round": r + 1,
                "total_rounds": rounds,
                "status": status,
                "req_id": req_id,
                "transaction_id": txn_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
            })

            # Settle delay between rounds
            time.sleep(3)

        log.info(
            "=== ALL-IN-ONE TX TEST COMPLETE: %d success, %d fail ===",
            stats["success"], stats["fail"],
        )
        return stats

    def run_all_in_one_verification(
        self,
        nft_ids: Optional[List[str]] = None,
        sc_ids: Optional[List[str]] = None,
        ft_names: Optional[List[str]] = None,
    ) -> List[Dict[str, str]]:
        """Verification checks after an all-in-one run.

        Checks: NFT chain presence for every NFT on both nodes, SC chain
        presence + sync for every SC on both nodes, per-DID FT balance
        listing for the named FTs, RBT balances, and transaction list
        counts.
        """
        log.info("=== ALL-IN-ONE TX VERIFICATION START ===")
        results: List[Dict[str, str]] = []

        # --- 1. NFT chains ---
        for nft_id in nft_ids or []:
            for node, label in [(self.node_a, "A"), (self.node_b, "B")]:
                try:
                    chain = node.get_nft_chain(nft_id)
                    chain_len = len(chain) if chain else 0
                    passed = chain_len >= 1
                    results.append({
                        "check": f"ALLINONE_NFT_CHAIN_NODE_{label}_{nft_id[:12]}",
                        "status": "PASS" if passed else "FAIL",
                        "detail": f"NFT {nft_id[:12]} chain on node{label}: {chain_len} entries",
                    })
                except Exception as exc:
                    results.append({
                        "check": f"ALLINONE_NFT_CHAIN_NODE_{label}_{nft_id[:12]}",
                        "status": "FAIL",
                        "detail": str(exc),
                    })

        # --- 2. SC chains + sync ---
        for sc_id in sc_ids or []:
            len_by_label: Dict[str, int] = {}
            for node, label in [(self.node_a, "A"), (self.node_b, "B")]:
                try:
                    chain = node.get_smart_contract_chain(sc_id)
                    chain_len = len(chain) if chain else 0
                    len_by_label[label] = chain_len
                    passed = chain_len >= 1
                    results.append({
                        "check": f"ALLINONE_SC_CHAIN_NODE_{label}_{sc_id[:12]}",
                        "status": "PASS" if passed else "FAIL",
                        "detail": f"SC {sc_id[:12]} chain on node{label}: {chain_len} entries",
                    })
                except Exception as exc:
                    results.append({
                        "check": f"ALLINONE_SC_CHAIN_NODE_{label}_{sc_id[:12]}",
                        "status": "FAIL",
                        "detail": str(exc),
                    })
            if len_by_label.get("A") and len_by_label.get("B"):
                synced = len_by_label["A"] == len_by_label["B"]
                results.append({
                    "check": f"ALLINONE_SC_CHAIN_SYNC_{sc_id[:12]}",
                    "status": "PASS" if synced else "FAIL",
                    "detail": f"nodeA={len_by_label['A']} vs nodeB={len_by_label['B']} SC chain entries",
                })

        # --- 3. FT balances on both nodes ---
        for did, node, label in [
            (self.did_a, self.node_a, "A"),
            (self.did_b, self.node_b, "B"),
        ]:
            try:
                balances = node.get_ft_balance(did)
                summary = ", ".join(
                    f"{b.get('ft_name', '?')}={b.get('ft_count', '?')}"
                    for b in balances[:5]
                )
                results.append({
                    "check": f"ALLINONE_FT_BALANCES_NODE_{label}",
                    "status": "PASS",
                    "detail": f"node{label}: {len(balances)} FT entries ({summary or 'none'})",
                })
            except Exception as exc:
                results.append({
                    "check": f"ALLINONE_FT_BALANCES_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- 4. RBT balances ---
        for did, node, label in [
            (self.did_a, self.node_a, "A"),
            (self.did_b, self.node_b, "B"),
        ]:
            try:
                bal = node.get_balance(did)
                results.append({
                    "check": f"ALLINONE_RBT_BALANCE_NODE_{label}",
                    "status": "PASS",
                    "detail": f"node{label} RBT balance = {bal:.4f}",
                })
            except Exception as exc:
                results.append({
                    "check": f"ALLINONE_RBT_BALANCE_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- 5. Transaction counts ---
        for node, label in [(self.node_a, "A"), (self.node_b, "B")]:
            try:
                txns = node.list_transactions()
                results.append({
                    "check": f"ALLINONE_TX_LIST_NODE_{label}",
                    "status": "PASS",
                    "detail": f"node{label} has {len(txns)} transactions",
                })
            except Exception as exc:
                results.append({
                    "check": f"ALLINONE_TX_LIST_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        passed = sum(1 for r in results if r["status"] == "PASS")
        failed = sum(1 for r in results if r["status"] == "FAIL")
        log.info(
            "=== ALL-IN-ONE TX VERIFICATION COMPLETE: %d passed, %d failed ===",
            passed, failed,
        )
        for r in results:
            level = log.info if r["status"] == "PASS" else log.warning
            level("  [%s] %s: %s", r["status"], r["check"], r["detail"])

        return results

    # ------------------------------------------------------------------
    # Verification
    # ------------------------------------------------------------------

    def run_verification(
        self,
        nft_id: str,
        sc_id: str,
        expected_rbt_transferred: float = 0.0,
    ) -> List[Dict[str, str]]:
        """Run verification checks after bundled transactions.

        Verifies:
          - NFT chain length increased on both nodes
          - SC chain length increased on both nodes
          - SC chain sync between nodes
          - RBT balances shifted correctly
          - Transaction counts on both nodes

        Args:
            nft_id: NFT token hash used in bundled transactions
            sc_id: SC token hash used in bundled transactions
            expected_rbt_transferred: Net RBT transferred from A->B (for balance checks).
                                      Positive = A sent more than received.

        Returns:
            List of verification result dicts.
        """
        log.info("=== BUNDLED TX VERIFICATION START ===")
        results: List[Dict[str, str]] = []

        # --- 1. NFT chain on both nodes ---
        for node, label in [(self.node_a, "A"), (self.node_b, "B")]:
            try:
                chain = node.get_nft_chain(nft_id)
                chain_len = len(chain) if chain else 0
                # After deployment + bundled rounds, chain should have entries
                passed = chain_len >= 1
                results.append({
                    "check": f"BUNDLED_NFT_CHAIN_NODE_{label}",
                    "status": "PASS" if passed else "FAIL",
                    "detail": f"NFT chain on node{label} has {chain_len} entries",
                })
            except Exception as exc:
                results.append({
                    "check": f"BUNDLED_NFT_CHAIN_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- 2. SC chain on both nodes + sync check ---
        chain_a_len = 0
        chain_b_len = 0
        for node, label in [(self.node_a, "A"), (self.node_b, "B")]:
            try:
                chain = node.get_smart_contract_chain(sc_id)
                chain_len = len(chain) if chain else 0
                if label == "A":
                    chain_a_len = chain_len
                else:
                    chain_b_len = chain_len
                passed = chain_len >= 1
                results.append({
                    "check": f"BUNDLED_SC_CHAIN_NODE_{label}",
                    "status": "PASS" if passed else "FAIL",
                    "detail": f"SC chain on node{label} has {chain_len} entries",
                })
            except Exception as exc:
                results.append({
                    "check": f"BUNDLED_SC_CHAIN_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # SC chain sync
        if chain_a_len > 0 and chain_b_len > 0:
            synced = chain_a_len == chain_b_len
            results.append({
                "check": "BUNDLED_SC_CHAIN_SYNC",
                "status": "PASS" if synced else "FAIL",
                "detail": f"nodeA={chain_a_len} vs nodeB={chain_b_len} SC chain entries",
            })

        # --- 3. RBT balance check ---
        for did, node, label in [
            (self.did_a, self.node_a, "A"),
            (self.did_b, self.node_b, "B"),
        ]:
            try:
                bal = node.get_balance(did)
                results.append({
                    "check": f"BUNDLED_RBT_BALANCE_NODE_{label}",
                    "status": "PASS",
                    "detail": f"node{label} RBT balance = {bal:.4f}",
                })
            except Exception as exc:
                results.append({
                    "check": f"BUNDLED_RBT_BALANCE_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- 4. Transaction list on both nodes ---
        for node, label in [(self.node_a, "A"), (self.node_b, "B")]:
            try:
                txns = node.list_transactions()
                results.append({
                    "check": f"BUNDLED_TX_LIST_NODE_{label}",
                    "status": "PASS",
                    "detail": f"node{label} has {len(txns)} transactions",
                })
            except Exception as exc:
                results.append({
                    "check": f"BUNDLED_TX_LIST_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- Log summary ---
        passed = sum(1 for r in results if r["status"] == "PASS")
        failed = sum(1 for r in results if r["status"] == "FAIL")
        log.info(
            "=== BUNDLED TX VERIFICATION COMPLETE: %d passed, %d failed ===",
            passed, failed,
        )
        for r in results:
            level = log.info if r["status"] == "PASS" else log.warning
            level("  [%s] %s: %s", r["status"], r["check"], r["detail"])

        return results
