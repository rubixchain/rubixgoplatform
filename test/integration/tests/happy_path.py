"""
happy_path.py — StressRunner: the happy-path integration suite.

Orchestrates the full lifecycle against a 3-node cluster (nodeA, nodeB,
quorum) provided by an Environment (see env/). The cluster lifecycle
(docker up/down, or external connect) is owned by the caller in runner.py;
this module only drives the nodes and asserts correctness.

Workflow:
  1. DID creation, peer cross-registration, quorum setup
  2. Batch mint nodeA tokens
  3. Batch mint quorum tokens
  4. Run shuttle phases (sequential + optional parallel levels)
  5. (OPTIONAL) NFT / Smart Contract / FT / bundled / all-in-one / intra-node
  6. Write summary + verification.json
  7. Dump DB snapshot queries to db_snapshot_<ts>.txt

Verification results are aggregated into verification.json; the public
`verification_failed` count lets the entry point (runner.py) map a failed
check to a non-zero process exit so CI catches assertion failures.

Invoked via the entry point:
  python3 -m test.integration.runner --run-all-tests
  python3 -m test.integration.runner --micro-test --run-all-tests
"""

from __future__ import annotations

import json
import logging
import os
import sys
import time
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional, Tuple

import psycopg2

from test.integration.clients.api_client import NodeClient
from test.integration.clients.db_validator import (
    DBValidator,
    FullnodeDBValidator,
    check_transactions_persisted,
)
from test.integration.config import StressConfig
from test.integration.engines.minter import BatchMinter
from test.integration.engines.nft_engine import NFTEngine
from test.integration.engines.reporter import StressReporter
from test.integration.engines.shuttle import ShuttleEngine

# DB snapshots are written under the run's output_dir (a run artifact, not
# committed source) — see StressRunner.run_db_snapshot, which uses
# self.cfg.output_dir.

# Settle time (seconds) inserted between major phases when the runner is
# instructed to execute every subsystem strictly sequentially (e.g. under
# --run-all-tests). Gives the chain / quorum / DB a chance to commit all
# writes from the phase that just finished before the next phase touches
# the same DIDs or token ledger.
_PHASE_SETTLE_SECONDS = 5

log = logging.getLogger(__name__)


class StressRunner:
    """Orchestrates the full stress run lifecycle."""

    def __init__(self, config: StressConfig) -> None:
        self.cfg = config

        self.node_a = NodeClient(
            f"http://localhost:{config.node_a_port}", "nodeA", config.password
        )
        self.node_b = NodeClient(
            f"http://localhost:{config.node_b_port}", "nodeB", config.password
        )
        self.quorum = NodeClient(
            f"http://localhost:{config.quorum_port}", "quorum", config.password
        )

        self.db_a = DBValidator(
            host="localhost", port=config.db_a_port, dbname="rubix_a"
        )
        self.db_b = DBValidator(
            host="localhost", port=config.db_b_port, dbname="rubix_b"
        )
        self.db_q = DBValidator(
            host="localhost", port=config.db_q_port, dbname="rubix_q"
        )

        # Optional 4th node, only reachable when the harness was started with
        # --fullnode-test (compose profile `fullnode`). Constructed
        # unconditionally — they are lazy: nothing connects until a check runs.
        self.fullnode = NodeClient(
            f"http://localhost:{config.fullnode_port}", "fullnode", config.password
        )
        self.db_fullnode = FullnodeDBValidator(
            host="localhost", port=config.db_fullnode_port, dbname="rubix_f"
        )
        # Set by runner.py when the environment can control the fullnode
        # container (docker exec / logs / restart). None otherwise.
        self.fullnode_controller: Optional[Any] = None
        # {service: ContainerController} for the transacting nodes, so the
        # fullnode suite can confirm the gossipsub mesh from the publisher side.
        self.publisher_controllers: Dict[str, Any] = {}
        self.fullnode_engine = None

        self.reporter = StressReporter(output_dir=config.output_dir)

        self.did_a: Optional[str] = None
        self.did_b: Optional[str] = None
        self.did_q: Optional[str] = None
        # Optional secondary DID on nodeA for intra-node two-DID tests.
        self.did_a2: Optional[str] = None
        # Intra-node engine kept so its deferred FT-balance check can run at
        # end of the run (intra-node FT settlement lags).
        self._intra_node_engine = None

        # Number of verification checks that FAILed in the last run. The entry
        # point (runner.py) reads this to set a non-zero exit code so CI catches
        # assertion failures (the run itself does not raise on a failed check).
        self.verification_failed: int = 0

    # ------------------------------------------------------------------
    # Run-state persistence (for --skip-setup / resume after --no-teardown)
    # ------------------------------------------------------------------

    def _state_path(self) -> str:
        return os.path.join(self.cfg.output_dir, "run_state.json")

    def _save_state(self) -> None:
        """Persist DIDs and mint indexes to run_state.json so a subsequent run can reuse them."""
        os.makedirs(self.cfg.output_dir, exist_ok=True)
        # Load existing state to preserve fields not being updated
        existing: Dict[str, Any] = {}
        if os.path.exists(self._state_path()):
            with open(self._state_path(), encoding="utf-8") as fh:
                existing = json.load(fh)
        existing.update({
            "did_a": self.did_a,
            "did_b": self.did_b,
            "did_q": self.did_q,
            "did_a2": self.did_a2,
        })
        with open(self._state_path(), "w", encoding="utf-8") as fh:
            json.dump(existing, fh, indent=2)
        log.info("Run state saved to %s", self._state_path())

    def _save_mint_state(self) -> None:
        """Persist next mint indexes to run_state.json after minting completes."""
        os.makedirs(self.cfg.output_dir, exist_ok=True)
        existing: Dict[str, Any] = {}
        if os.path.exists(self._state_path()):
            with open(self._state_path(), encoding="utf-8") as fh:
                existing = json.load(fh)
        existing.update({
            "next_nodeA_index": self.cfg.node_a_mint.end_index,
            "next_quorum_index": self.cfg.quorum_mint.end_index,
        })
        with open(self._state_path(), "w", encoding="utf-8") as fh:
            json.dump(existing, fh, indent=2)
        log.info(
            "Mint state saved: next_nodeA_index=%d  next_quorum_index=%d",
            self.cfg.node_a_mint.end_index, self.cfg.quorum_mint.end_index,
        )

    def _load_state(self) -> None:
        """Restore DIDs from run_state.json written by a previous run."""
        path = self._state_path()
        if not os.path.exists(path):
            raise FileNotFoundError(
                f"--skip-setup requires a prior run state at {path!r}. "
                "Run once without --skip-setup to generate it."
            )
        with open(path, encoding="utf-8") as fh:
            state = json.load(fh)
        self.did_a = state["did_a"]
        self.did_b = state["did_b"]
        self.did_q = state["did_q"]
        self.did_a2 = state.get("did_a2")
        log.info(
            "Loaded state from %s: did_a=%s  did_b=%s  did_q=%s  did_a2=%s",
            path, self.did_a, self.did_b, self.did_q, self.did_a2,
        )

    # ------------------------------------------------------------------
    # Step 1: Node bootstrap
    # ------------------------------------------------------------------

    def setup_nodes(self) -> None:
        """Create DIDs, register peers, configure quorum."""
        log.info("=== SETUP: Creating DIDs ===")
        result_a = self.node_a.create_did(self.cfg.password)
        result_b = self.node_b.create_did(self.cfg.password)
        result_q = self.quorum.create_did(self.cfg.password)

        self.did_a = result_a["did"]
        self.did_b = result_b["did"]
        self.did_q = result_q["did"]

        log.info(
            "DIDs: nodeA=%s  nodeB=%s  quorum=%s",
            self.did_a, self.did_b, self.did_q,
        )

        log.info("=== SETUP: Fetching peer IDs ===")
        peer_a = self.node_a.get_peer_id()
        peer_b = self.node_b.get_peer_id()
        peer_q = self.quorum.get_peer_id()

        log.info(
            "Peer IDs: nodeA=%s  nodeB=%s  quorum=%s", peer_a, peer_b, peer_q
        )

        log.info("=== SETUP: Configuring quorum ===")
        self.quorum.setup_quorum(self.did_q, self.cfg.password)
        self.node_a.add_quorum(self.did_q)
        self.node_b.add_quorum(self.did_q)

        log.info("=== SETUP: Registering DIDs (post-setup, all nodes ready) ===")
        log.info("Waiting 10 seconds for IPFS content propagation before RegisterDID...")
        time.sleep(10)
        self.node_a.register_did(self.did_a)
        self.node_b.register_did(self.did_b)
        self.quorum.register_did(self.did_q)

        # Validate peer IDs before cross-registering
        for label, pid in [("nodeA", peer_a), ("nodeB", peer_b), ("quorum", peer_q)]:
            assert pid and pid.startswith("12D3KooW") and len(pid) == 52, \
                f"{label} peer_id is invalid: {pid!r}"
        for label, d in [("nodeA", self.did_a), ("nodeB", self.did_b), ("quorum", self.did_q)]:
            assert d and d.startswith("bafybmi") and len(d) == 59, \
                f"{label} DID is invalid: {d!r}"

        log.info("=== SETUP: Cross-registering peers (add_peer_details) ===")
        self.node_a.add_peer_details(peer_b, self.did_b)
        self.node_a.add_peer_details(peer_q, self.did_q)
        self.node_b.add_peer_details(peer_a, self.did_a)
        self.node_b.add_peer_details(peer_q, self.did_q)
        self.quorum.add_peer_details(peer_a, self.did_a)
        self.quorum.add_peer_details(peer_b, self.did_b)

        log.info("=== SETUP COMPLETE ===")
        self._save_state()

    # ------------------------------------------------------------------
    # Step 2: Token minting
    # ------------------------------------------------------------------

    def _auto_advance_mint_indexes(self) -> None:
        """If run_state.json has saved mint indexes, advance config to avoid collisions."""
        path = self._state_path()
        if not os.path.exists(path):
            return

        with open(path, encoding="utf-8") as fh:
            state = json.load(fh)

        next_a = state.get("next_nodeA_index")
        next_q = state.get("next_quorum_index")

        if next_a is not None:
            token_count = self.cfg.node_a_mint.total_tokens
            old_start = self.cfg.node_a_mint.start_index
            self.cfg.node_a_mint.start_index = next_a
            self.cfg.node_a_mint.end_index = next_a + token_count
            log.info(
                "Auto-advanced nodeA mint index: %d -> [%d, %d) (%d tokens)",
                old_start, next_a, next_a + token_count, token_count,
            )

        if next_q is not None:
            token_count = self.cfg.quorum_mint.total_tokens
            old_start = self.cfg.quorum_mint.start_index
            self.cfg.quorum_mint.start_index = next_q
            self.cfg.quorum_mint.end_index = next_q + token_count
            log.info(
                "Auto-advanced quorum mint index: %d -> [%d, %d) (%d tokens)",
                old_start, next_q, next_q + token_count, token_count,
            )

    def mint_tokens(self) -> None:
        """Mint nodeA and quorum tokens in batches, logging timing.

        Auto-advances mint indexes from run_state.json if available,
        then saves the new end indexes back after minting completes.
        """
        self._auto_advance_mint_indexes()
        os.makedirs(self.cfg.output_dir, exist_ok=True)

        log.info("=== MINTING: nodeA (%d tokens) ===", self.cfg.node_a_mint.total_tokens)
        minter_a = BatchMinter(
            node_client=self.node_a,
            did=self.did_a,
            mint_config=self.cfg.node_a_mint,
            output_dir=self.cfg.output_dir,
            node_label="nodeA",
        )
        elapsed_a = minter_a.run()
        log.info("nodeA minting done in %.1fs", elapsed_a)

        bal_a = self.node_a.get_balance(self.did_a)
        log.info("nodeA balance after mint: %.0f RBT", bal_a)

        log.info("=== MINTING: quorum (%d tokens) ===", self.cfg.quorum_mint.total_tokens)
        minter_q = BatchMinter(
            node_client=self.quorum,
            did=self.did_q,
            mint_config=self.cfg.quorum_mint,
            output_dir=self.cfg.output_dir,
            node_label="quorum",
        )
        elapsed_q = minter_q.run()
        log.info("quorum minting done in %.1fs", elapsed_q)

        bal_q = self.quorum.get_balance(self.did_q)
        log.info("quorum balance after mint: %.0f RBT", bal_q)

        self._save_mint_state()
        log.info("=== MINTING COMPLETE ===")

    # ------------------------------------------------------------------
    # Step 3: Shuttle run
    # ------------------------------------------------------------------

    def run_shuttle(self, decimal_transfers: bool = False) -> List[Dict[str, str]]:
        """Run all configured shuttle phases.

        Returns a reconciliation verification that proves the RECEIVER actually
        got the tokens: it snapshots nodeA/nodeB RBT balances before and after
        the shuttle, sums the net A<->B amount the (successful) transfers should
        have moved, and asserts each node's measured balance delta matches that
        net within a small tolerance. This closes the gap where a transfer's API
        success doesn't by itself prove the receiver's balance synced.
        """
        assert self.did_a and self.did_b, "DIDs not initialised — run setup_nodes first"

        log.info("=== SHUTTLE START ===")
        if decimal_transfers:
            log.info("Decimal transfers enabled (random amounts up to 3 decimal places)")
        log.info(self.cfg.summary())

        # Balance snapshot BEFORE the shuttle.
        bal_a_before = self.node_a.get_balance(self.did_a)
        bal_b_before = self.node_b.get_balance(self.did_b)
        tx_before = len(self.reporter._records)

        engine = ShuttleEngine(
            node_a=self.node_a,
            node_b=self.node_b,
            did_a=self.did_a,
            did_b=self.did_b,
            db_q=self.db_q,
            config=self.cfg,
            reporter=self.reporter,
            decimal_transfers=decimal_transfers,
        )
        engine.run_all_phases()

        log.info(
            "=== SHUTTLE COMPLETE: %d transactions  stop_reason=%s ===",
            engine.total_transactions,
            engine.stop_reason or "all phases done",
        )

        return self._verify_shuttle_reconciliation(bal_a_before, bal_b_before, tx_before)

    def _verify_shuttle_reconciliation(
        self, bal_a_before: float, bal_b_before: float, tx_before: int
    ) -> List[Dict[str, str]]:
        """Assert nodeA/nodeB balance deltas match the net shuttle transfer.

        Net A→B (sum of successful A→B amounts − successful B→A amounts) must
        equal: nodeA balance DECREASE and nodeB balance INCREASE. If the
        receiver didn't actually get its tokens, the measured delta won't match
        the recorded net and this FAILs.
        """
        results: List[Dict[str, str]] = []

        # Sum successful shuttle transfers recorded during this phase.
        shuttle_txns = [
            r for r in self.reporter._records[tx_before:]
            if r.get("status") == "SUCCESS" and r.get("from") in ("A", "B")
        ]
        net_a_to_b = 0.0
        for r in shuttle_txns:
            amt = float(r.get("amount", 0) or 0)
            if r.get("from") == "A" and r.get("to") == "B":
                net_a_to_b += amt
            elif r.get("from") == "B" and r.get("to") == "A":
                net_a_to_b -= amt

        if not shuttle_txns:
            results.append({
                "check": "RBT_RECONCILE_SHUTTLE",
                "status": "WARN",
                "detail": "no successful shuttle transfers to reconcile",
            })
            return results

        # Settle, then read final balances on BOTH nodes.
        time.sleep(5)
        bal_a_after = self.node_a.get_balance(self.did_a)
        bal_b_after = self.node_b.get_balance(self.did_b)

        delta_a = bal_a_before - bal_a_after   # nodeA should have DECREASED by net
        delta_b = bal_b_after - bal_b_before   # nodeB should have INCREASED by net

        # Tolerance: RBT fee/rounding on localnet is small; allow a little slack.
        tol = max(0.01, abs(net_a_to_b) * 0.001)
        a_ok = abs(delta_a - net_a_to_b) <= tol
        b_ok = abs(delta_b - net_a_to_b) <= tol
        a_tag = "ok" if a_ok else "MISMATCH"
        b_tag = "ok" if b_ok else "MISMATCH (receiver missing expected tokens)"

        detail = (
            f"net A->B={net_a_to_b:.4f}; "
            f"nodeA delta={delta_a:.4f} (expected {net_a_to_b:.4f}, {a_tag}); "
            f"nodeB delta={delta_b:.4f} (expected {net_a_to_b:.4f}, {b_tag})"
        )
        results.append({
            "check": "RBT_RECONCILE_SHUTTLE",
            "status": "PASS" if (a_ok and b_ok) else "FAIL",
            "detail": detail,
        })
        return results

    # ------------------------------------------------------------------
    # Read-only query endpoints (tx-history-by-DID, NFT children/parent)
    # ------------------------------------------------------------------

    def _verify_extra_apis(self) -> List[Dict[str, str]]:
        """Smoke-test query endpoints not exercised by the transfer/asset flows:

          - GET /rubix/v1/tx/{did}/{token_type}  (tx history by DID + type)
          - GET /rubix/v1/nfts/{nft_id}/children
          - GET /rubix/v1/nfts/{nft_id}/parent
          - GET /rubix/v1/dids/{did}/public_key

        tx-history: assert nodeA reports >=1 rbt transaction (the shuttle moved
        RBT from did_a), proving the endpoint returns the history we created.
        NFT children/parent: assert the endpoints respond successfully for a
        deployed NFT (our test NFTs have no hierarchy, so an empty-but-OK
        response is the expected, correct result — this is an endpoint smoke
        test, not a hierarchy assertion).
        public_key: assert both the owning node and a remote node resolve the
        SAME key for did_a. Because a DID is the IPFS hash of its public key,
        agreement across nodes is the real correctness property — whether the
        remote answered from its local cache or from IPFS.
        """
        results: List[Dict[str, str]] = []

        # --- public key by DID (local + cross-node agreement) ---
        results.extend(self._verify_public_key_api())

        # --- tx history by DID + token_type ---
        if self.did_a:
            try:
                rbt_hist = self.node_a.get_transactions_by_did(self.did_a, "rbt")
                results.append({
                    "check": "TX_HISTORY_BY_DID_RBT",
                    "status": "PASS" if len(rbt_hist) >= 1 else "FAIL",
                    "detail": (
                        f"did_a rbt tx history: {len(rbt_hist)} record(s)"
                        + ("" if rbt_hist else " (expected >=1 from shuttle transfers)")
                    ),
                })
            except Exception as exc:  # noqa: BLE001
                results.append({
                    "check": "TX_HISTORY_BY_DID_RBT",
                    "status": "FAIL",
                    "detail": f"get_transactions_by_did error: {exc}",
                })

        # --- NFT children + parent (endpoint smoke test) ---
        nft_id = None
        if hasattr(self, "nft_engine") and getattr(self.nft_engine, "_deployed_nfts", None):
            nft_id = self.nft_engine._deployed_nfts[0].get("nft_id")
        if nft_id:
            try:
                children = self.node_a.get_nft_children(nft_id)
                results.append({
                    "check": "NFT_CHILDREN_QUERY",
                    "status": "PASS",
                    "detail": f"nfts/{nft_id[:12]}.../children responded: {len(children)} child(ren)",
                })
            except Exception as exc:  # noqa: BLE001
                results.append({
                    "check": "NFT_CHILDREN_QUERY",
                    "status": "FAIL",
                    "detail": f"get_nft_children error: {exc}",
                })
            try:
                parent_resp = self.node_a.get_nft_parent(nft_id)
                ok = bool(parent_resp.get("status", False))
                results.append({
                    "check": "NFT_PARENT_QUERY",
                    "status": "PASS" if ok else "FAIL",
                    "detail": (
                        f"nfts/{nft_id[:12]}.../parent responded: "
                        f"status={ok}, result={'present' if parent_resp.get('result') else 'none'}"
                    ),
                })
            except Exception as exc:  # noqa: BLE001
                results.append({
                    "check": "NFT_PARENT_QUERY",
                    "status": "FAIL",
                    "detail": f"get_nft_parent error: {exc}",
                })

        return results

    def _verify_public_key_api(self) -> List[Dict[str, str]]:
        """Assert GET /rubix/v1/dids/{did}/public_key resolves did_a on both the
        owning node (local pubKey.pem) and a remote node (local cache or IPFS),
        and that the two agree byte-for-byte."""
        results: List[Dict[str, str]] = []
        if not self.did_a:
            return results

        try:
            own = self.node_a.get_public_key(self.did_a)
        except Exception as exc:  # noqa: BLE001
            return [{
                "check": "DID_PUBLIC_KEY_LOCAL",
                "status": "FAIL",
                "detail": f"nodeA get_public_key error: {exc}",
            }]

        own_key = own.get("public_key") or ""
        # 65-byte uncompressed secp256k1 point, hex-encoded.
        local_ok = len(own_key) == 130
        results.append({
            "check": "DID_PUBLIC_KEY_LOCAL",
            "status": "PASS" if local_ok else "FAIL",
            "detail": (
                f"nodeA resolved did_a key, len(hex)={len(own_key)}"
                + ("" if local_ok else " (expected 130 hex chars)")
            ),
        })

        # A remote node must resolve the SAME key — from its cached copy of the
        # DID directory, or straight from IPFS if it never fetched did_a.
        try:
            remote = self.node_b.get_public_key(self.did_a)
        except Exception as exc:  # noqa: BLE001
            results.append({
                "check": "DID_PUBLIC_KEY_CROSS_NODE",
                "status": "FAIL",
                "detail": f"nodeB get_public_key error: {exc}",
            })
            return results

        remote_key = remote.get("public_key") or ""
        match = bool(own_key) and own_key == remote_key
        results.append({
            "check": "DID_PUBLIC_KEY_CROSS_NODE",
            "status": "PASS" if match else "FAIL",
            "detail": (
                "nodeB resolved did_a key; "
                + ("matches nodeA" if match else "DIFFERS from nodeA's key")
            ),
        })
        return results

    # ------------------------------------------------------------------
    # Final assurance: every recorded transaction persisted on its nodes
    # ------------------------------------------------------------------

    def _verify_transactions_persisted(self) -> List[Dict[str, str]]:
        """Assert every recorded-SUCCESS txn is in the transactions table on
        each participating node (sender AND receiver).

        Final cross-node integrity gate: a transaction the harness saw succeed
        must be durably persisted on every node that took part. Uses the txn's
        own initiator/owner DIDs (read from the DB) to decide which nodes must
        hold it, so cross-node transfers require both ends and single-node
        operations require just the one. A missing row is a FAIL.
        """
        node_dbs = {"A": self.db_a, "B": self.db_b, "quorum": self.db_q}
        did_to_node: Dict[str, str] = {}
        for did, node in (
            (self.did_a, "A"),
            (self.did_a2, "A"),   # secondary intra-node DID lives on nodeA
            (self.did_b, "B"),
            (self.did_q, "quorum"),
        ):
            if did:
                did_to_node[did] = node

        result = check_transactions_persisted(
            self.reporter._records, node_dbs, did_to_node
        )

        if result["status"] == "PASS":
            detail = (
                f"{result['checked']} txns present on all participant nodes "
                f"({result['skipped']} skipped: no on-chain id)"
            )
        else:
            misses = result["missing"]
            sample = "; ".join(
                f"{m['transaction_id']}"
                + (" (not found on ANY node)" if m["not_found_anywhere"]
                   else f" (missing on {','.join(m['missing_on'])})")
                for m in misses[:5]
            )
            more = "" if len(misses) <= 5 else f" … +{len(misses) - 5} more"
            detail = (
                f"{len(misses)}/{result['checked']} txns not persisted on all "
                f"participant nodes: {sample}{more}"
            )

        return [{
            "check": "TX_PERSISTED_BOTH_NODES",
            "status": result["status"],
            "detail": detail,
        }]

    # ------------------------------------------------------------------
    # Step 4: NFT creation (optional)
    # ------------------------------------------------------------------

    def run_nft(self, nft_phases: List[Dict[str, Any]]) -> None:
        """Run NFT creation and deployment phases.

        NFTs are automatically deployed to blockchain after creation.

        Args:
            nft_phases: List of phase configs (same format as shuttle phases)
                        [{"name": "nft_sequential", "concurrency": 1, "tx_count": 10}, ...]
        """
        assert self.did_a and self.did_b, "DIDs not initialised — run setup_nodes first"

        if not nft_phases:
            log.info("=== NFT CREATION SKIPPED (no phases configured) ===")
            return

        log.info("=== NFT CREATION & DEPLOYMENT START ===")

        # Convert dict phases to PhaseConfig objects
        from test.integration.config import PhaseConfig
        phase_configs = [PhaseConfig.from_dict(p) for p in nft_phases]

        self.nft_engine = NFTEngine(
            node_a=self.node_a,
            node_b=self.node_b,
            did_a=self.did_a,
            did_b=self.did_b,
            config=self.cfg,
            reporter=self.reporter,
        )
        self.nft_engine.run_all_phases(phase_configs)

        log.info(
            "=== NFT CREATION & DEPLOYMENT COMPLETE: %d NFTs created and deployed ===",
            self.nft_engine.total_nfts,
        )

    # ------------------------------------------------------------------
    # Step 4b: FT creation & transfer (optional)
    # ------------------------------------------------------------------

    def run_ft(
        self,
        ft_phases: List[Dict[str, Any]],
        tokens_per_batch: int,
        rbt_per_batch: int,
        do_transfer: bool,
        transfer_rounds: int,
    ) -> List[Dict[str, str]]:
        """Run FT mint + optional transfer phases.

        Args:
            ft_phases: List of phase configs (same format as shuttle phases):
                       [{"name": "ft_sequential", "concurrency": 1, "tx_count": 5}, ...]
            tokens_per_batch: FTs created per mint op.
            rbt_per_batch:    RBT burned per mint op.
            do_transfer:      If True, run a single transfer pass after minting.
            transfer_rounds:  Additional alternating A<->B transfer rounds.

        Returns:
            List of verification result dicts.
        """
        assert self.did_a and self.did_b, "DIDs not initialised — run setup_nodes first"

        if not ft_phases:
            log.info("=== FT MINT SKIPPED (no phases configured) ===")
            return []

        log.info("=== FT MINT START ===")

        from test.integration.config import PhaseConfig
        from test.integration.engines.ft_engine import FTEngine

        phase_configs = [PhaseConfig.from_dict(p) for p in ft_phases]

        self.ft_engine = FTEngine(
            node_a=self.node_a,
            node_b=self.node_b,
            did_a=self.did_a,
            did_b=self.did_b,
            config=self.cfg,
            reporter=self.reporter,
            tokens_per_batch=tokens_per_batch,
            rbt_per_batch=rbt_per_batch,
        )
        self.ft_engine.run_all_phases(phase_configs)

        log.info(
            "=== FT MINT COMPLETE: %d mint operations ===",
            self.ft_engine.total_mints,
        )

        if do_transfer:
            log.info("=== FT TRANSFER START ===")
            self.ft_engine.run_ft_transfer()
            log.info("=== FT TRANSFER COMPLETE ===")

        if transfer_rounds > 0:
            log.info("=== FT REPEATED TRANSFERS START (%d rounds) ===", transfer_rounds)
            self.ft_engine.run_repeated_transfers(transfer_rounds)
            log.info("=== FT REPEATED TRANSFERS COMPLETE ===")

        log.info("=== FT API VERIFICATION START ===")
        results = self.ft_engine.run_verification()
        log.info("=== FT API VERIFICATION COMPLETE ===")
        return results

    def run_smart_contract(self, sc_count: int, sc_execute: bool) -> None:
        """Run smart contract deployment and optionally cross-node execution.

        Args:
            sc_count: Number of smart contracts to deploy from nodeA
            sc_execute: Whether to execute contracts from nodeB after deployment
        """
        assert self.did_a and self.did_b, "DIDs not initialised — run setup_nodes first"

        if sc_count <= 0:
            log.info("=== SMART CONTRACT SKIPPED (count = 0) ===")
            return

        log.info("=== SMART CONTRACT DEPLOYMENT & EXECUTION START ===")

        from test.integration.engines.smart_contract_engine import SmartContractEngine

        self.sc_engine = SmartContractEngine(
            node_a=self.node_a,
            node_b=self.node_b,
            did_a=self.did_a,
            did_b=self.did_b,
            config=self.cfg,
            reporter=self.reporter,
        )

        # Deploy smart contracts from nodeA
        self.sc_engine.run_deployment(sc_count)

        # Self-execute: Execute smart contract on the same node that deployed it
        if sc_execute:
            log.info("=== SMART CONTRACT SELF-EXECUTE START ===")
            self.sc_engine.run_self_execute()
            log.info("=== SMART CONTRACT SELF-EXECUTE COMPLETE ===")

        # Cross-node execution from nodeB if requested
        if sc_execute:
            log.info("=== SMART CONTRACT CROSS-NODE EXECUTION START ===")
            self.sc_engine.run_cross_node_execution()
            log.info("=== SMART CONTRACT CROSS-NODE EXECUTION COMPLETE ===")

        log.info(
            "=== SMART CONTRACT DEPLOYMENT & EXECUTION COMPLETE: %d contracts deployed ===",
            self.sc_engine.total_contracts,
        )

    # ------------------------------------------------------------------
    # DB Snapshot — post-shuttle diagnostic queries
    # ------------------------------------------------------------------

    _QUERY_1 = """
SELECT
    did,
    COUNT(*)                                  AS total,
    COUNT(*) FILTER (WHERE token_status = 0)  AS free,
    COUNT(*) FILTER (WHERE token_status = 6)  AS pledged,
    MIN(token_id)                             AS min_id,
    MAX(token_id)                             AS max_id
FROM tokens
GROUP BY did
ORDER BY total DESC;
"""

    _QUERY_2 = """
SELECT
    MIN(token_id)                                    AS min_token_id,
    MAX(token_id)                                    AS max_token_id,
    COUNT(*)                                         AS total_tokens,
    COUNT(*) FILTER (WHERE token_status = 0)         AS status_free,
    COUNT(*) FILTER (WHERE token_status = 6)         AS status_pledged,
    COUNT(*) FILTER (WHERE token_status = 99)        AS status_unknown,
    COUNT(DISTINCT did)                              AS distinct_owners,
    MIN(created_at)                                  AS first_minted,
    MAX(created_at)                                  AS last_minted
FROM tokens;
"""

    def _run_query_on_node(
        self,
        label: str,
        conn_params: Dict[str, Any],
        sql: str,
    ) -> Tuple[List[str], List[Tuple]]:
        """Execute *sql* on the given node and return (column_names, rows)."""
        try:
            with psycopg2.connect(**conn_params) as conn, conn.cursor() as cur:
                cur.execute(sql)
                col_names = [desc[0] for desc in cur.description]
                rows = cur.fetchall()
            return col_names, rows
        except Exception as exc:
            log.warning("[%s] DB snapshot query failed: %s", label, exc)
            return [], []

    def _format_table(
        self,
        label: str,
        col_names: List[str],
        rows: List[Tuple],
    ) -> str:
        """Render a simple fixed-width text table."""
        lines: List[str] = [f"--- {label} ---"]
        if not col_names:
            lines.append("  (no data / query error)")
            return "\n".join(lines)

        # Compute column widths
        widths = [len(c) for c in col_names]
        str_rows: List[List[str]] = []
        for row in rows:
            str_row = [str(v) if v is not None else "NULL" for v in row]
            str_rows.append(str_row)
            for i, val in enumerate(str_row):
                widths[i] = max(widths[i], len(val))

        sep = "+-" + "-+-".join("-" * w for w in widths) + "-+"
        header = "| " + " | ".join(c.ljust(widths[i]) for i, c in enumerate(col_names)) + " |"
        lines.append(sep)
        lines.append(header)
        lines.append(sep)
        for str_row in str_rows:
            lines.append("| " + " | ".join(v.ljust(widths[i]) for i, v in enumerate(str_row)) + " |")
        lines.append(sep)
        lines.append(f"  ({len(rows)} row{'s' if len(rows) != 1 else ''})")
        return "\n".join(lines)

    # ------------------------------------------------------------------
    # Intra-node two-DID test
    # ------------------------------------------------------------------

    def run_intra_node(
        self,
        *,
        rbt_rounds: int = 3,
        rbt_amount: float = 1.0,
        rbt_fund: float = 5.0,
        ft_rounds: int = 2,
        ft_amount: int = 1,
        ft_fund: int = 2,
    ) -> List[Dict[str, str]]:
        """Spin up a second DID on nodeA and exercise RBT / FT / NFT / SC
        between ``did_a`` and ``did_a2`` — everything strictly intra-node.

        Reuses ``FTEngine._minted_fts`` if present so we don't have to mint
        a fresh batch just for this test.
        """
        from test.integration.engines.intra_node_engine import IntraNodeEngine

        assert self.did_a and self.did_b and self.did_q, \
            "DIDs not initialised — run setup_nodes first"

        log.info("=== INTRA-NODE TEST START ===")

        peer_b = self.node_b.get_peer_id()
        peer_q = self.quorum.get_peer_id()

        engine = IntraNodeEngine(
            node_a=self.node_a,
            node_b=self.node_b,
            quorum=self.quorum,
            primary_did=self.did_a,
            peer_b_id=peer_b,
            peer_q_id=peer_q,
            did_b=self.did_b,
            did_q=self.did_q,
            config=self.cfg,
            reporter=self.reporter,
            password=self.cfg.password,
        )

        # 1. Second DID on nodeA
        engine.setup_secondary_did()

        # 2+3. RBT fund + back-and-forth between did_a and did_a2
        engine.run_rbt_phase(
            fund_amount=rbt_fund,
            rounds=rbt_rounds,
            per_round=rbt_amount,
        )

        # 4+5. FT back-and-forth (reuse an existing minted batch if we have one)
        ft_batch_for_intra: Optional[Dict[str, Any]] = None
        if hasattr(self, "ft_engine") and getattr(self.ft_engine, "_minted_fts", None):
            for b in self.ft_engine._minted_fts:
                if b.get("creator_did") == self.did_a and b.get("ft_count", 0) >= ft_fund:
                    ft_batch_for_intra = b
                    break

        if ft_batch_for_intra is not None:
            engine.run_ft_phase(
                ft_batch=ft_batch_for_intra,
                fund_count=ft_fund,
                rounds=ft_rounds,
                per_round=ft_amount,
            )
        else:
            log.info(
                "=== INTRA-NODE FT SKIPPED — "
                "no FT batch minted by did_a with >= %d tokens available ===",
                ft_fund,
            )

        # 6. NFT deploy + self-execute by did_a2
        engine.run_nft_phase()

        # 7. SC deploy + self-execute by did_a2
        engine.run_sc_phase()

        # 8. Verification
        log.info("=== INTRA-NODE VERIFICATION START ===")
        verification = engine.run_verification()
        log.info("=== INTRA-NODE VERIFICATION COMPLETE ===")

        # Keep the engine so the deferred intra-node FT balance check can run at
        # the very END of the run (intra-node FT settlement lags — see
        # IntraNodeEngine.verify_ft_balance_deferred).
        self._intra_node_engine = engine

        # Remember the second DID on the runner in case future phases want
        # to reuse it (also persists into run_state.json on next _save_state).
        self.did_a2 = engine.secondary_did
        try:
            self._save_state()
        except Exception:
            pass

        log.info("=== INTRA-NODE TEST COMPLETE ===")
        return verification

    # ------------------------------------------------------------------
    # Fullnode (-fullnode observer node)
    # ------------------------------------------------------------------

    def setup_fullnode(self) -> None:
        """Create the fullnode's DID and mesh it into every node's peer table.

        Called from run() immediately after setup_nodes() and BEFORE any
        transaction is published, because gossipsub delivers only to peers that
        are already subscribed — a fullnode that joins late silently misses
        everything published before it arrived.
        """
        from test.integration.engines.fullnode_engine import FullnodeEngine

        self.fullnode_engine = FullnodeEngine(
            fullnode=self.fullnode,
            fullnode_db=self.db_fullnode,
            node_a=self.node_a,
            node_b=self.node_b,
            quorum=self.quorum,
            did_a=self.did_a,
            did_b=self.did_b,
            did_q=self.did_q,
            controller=self.fullnode_controller,
            publisher_controllers=self.publisher_controllers,
            password=self.cfg.password,
        )
        self.fullnode_engine.setup()
        # Join the transaction topic's gossipsub mesh now, not at verification
        # time, so the fullnode observes the whole run.
        self.fullnode_engine.ensure_gossip_mesh()

    def run_fullnode(self, include_restart: bool = True) -> List[Dict[str, str]]:
        """Run the fullnode verification suite.

        Returns verification records; a FAIL here fails the CI run exactly like
        any other check (runner.py maps verification_failed to a non-zero exit).
        """
        if self.fullnode_engine is None:
            return [{
                "check": "FULLNODE_SUITE",
                "status": "FAIL",
                "detail": "--fullnode-test was requested but the fullnode engine "
                          "was never set up (setup_fullnode did not run)",
            }]
        log.info("=== FULLNODE VERIFICATION START ===")
        results = self.fullnode_engine.run_verification(include_restart=include_restart)
        log.info("=== FULLNODE VERIFICATION COMPLETE ===")
        return results

    def run_negative(self) -> List[Dict[str, str]]:
        """Run the negative / failure-path suite against nodeA <-> nodeB.

        Returns verification results (same shape as the happy-path engines) so
        they fold into verification.json and the runner's exit-on-fail gate.
        """
        from test.integration.tests.negative import NegativeEngine

        log.info("=== NEGATIVE: running failure-path tests ===")
        engine = NegativeEngine(
            node_a=self.node_a,
            node_b=self.node_b,
            did_a=self.did_a,
            did_b=self.did_b,
            password=self.cfg.password,
        )
        return engine.run()

    def run_db_snapshot(self) -> str:
        """Run diagnostic queries against all 3 DBs and write the results.

        Writes: {output_dir}/db_snapshot_<ISO-timestamp>.txt (a run artifact).
        Returns the path of the written file.
        """
        ts = datetime.now(tz=timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        snapshot_dir = self.cfg.output_dir
        out_path = os.path.join(snapshot_dir, f"db_snapshot_{ts}.txt")

        nodes = [
            ("nodeA", self.db_a.conn_params),
            ("nodeB", self.db_b.conn_params),
            ("quorum", self.db_q.conn_params),
        ]

        sections: List[str] = [
            f"Rubix DB Snapshot — {ts}",
            "=" * 72,
        ]

        for query_label, sql in [
            ("Query 1 — per-DID token distribution", self._QUERY_1),
            ("Query 2 — global token summary", self._QUERY_2),
        ]:
            sections.append(f"\n{'=' * 72}")
            sections.append(f"  {query_label}")
            sections.append(f"{'=' * 72}")
            sections.append(f"SQL:\n{sql.strip()}\n")
            for node_label, conn_params in nodes:
                col_names, rows = self._run_query_on_node(node_label, conn_params, sql)
                sections.append(self._format_table(node_label, col_names, rows))
                sections.append("")

        content = "\n".join(sections)
        os.makedirs(snapshot_dir, exist_ok=True)
        with open(out_path, "w", encoding="utf-8") as fh:
            fh.write(content)

        log.info("DB snapshot written to %s", out_path)
        return out_path

    # ------------------------------------------------------------------
    # Sequential phase pacing
    # ------------------------------------------------------------------

    def _settle_between_phases(
        self,
        finished_phase: str,
        next_phase: str,
        seconds: int = _PHASE_SETTLE_SECONDS,
    ) -> None:
        """Log a visible boundary and sleep *seconds* seconds.

        Used between the major test phases (shuttle -> NFT -> SC -> bundled
        -> FT -> all-in-one) when strict sequential ordering is requested
        so each subsystem's writes can fully commit on every node's
        backing DB / ledger before the next one starts.
        """
        log.info("-" * 72)
        log.info(
            ">>> PHASE BOUNDARY: '%s' DONE  →  settling %ds before '%s' <<<",
            finished_phase, seconds, next_phase,
        )
        log.info("-" * 72)
        time.sleep(seconds)

    # ------------------------------------------------------------------
    # Finalisation
    # ------------------------------------------------------------------

    def finalise(self) -> None:
        """Collect final balances and write summary."""
        final_balances = {}
        for label, client, did in [
            ("nodeA", self.node_a, self.did_a),
            ("nodeB", self.node_b, self.did_b),
            ("quorum", self.quorum, self.did_q),
        ]:
            try:
                bal = client.get_balance(did) if did else 0.0
                final_balances[label] = bal
            except Exception as exc:
                log.warning("Could not fetch final balance for %s: %s", label, exc)
                final_balances[label] = -1.0

        self.reporter.print_summary(final_balances)
        self.reporter.write_summary(final_balances)

    # ------------------------------------------------------------------
    # Verification summary
    # ------------------------------------------------------------------

    def _write_verification_summary(self, results: List[Dict[str, str]]) -> None:
        """Write API verification results to verification.json alongside other logs.

        Also logs a final summary line.
        """
        passed = sum(1 for r in results if r["status"] == "PASS")
        failed = sum(1 for r in results if r["status"] == "FAIL")
        total = len(results)

        # Record for the entry point so a failed check becomes a non-zero exit.
        self.verification_failed = failed

        summary = {
            "generated_at": datetime.now(tz=timezone.utc).isoformat(),
            "total_checks": total,
            "passed": passed,
            "failed": failed,
            "results": results,
        }

        os.makedirs(self.cfg.output_dir, exist_ok=True)
        out_path = os.path.join(self.cfg.output_dir, "verification.json")
        with open(out_path, "w", encoding="utf-8") as fh:
            json.dump(summary, fh, indent=2)

        log.info(
            "=== API VERIFICATION SUMMARY: %d/%d passed, %d failed ===",
            passed, total, failed,
        )
        if failed > 0:
            for r in results:
                if r["status"] == "FAIL":
                    log.warning("  FAIL: %s — %s", r["check"], r["detail"])

        log.info("Verification results written to %s", out_path)

    # ------------------------------------------------------------------
    # Full run
    # ------------------------------------------------------------------

    # v1 — preserved for reference
    # def run(self) -> None:
    #     self.setup_nodes()
    #     self.mint_tokens()
    #     self.run_shuttle()
    #     self.finalise()

    def run(
        self,
        skip_setup: bool = False,
        skip_mint: bool = False,
        nft_phases: Optional[List[Dict[str, Any]]] = None,
        nft_only: bool = False,
        nft_self_execute: bool = False,
        nft_transfer: bool = False,
        nft_cross_execute: bool = False,
        sc_count: int = 0,
        sc_execute: bool = False,
        sc_only: bool = False,
        exec_rounds: int = 0,
        bundled_test: bool = False,
        bundled_rounds: int = 3,
        bundled_rbt: float = 1.0,
        decimal_transfers: bool = False,
        ft_phases: Optional[List[Dict[str, Any]]] = None,
        ft_only: bool = False,
        ft_transfer: bool = False,
        ft_transfer_rounds: int = 0,
        ft_tokens_per_batch: int = 100,
        ft_rbt_per_batch: int = 10,
        all_in_one_test: bool = False,
        all_in_one_rounds: int = 3,
        all_in_one_rbt: float = 1.0,
        all_in_one_ft_amount: float = 1.0,
        intra_node_test: bool = False,
        intra_node_rbt_rounds: int = 3,
        intra_node_rbt_amount: float = 1.0,
        intra_node_rbt_fund: float = 5.0,
        intra_node_ft_rounds: int = 2,
        intra_node_ft_amount: int = 1,
        intra_node_ft_fund: int = 2,
        run_all_tests: bool = False,
        negative_tests: bool = False,
        fullnode_test: bool = False,
        fullnode_restart_test: bool = True,
    ) -> None:
        if skip_setup:
            self._load_state()
        else:
            self.setup_nodes()

        # Register the fullnode as a peer BEFORE any transaction is published.
        # It must already be meshed when the first rubix_txn event goes out —
        # gossipsub has no replay, so anything published before it joins the
        # topic is simply never delivered to it.
        if fullnode_test:
            self.setup_fullnode()

        if skip_mint:
            log.info("=== MINTING SKIPPED (--skip-mint) ===")
        else:
            self.mint_tokens()
            if run_all_tests:
                self._settle_between_phases("MINTING", "SHUTTLE")

        nft_verification = []
        sc_verification = []
        ft_verification: List[Dict[str, str]] = []

        # Under --run-all-tests we want every subsystem to run strictly
        # sequentially end-to-end. Collapse the shuttle phase list down to
        # the single sequential warmup so the concurrency=5/10/20 bursts
        # don't race the NFT/SC/FT/bundled/all-in-one steps that follow.
        if run_all_tests and self.cfg.phases:
            seq_only = [p for p in self.cfg.phases if p.concurrency <= 1]
            if seq_only:
                dropped = [p.name for p in self.cfg.phases if p.concurrency > 1]
                self.cfg.phases = seq_only
                log.info(
                    "=== SHUTTLE: run_all_tests active — restricted to sequential phases "
                    "(dropped parallel phases: %s) ===",
                    ", ".join(dropped) or "<none>",
                )

        # Skip RBT shuttle if any *-only mode is active
        shuttle_verification: List[Dict[str, str]] = []
        if not nft_only and not sc_only and not ft_only:
            shuttle_verification = self.run_shuttle(decimal_transfers=decimal_transfers)
            if run_all_tests:
                self._settle_between_phases("SHUTTLE", "NFT")
        else:
            if nft_only:
                log.info("=== SHUTTLE SKIPPED (--nft-only mode) ===")
            if sc_only:
                log.info("=== SHUTTLE SKIPPED (--sc-only mode) ===")
            if ft_only:
                log.info("=== SHUTTLE SKIPPED (--ft-only mode) ===")

        if nft_phases:
            self.run_nft(nft_phases)

            # Mint child NFTs under a deployed parent (exercises the
            # parentNFTId + numberOfChildren tx path and the children/parent
            # query endpoints). Run right after deploy so the parent is fresh.
            log.info("=== NFT CHILD MINT START ===")
            self.nft_engine.run_nft_mint_children(number_of_children=2)
            log.info("=== NFT CHILD MINT COMPLETE ===")

            # NFT Execute phases (run after deployment)
            if nft_self_execute:
                log.info("=== NFT SELF-EXECUTE START ===")
                self.nft_engine.run_nft_self_execute()
                log.info("=== NFT SELF-EXECUTE COMPLETE ===")

            if nft_transfer:
                log.info("=== NFT OWNERSHIP TRANSFER START ===")
                self.nft_engine.run_nft_transfer_ownership()
                log.info("=== NFT OWNERSHIP TRANSFER COMPLETE ===")

            if nft_cross_execute:
                log.info("=== NFT CROSS-NODE EXECUTION START ===")
                self.nft_engine.run_nft_cross_execute()
                log.info("=== NFT CROSS-NODE EXECUTION COMPLETE ===")

            # Repeated NFT execution rounds (mixed self + cross-node)
            if exec_rounds > 0:
                log.info("=== NFT REPEATED EXECUTION START (%d rounds) ===", exec_rounds)
                nft_exec_stats = self.nft_engine.run_repeated_executions(exec_rounds)
                log.info("=== NFT REPEATED EXECUTION COMPLETE ===")

            # NFT API verification (exercises list, chain, fetch, balance, tx APIs)
            log.info("=== NFT API VERIFICATION START ===")
            nft_verification = self.nft_engine.run_verification()
            log.info("=== NFT API VERIFICATION COMPLETE ===")

            if run_all_tests:
                self._settle_between_phases("NFT", "SMART_CONTRACT")

        # Smart contract deployment and execution
        if sc_count > 0:
            self.run_smart_contract(sc_count, sc_execute)

            # Repeated SC execution rounds (mixed self + cross-node)
            if exec_rounds > 0 and sc_execute:
                log.info("=== SC REPEATED EXECUTION START (%d rounds) ===", exec_rounds)
                sc_exec_stats = self.sc_engine.run_repeated_executions(exec_rounds)
                log.info("=== SC REPEATED EXECUTION COMPLETE ===")

            # SC API verification (exercises list, chain, callback, tx APIs)
            # Calculate expected minimum chain length:
            #   1 (deploy) + 1 (cross-exec if sc_execute) + exec_rounds
            #   Note: self-execute only runs on the FIRST SC, so use the lower bound
            #   (deploy + cross-exec + rounds) as the minimum for all SCs.
            sc_expected_min = 1 + (1 if sc_execute else 0) + exec_rounds
            log.info("=== SMART CONTRACT API VERIFICATION START ===")
            sc_verification = self.sc_engine.run_verification(
                cross_executed=sc_execute,
                expected_min_chain=sc_expected_min if exec_rounds > 0 else 0,
            )
            log.info("=== SMART CONTRACT API VERIFICATION COMPLETE ===")

            # End-to-end callback-delivery check: verifies that nodeB actually
            # POSTs to the URL registered in call_back_urls when it receives a
            # new SC event. Requires nodeB to have already subscribed (happens
            # during cross-node execution).
            if sc_execute:
                log.info("=== SMART CONTRACT CALLBACK DELIVERY CHECK START ===")
                callback_delivery_results = self.sc_engine.run_callback_delivery_check()
                sc_verification.extend(callback_delivery_results)
                log.info("=== SMART CONTRACT CALLBACK DELIVERY CHECK COMPLETE ===")

            if run_all_tests:
                self._settle_between_phases("SMART_CONTRACT", "BUNDLED_TX")

        # Bundled (combined) transaction test
        bundled_verification = []
        if bundled_test:
            from test.integration.engines.bundled_engine import BundledEngine

            # Pick the second NFT and second SC for bundled tests
            # (first NFT may have ownership-transferred issues; second is clean)
            bundled_nft_id = None
            bundled_sc_id = None

            if hasattr(self, "nft_engine") and self.nft_engine._deployed_nfts:
                # Use the last deployed NFT (least likely to have ownership transfer issues)
                bundled_nft_id = self.nft_engine._deployed_nfts[-1]["nft_id"]
            if hasattr(self, "sc_engine") and self.sc_engine._deployed_contracts:
                bundled_sc_id = self.sc_engine._deployed_contracts[-1]["sc_id"]

            if not bundled_nft_id or not bundled_sc_id:
                log.error(
                    "Bundled test requires deployed NFT and SC. "
                    "Got nft_id=%s, sc_id=%s. "
                    "Ensure --nft-count and --sc-count are set.",
                    bundled_nft_id, bundled_sc_id,
                )
            else:
                log.info("=== BUNDLED TRANSACTION TEST START ===")
                log.info(
                    "  Using NFT=%s  SC=%s  RBT=%.4f  rounds=%d",
                    bundled_nft_id[:12] + "...",
                    bundled_sc_id[:12] + "...",
                    bundled_rbt,
                    bundled_rounds,
                )

                bundled_engine = BundledEngine(
                    node_a=self.node_a,
                    node_b=self.node_b,
                    did_a=self.did_a,
                    did_b=self.did_b,
                    config=self.cfg,
                    reporter=self.reporter,
                )

                bundled_stats = bundled_engine.run_bundled_test(
                    nft_id=bundled_nft_id,
                    sc_id=bundled_sc_id,
                    rbt_amount=bundled_rbt,
                    rounds=bundled_rounds,
                )

                log.info(
                    "=== BUNDLED TRANSACTION TEST COMPLETE: %d success, %d fail ===",
                    bundled_stats["success"], bundled_stats["fail"],
                )

                # Verification
                log.info("=== BUNDLED TX VERIFICATION START ===")
                bundled_verification = bundled_engine.run_verification(
                    nft_id=bundled_nft_id,
                    sc_id=bundled_sc_id,
                )
                log.info("=== BUNDLED TX VERIFICATION COMPLETE ===")

            if run_all_tests:
                self._settle_between_phases("BUNDLED_TX", "FT")

        # FT mint / transfer / verification
        if ft_phases:
            ft_verification = self.run_ft(
                ft_phases=ft_phases,
                tokens_per_batch=ft_tokens_per_batch,
                rbt_per_batch=ft_rbt_per_batch,
                do_transfer=ft_transfer,
                transfer_rounds=ft_transfer_rounds,
            )

            if run_all_tests:
                self._settle_between_phases("FT", "ALL_IN_ONE")

        # All-in-one transaction test (RBT + FT[] + NFT[] + SC[] atomically)
        all_in_one_verification: List[Dict[str, str]] = []
        if all_in_one_test:
            from test.integration.engines.bundled_engine import BundledEngine

            # Collect every deployed NFT id
            aio_nft_ids: List[str] = []
            if hasattr(self, "nft_engine") and self.nft_engine._deployed_nfts:
                aio_nft_ids = [
                    n["nft_id"] for n in self.nft_engine._deployed_nfts
                    if n.get("nft_id")
                ]

            # Collect every deployed SC id
            aio_sc_ids: List[str] = []
            if hasattr(self, "sc_engine") and self.sc_engine._deployed_contracts:
                aio_sc_ids = [
                    c["sc_id"] for c in self.sc_engine._deployed_contracts
                    if c.get("sc_id")
                ]

            # Snapshot the minted FT batches — the engine mutates per-batch
            # ``ft_count`` / ``owner_label`` during the run, so copy.
            aio_ft_batches: List[Dict[str, Any]] = []
            if hasattr(self, "ft_engine") and self.ft_engine._minted_fts:
                for b in self.ft_engine._minted_fts:
                    aio_ft_batches.append({
                        "ft_name": b.get("ft_name"),
                        "creator_did": b.get("creator_did"),
                        "owner_did": b.get("owner_did"),
                        "owner_label": b.get("owner_label"),
                        "ft_count": b.get("ft_count", 0),
                    })

            log.info(
                "=== ALL-IN-ONE TX TEST: nfts=%d  scs=%d  ft_batches=%d  rounds=%d  rbt=%.4f ===",
                len(aio_nft_ids), len(aio_sc_ids), len(aio_ft_batches),
                all_in_one_rounds, all_in_one_rbt,
            )

            if not aio_nft_ids and not aio_sc_ids and not aio_ft_batches:
                log.warning(
                    "All-in-one test skipped: no NFTs, SCs, or FTs available. "
                    "Set --nft-count / --sc-count / --ft-count."
                )
            else:
                aio_engine = BundledEngine(
                    node_a=self.node_a,
                    node_b=self.node_b,
                    did_a=self.did_a,
                    did_b=self.did_b,
                    config=self.cfg,
                    reporter=self.reporter,
                )
                aio_stats = aio_engine.run_all_in_one_test(
                    nft_ids=aio_nft_ids,
                    sc_ids=aio_sc_ids,
                    ft_batches=aio_ft_batches,
                    rbt_amount=all_in_one_rbt,
                    rounds=all_in_one_rounds,
                    ft_amount_per_batch=all_in_one_ft_amount,
                )
                log.info(
                    "=== ALL-IN-ONE TX TEST COMPLETE: %d success, %d fail ===",
                    aio_stats["success"], aio_stats["fail"],
                )

                all_in_one_verification = aio_engine.run_all_in_one_verification(
                    nft_ids=aio_nft_ids,
                    sc_ids=aio_sc_ids,
                    ft_names=[b["ft_name"] for b in aio_ft_batches],
                )

            if run_all_tests:
                self._settle_between_phases("ALL_IN_ONE", "INTRA_NODE")

        # Intra-node two-DID test (second DID on nodeA exercising RBT / FT /
        # NFT / SC with did_a as counterparty — everything stays on nodeA).
        intra_node_verification: List[Dict[str, str]] = []
        if intra_node_test:
            intra_node_verification = self.run_intra_node(
                rbt_rounds=intra_node_rbt_rounds,
                rbt_amount=intra_node_rbt_amount,
                rbt_fund=intra_node_rbt_fund,
                ft_rounds=intra_node_ft_rounds,
                ft_amount=intra_node_ft_amount,
                ft_fund=intra_node_ft_fund,
            )

            if run_all_tests:
                self._settle_between_phases("INTRA_NODE", "FINALISE")

        # Negative / failure-path tests. Run LAST: they only attempt INVALID
        # operations (each asserted to be rejected + leave state unchanged), so
        # they cannot corrupt the happy-path state they run after.
        negative_verification: List[Dict[str, str]] = []
        if negative_tests:
            negative_verification = self.run_negative()

        # Deferred intra-node FT balance check — run at the very end so the
        # (slow) intra-node FT settlement to did_a2 has maximum elapsed time.
        deferred_verification: List[Dict[str, str]] = []
        if self._intra_node_engine is not None:
            deferred_verification = self._intra_node_engine.verify_ft_balance_deferred(db_a=self.db_a)

        # Exercise read-only query endpoints not covered by the flows above:
        # tx-history-by-DID and NFT children/parent.
        extra_api_verification = self._verify_extra_apis()

        # Fullnode suite. Runs after every other subsystem so the fullnode has
        # observed a full, varied stream of published transactions (RBT, FT,
        # NFT, SC, bundled) before its chain-integrity and duplicate checks are
        # evaluated — those assert over EVERYTHING it stored, not just its own
        # tracked transfer. Its own tracked transfer is driven inside the
        # engine, so the assertion never depends on another phase's timing.
        fullnode_verification: List[Dict[str, str]] = []
        if fullnode_test:
            fullnode_verification = self.run_fullnode(
                include_restart=fullnode_restart_test
            )

        # Final assurance — every recorded-SUCCESS transaction must be durably
        # persisted on each participating node. Runs LAST so all subsystem and
        # deferred writes have committed across nodes.
        tx_persist_verification = self._verify_transactions_persisted()

        self.finalise()
        self.run_db_snapshot()

        # Write verification summary
        all_verifications = (
            shuttle_verification
            + nft_verification
            + sc_verification
            + bundled_verification
            + ft_verification
            + all_in_one_verification
            + intra_node_verification
            + negative_verification
            + deferred_verification
            + extra_api_verification
            + fullnode_verification
            + tx_persist_verification
        )
        if all_verifications:
            self._write_verification_summary(all_verifications)

