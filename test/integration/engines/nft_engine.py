"""
nft_engine.py — NFT creation engine for stress testing.

Workflow:
  - Alternates NFT creation between nodeA and nodeB
  - Supports sequential and parallel creation phases
  - Uses random file selection to avoid collisions
  - Records all NFT creation operations in the reporter

Pattern matches shuttle.py for consistency.
"""

from __future__ import annotations

import logging
import threading
import time
from datetime import datetime, timezone
from typing import TYPE_CHECKING, Optional

if TYPE_CHECKING:
    from test.integration.clients.api_client import NodeClient
    from test.integration.config import StressConfig, PhaseConfig
    from test.integration.engines.reporter import StressReporter

try:
    from test.integration.engines.file_selector import select_nft_files
except ImportError:
    import sys
    import os
    sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))
    from test.integration.engines.file_selector import select_nft_files

log = logging.getLogger(__name__)


class NFTEngine:
    """Runs NFT creation phases, alternating between nodeA and nodeB.

    Args:
        node_a, node_b: NodeClient instances.
        did_a, did_b:   DIDs for nodeA and nodeB respectively.
        config:         StressConfig.
        reporter:       StressReporter for recording NFT operations.
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

        self._nft_counter = 0
        self._counter_lock = threading.Lock()

        # Track deployed NFTs for execute phase
        self._deployed_nfts = []  # List of {"nft_id": str, "owner_did": str, "owner_node": NodeClient, "label": str}
        self._deployed_lock = threading.Lock()

        # nft_id -> executor NodeClient, for NFTs executed cross-node (used to
        # verify the chain synced onto the executor node). Mirrors the SC engine's
        # cross-node chain-sync verification.
        self._cross_executed = {}  # type: Dict[str, "NodeClient"]

        # Child NFTs minted under a parent: {"parent": str, "children": [child_id, ...],
        # "owner_node": NodeClient, "owner_did": str}. Populated by run_nft_mint_children.
        self._minted_children = []  # type: list
        # Outcome of the child-mint phase, surfaced as a verification check.
        self._child_mint_outcome = None  # type: Optional[dict]

    # ------------------------------------------------------------------
    # Entry point
    # ------------------------------------------------------------------

    def run_all_phases(self, phases: list["PhaseConfig"]) -> None:
        """Execute all NFT creation phases in order.

        Args:
            phases: List of PhaseConfig objects defining NFT creation phases
        """
        for phase in phases:
            log.info(
                "=== NFT Phase: %s  concurrency=%d  nft_count=%d ===",
                phase.name,
                phase.concurrency,
                phase.tx_count,  # Reusing tx_count field for nft_count
            )
            t0 = time.time()
            if phase.concurrency == 1:
                self._run_sequential(phase.tx_count)
            else:
                self._run_parallel(phase.tx_count, phase.concurrency)
            elapsed = time.time() - t0
            log.info(
                "NFT Phase '%s' done in %.1fs.  Total NFTs created: %d",
                phase.name,
                elapsed,
                self._nft_counter,
            )

    # ------------------------------------------------------------------
    # Sequential phase
    # ------------------------------------------------------------------

    def _run_sequential(self, nft_count: int) -> None:
        """Alternate A / B, one at a time."""
        completed = 0
        use_node_a = True  # Start with nodeA

        while completed < nft_count:
            if use_node_a:
                node, did, label = self.node_a, self.did_a, "A"
            else:
                node, did, label = self.node_b, self.did_b, "B"

            self._fire_nft_creation(node, did, label)
            completed += 1
            use_node_a = not use_node_a

    # ------------------------------------------------------------------
    # Parallel phase
    # ------------------------------------------------------------------

    def _run_parallel(self, nft_count: int, concurrency: int) -> None:
        """Fire batches of *concurrency* NFT creations; alternate node per batch."""
        completed = 0
        use_node_a = True

        while completed < nft_count:
            batch_size = min(concurrency, nft_count - completed)

            if use_node_a:
                node, did, label = self.node_a, self.did_a, "A"
            else:
                node, did, label = self.node_b, self.did_b, "B"

            log.info("[%s] Launching NFT batch of %d", label, batch_size)
            threads = [
                threading.Thread(
                    target=self._fire_nft_creation,
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
    # Core NFT creation
    # ------------------------------------------------------------------

    def _fire_nft_creation(
        self,
        node: "NodeClient",
        did: str,
        label: str,
    ) -> None:
        """Execute NFT creation and deploy to blockchain."""
        with self._counter_lock:
            self._nft_counter += 1
            nft_counter = self._nft_counter
            nft_seq_id = f"NFT-{nft_counter:05d}"

        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        error: Optional[str] = None
        metadata_file: Optional[str] = None
        artifact_file: Optional[str] = None
        nft_token_id: Optional[str] = None
        deploy_req_id: Optional[str] = None
        deploy_txn_id: Optional[str] = None

        try:
            # Step 1: Select random files for NFT
            metadata_path, artifact_path = select_nft_files()
            metadata_file = metadata_path
            artifact_file = artifact_path

            # Step 2: Create NFT
            result = node.create_nft(did, metadata_path, artifact_path)
            req_id = result.get("req_id")
            nft_token_id = result.get("nft_id")

            # Step 3: Deploy to blockchain (always)
            if nft_token_id:
                log.info(
                    "[%s] Deploying NFT %s to blockchain...",
                    nft_seq_id,
                    nft_token_id[:12] + "...",
                )
                deploy_result = node.transfer_nft(
                    sender_did=did,
                    receiver_did=did,  # Self-transfer for deployment
                    nft_id=nft_token_id,
                    data=f"NFT deployment #{nft_counter}",
                )
                deploy_req_id = deploy_result.get("req_id")
                deploy_txn_id = node.extract_txn_id(deploy_result)
                log.info(
                    "[%s] NFT deployed to blockchain, deploy_req_id=%s",
                    nft_seq_id,
                    deploy_req_id,
                )
            else:
                log.warning("[%s] No NFT ID returned, skipping deployment", nft_seq_id)

        except Exception as exc:
            status = "FAIL"
            error = str(exc)

        duration_ms = int((time.time() - t0) * 1000)

        self.reporter.record_transaction(
            {
                "id": nft_seq_id,
                "type": "NFT_DEPLOY",
                "node": label,
                "did": did[:20] + "...",  # Truncate for readability
                "status": status,
                "req_id": req_id,
                "transaction_id": deploy_txn_id,
                "nft_id": nft_token_id,
                "deploy_req_id": deploy_req_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
                "metadata": metadata_file,
                "artifact": artifact_file,
            }
        )

        if status == "SUCCESS":
            # Save deployed NFT info for execute phase
            if nft_token_id:
                with self._deployed_lock:
                    self._deployed_nfts.append({
                        "nft_id": nft_token_id,
                        "owner_did": did,
                        "owner_node": node,
                        "label": label,
                    })

            log.info(
                "[%s] Node-%s  nft=%s  create_req=%s  deploy_req=%s  %dms",
                nft_seq_id,
                label,
                nft_token_id[:12] + "..." if nft_token_id else "N/A",
                req_id,
                deploy_req_id,
                duration_ms,
            )
        else:
            log.warning(
                "[%s] Node-%s  FAIL  %dms  error=%s",
                nft_seq_id,
                label,
                duration_ms,
                error,
            )

    @property
    def total_nfts(self) -> int:
        """Total number of NFTs created so far."""
        with self._counter_lock:
            return self._nft_counter

    # ------------------------------------------------------------------
    # NFT Execute Phase
    # ------------------------------------------------------------------

    def run_nft_self_execute(self) -> None:
        """Execute NFT without transferring ownership (self-execution).

        Uses the first deployed NFT and executes it with the same owner.
        """
        with self._deployed_lock:
            if not self._deployed_nfts:
                log.warning("No deployed NFTs available for self-execution")
                return

            nft_info = self._deployed_nfts[0]

        nft_id = nft_info["nft_id"]
        owner_did = nft_info["owner_did"]
        owner_node = nft_info["owner_node"]
        label = nft_info["label"]

        log.info(
            "=== NFT Self-Execute: nft=%s  owner=%s (Node-%s) ===",
            nft_id[:12] + "...",
            owner_did[:20] + "...",
            label,
        )

        # Wait for NFT deployment to settle on blockchain
        log.info("Waiting 5 seconds for NFT deployment to settle...")
        time.sleep(5)

        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            result = owner_node.execute_nft(
                executor_did=owner_did,
                nft_id=nft_id,
                data="NFT self-execution test",
            )
            req_id = result.get("req_id")
            txn_id = owner_node.extract_txn_id(result)
            log.info(
                "NFT self-execution completed: req_id=%s",
                req_id,
            )
        except Exception as exc:
            status = "FAIL"
            error = str(exc)
            log.error("NFT self-execution failed: %s", exc)

        duration_ms = int((time.time() - t0) * 1000)

        self.reporter.record_transaction(
            {
                "id": "EXEC-SELF-001",
                "type": "NFT_EXECUTE_SELF",
                "node": label,
                "did": owner_did[:20] + "...",
                "nft_id": nft_id,
                "status": status,
                "req_id": req_id,
                "transaction_id": txn_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
            }
        )

    def run_nft_transfer_ownership(self) -> None:
        """Execute NFT and transfer ownership to opposite node."""
        with self._deployed_lock:
            if not self._deployed_nfts:
                log.warning("No deployed NFTs available for ownership transfer")
                return

            nft_info = self._deployed_nfts[0]

        nft_id = nft_info["nft_id"]
        sender_did = nft_info["owner_did"]
        sender_node = nft_info["owner_node"]
        sender_label = nft_info["label"]

        # Transfer to opposite node
        if sender_label == "A":
            receiver_did = self.did_b
            receiver_node = self.node_b
            receiver_label = "B"
        else:
            receiver_did = self.did_a
            receiver_node = self.node_a
            receiver_label = "A"

        log.info(
            "=== NFT Transfer Ownership: nft=%s  %s->%s (%s->%s) ===",
            nft_id[:12] + "...",
            sender_label,
            receiver_label,
            sender_did[:20] + "...",
            receiver_did[:20] + "...",
        )

        # Wait for NFT deployment to settle on blockchain
        log.info("Waiting 5 seconds for NFT deployment to settle...")
        time.sleep(5)

        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            result = sender_node.transfer_nft_ownership(
                sender_did=sender_did,
                receiver_did=receiver_did,
                nft_id=nft_id,
                data="NFT ownership transfer test",
            )
            req_id = result.get("req_id")
            txn_id = sender_node.extract_txn_id(result)
            log.info(
                "NFT ownership transfer completed: req_id=%s",
                req_id,
            )

            # Update ownership tracking
            with self._deployed_lock:
                for nft in self._deployed_nfts:
                    if nft["nft_id"] == nft_id:
                        nft["owner_did"] = receiver_did
                        nft["owner_node"] = receiver_node
                        nft["label"] = receiver_label
                        break

        except Exception as exc:
            status = "FAIL"
            error = str(exc)
            log.error("NFT ownership transfer failed: %s", exc)

        duration_ms = int((time.time() - t0) * 1000)

        self.reporter.record_transaction(
            {
                "id": "EXEC-TRANSFER-001",
                "type": "NFT_EXECUTE_TRANSFER",
                "node": sender_label,
                "sender_did": sender_did[:20] + "...",
                "receiver_did": receiver_did[:20] + "...",
                "nft_id": nft_id,
                "status": status,
                "transaction_id": txn_id,
                "req_id": req_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
            }
        )

    # ------------------------------------------------------------------
    # NFT Child-Minting Phase
    # ------------------------------------------------------------------

    def run_nft_mint_children(self, number_of_children: int = 2) -> None:
        """Mint child NFTs under the first deployed NFT (owned by nodeA).

        Exercises POST /rubix/v1/tx with one NFT entry per child carrying
        parentNFTId (the server derives each child id via IPFS). Records the
        transaction (with its on-chain id, so TX_PERSISTED covers it) and stores
        the parent→children relationship for run_verification to assert via the
        nfts/{id}/children and nfts/{id}/parent endpoints. The mint outcome is
        also surfaced as a verification check (NFT_MINT_CHILDREN) so a failure
        cannot pass silently.
        """
        with self._deployed_lock:
            if not self._deployed_nfts:
                log.warning("No deployed NFTs available for child minting")
                return
            nft_info = self._deployed_nfts[0]

        parent_id = nft_info["nft_id"]
        owner_node = nft_info["owner_node"]
        owner_did = nft_info["owner_did"]

        log.info("=== NFT CHILD MINT: %d children under %s ===",
                 number_of_children, parent_id[:12] + "...")

        seq_id = f"NFT-CHILD-{parent_id[:8]}"
        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        txn_id: Optional[str] = None
        children: list = []
        error: Optional[str] = None

        try:
            result = owner_node.mint_nft_children(
                initiator_did=owner_did,
                parent_nft_id=parent_id,
                number_of_children=number_of_children,
            )
            req_id = result.get("req_id")
            txn_id = owner_node.extract_txn_id(result)
            minted = owner_node.extract_minted_children(result)
            children = [m.get("childNFTId") for m in minted if m.get("childNFTId")]
            if children:
                with self._deployed_lock:
                    self._minted_children.append({
                        "parent": parent_id,
                        "children": children,
                        "owner_node": owner_node,
                        "owner_did": owner_did,
                    })
            log.info("[%s] Minted %d child NFT(s): %s",
                     seq_id, len(children), [c[:12] + "..." for c in children])
        except Exception as exc:  # noqa: BLE001
            status = "FAIL"
            error = str(exc)
            log.error("[%s] child-NFT mint FAILED: %s", seq_id, exc)

        self.reporter.record_transaction({
            "id": seq_id,
            "type": "NFT_MINT_CHILDREN",
            "node": nft_info["label"],
            "parent_nft_id": parent_id,
            "children_count": len(children),
            "status": status,
            "req_id": req_id,
            "transaction_id": txn_id,
            "duration_ms": int((time.time() - t0) * 1000),
            "timestamp": ts,
            "error": error,
        })

        # Record the mint outcome so run_verification surfaces a failure as a
        # FAIL check (not a silent skip when no children were minted).
        self._child_mint_outcome = {
            "attempted": number_of_children,
            "minted": len(children),
            "status": status,
            "error": error,
        }

        # Settle so children are queryable on the owner node.
        time.sleep(5)
        log.info("=== NFT CHILD MINT COMPLETE ===")

    # ------------------------------------------------------------------
    # NFT Cross-Node Execute Phase
    # ------------------------------------------------------------------

    def run_nft_cross_execute(self) -> None:
        """Execute NFT from the opposite node (cross-node execution).

        For each deployed NFT owned by nodeA:
        1. NodeB subscribes to the NFT
        2. NodeB executes the NFT

        Mirrors the SmartContractEngine.run_cross_node_execution() pattern.
        """
        with self._deployed_lock:
            if not self._deployed_nfts:
                log.warning("No deployed NFTs available for cross-node execution")
                return

            nfts_to_execute = list(self._deployed_nfts)

        log.info("=== NFT CROSS-NODE EXECUTION START ===")
        log.info("Executing %d NFTs from opposite node", len(nfts_to_execute))

        # Wait for NFT deployments to settle
        log.info("Waiting 5 seconds for NFT deployments to settle...")
        time.sleep(5)

        for nft_info in nfts_to_execute:
            self._execute_nft_from_opposite_node(nft_info)

        # Wait for any pending P2P sync operations to complete
        log.info("Waiting 5 seconds for cross-node sync operations to finalize...")
        time.sleep(5)

        log.info("=== NFT CROSS-NODE EXECUTION COMPLETE ===")

    # ------------------------------------------------------------------
    # NFT Verification Phase
    # ------------------------------------------------------------------

    def run_verification(self) -> list:
        """Run verification checks using all NFT query APIs.

        Exercises:
          - GET /rubix/v1/nfts  (list all NFTs on both nodes)
          - GET /rubix/v1/nfts/{id}/chain  (chain per NFT on deployer)
          - GET /rubix/v1/dids/{did}/balances/nft  (NFT balance per DID)
          - GET /rubix/v1/tx  (list all transactions on both nodes)

        Returns:
            List of verification result dicts: {"check": str, "status": "PASS"|"FAIL", "detail": str}
        """
        log.info("=== NFT VERIFICATION START ===")
        results = []

        # --- 1. List NFTs on both nodes ---
        try:
            nfts_a = self.node_a.list_nfts()
            results.append({
                "check": "NFT_LIST_NODE_A",
                "status": "PASS",
                "detail": f"nodeA has {len(nfts_a)} NFTs",
            })
        except Exception as exc:
            results.append({
                "check": "NFT_LIST_NODE_A",
                "status": "FAIL",
                "detail": str(exc),
            })
            nfts_a = []

        try:
            nfts_b = self.node_b.list_nfts()
            results.append({
                "check": "NFT_LIST_NODE_B",
                "status": "PASS",
                "detail": f"nodeB has {len(nfts_b)} NFTs",
            })
        except Exception as exc:
            results.append({
                "check": "NFT_LIST_NODE_B",
                "status": "FAIL",
                "detail": str(exc),
            })
            nfts_b = []

        # --- 2. Get chain for each deployed NFT ---
        with self._deployed_lock:
            deployed_snapshot = list(self._deployed_nfts)

        for nft_info in deployed_snapshot:
            nft_id = nft_info["nft_id"]
            owner_node = nft_info["owner_node"]
            label = nft_info["label"]
            short_id = nft_id[:12] + "..."

            chain = None
            try:
                chain = owner_node.get_nft_chain(nft_id)
                chain_len = len(chain) if chain else 0
                passed = chain_len >= 1
                results.append({
                    "check": f"NFT_CHAIN_{short_id}_NODE_{label}",
                    "status": "PASS" if passed else "FAIL",
                    "detail": f"chain has {chain_len} entries (expected >= 1)",
                })
            except Exception as exc:
                results.append({
                    "check": f"NFT_CHAIN_{short_id}_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

            # Cross-node chain-sync verification: if this NFT was executed from
            # the opposite node, that node subscribed and should have synced the
            # FULL chain. Assert the executor node's chain matches the owner's —
            # proving subscribe→sync delivered the chain, not just that the
            # execute API returned success. Mirrors SC_CHAIN_SYNC.
            executor_node = self._cross_executed.get(nft_id)
            if executor_node is not None:
                exec_label = "B" if label == "A" else "A"
                try:
                    chain_x = executor_node.get_nft_chain(nft_id)
                    len_x = len(chain_x) if chain_x else 0
                    results.append({
                        "check": f"NFT_CHAIN_{short_id}_NODE_{exec_label}",
                        "status": "PASS" if len_x >= 1 else "FAIL",
                        "detail": f"executor-node chain has {len_x} entries (expected >= 1, sync verification)",
                    })
                    if chain is not None and chain_x is not None:
                        synced = len(chain) == len(chain_x)
                        results.append({
                            "check": f"NFT_CHAIN_SYNC_{short_id}",
                            "status": "PASS" if synced else "FAIL",
                            "detail": f"owner(node{label})={len(chain)} vs executor(node{exec_label})={len_x} entries",
                        })
                except Exception as exc:
                    results.append({
                        "check": f"NFT_CHAIN_SYNC_{short_id}",
                        "status": "FAIL",
                        "detail": str(exc),
                    })

        # --- 2b. Child-NFT relationships (parent/children endpoints) ---
        # Surface the mint outcome itself so a failed mint is a FAIL, not silent.
        if self._child_mint_outcome is not None:
            o = self._child_mint_outcome
            ok = o["status"] == "SUCCESS" and o["minted"] >= o["attempted"] and o["attempted"] > 0
            results.append({
                "check": "NFT_MINT_CHILDREN",
                "status": "PASS" if ok else "FAIL",
                "detail": (
                    f"minted {o['minted']}/{o['attempted']} child NFT(s)"
                    + (f" — {o['error']}" if o.get("error") else "")
                ),
            })

        with self._deployed_lock:
            minted_snapshot = list(self._minted_children)
        for mc in minted_snapshot:
            parent = mc["parent"]
            expected_children = mc["children"]
            node = mc["owner_node"]
            pshort = parent[:12] + "..."

            # children endpoint: parent must report >= the minted children
            try:
                children = node.get_nft_children(parent)
                child_ids = {
                    (c.get("nft_id") or c.get("nftId") or c.get("NFTId"))
                    for c in children
                } if children else set()
                found = sum(1 for c in expected_children if c in child_ids)
                ok = found >= len(expected_children) and len(expected_children) > 0
                results.append({
                    "check": f"NFT_CHILDREN_MINTED_{pshort}",
                    "status": "PASS" if ok else "FAIL",
                    "detail": (
                        f"parent reports {len(child_ids)} child(ren); "
                        f"{found}/{len(expected_children)} minted children present"
                    ),
                })
            except Exception as exc:  # noqa: BLE001
                results.append({
                    "check": f"NFT_CHILDREN_MINTED_{pshort}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

            # parent endpoint: each child must point back to the parent
            for child in expected_children:
                cshort = child[:12] + "..."
                try:
                    presp = node.get_nft_parent(child)
                    presult = presp.get("result") or {}
                    parent_back = (
                        presult.get("nft_id") or presult.get("nftId")
                        or presult.get("NFTId")
                    )
                    ok = bool(presp.get("status")) and parent_back == parent
                    results.append({
                        "check": f"NFT_PARENT_OF_{cshort}",
                        "status": "PASS" if ok else "FAIL",
                        "detail": f"child parent={str(parent_back)[:16]}... (expected {parent[:16]}...)",
                    })
                except Exception as exc:  # noqa: BLE001
                    results.append({
                        "check": f"NFT_PARENT_OF_{cshort}",
                        "status": "FAIL",
                        "detail": str(exc),
                    })

        # --- 3. Get NFT balance per DID ---
        for did, node, label in [
            (self.did_a, self.node_a, "A"),
            (self.did_b, self.node_b, "B"),
        ]:
            try:
                nft_balance = node.get_nft_balance(did)
                results.append({
                    "check": f"NFT_BALANCE_NODE_{label}",
                    "status": "PASS",
                    "detail": f"node{label} DID owns {len(nft_balance)} free NFTs",
                })
            except Exception as exc:
                results.append({
                    "check": f"NFT_BALANCE_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- 4. List all transactions on both nodes ---
        for node, label in [(self.node_a, "A"), (self.node_b, "B")]:
            try:
                txns = node.list_transactions()
                results.append({
                    "check": f"TX_LIST_NODE_{label}",
                    "status": "PASS",
                    "detail": f"node{label} has {len(txns)} transactions",
                })
            except Exception as exc:
                results.append({
                    "check": f"TX_LIST_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- Log summary ---
        passed = sum(1 for r in results if r["status"] == "PASS")
        failed = sum(1 for r in results if r["status"] == "FAIL")
        log.info(
            "=== NFT VERIFICATION COMPLETE: %d passed, %d failed ===",
            passed, failed,
        )
        for r in results:
            level = log.info if r["status"] == "PASS" else log.warning
            level("  [%s] %s: %s", r["status"], r["check"], r["detail"])

        return results

    # ------------------------------------------------------------------
    # Repeated Execution Phase (mixed self + cross-node)
    # ------------------------------------------------------------------

    def run_repeated_executions(self, rounds: int) -> dict:
        """Execute each NFT multiple rounds, alternating self-execute and cross-node execute.

        For round i (0-based):
          - even rounds → self-execute on the owner node
          - odd rounds  → cross-node execute from the opposite node

        Skips NFTs whose cross-node execution is known to fail (e.g. after ownership transfer
        where the token status is not yet 'Deployed/Executed' on the opposite node).

        Args:
            rounds: Number of execution rounds per NFT.

        Returns:
            Dict mapping nft_id → {"self": int, "cross": int, "fail": int} with actual success counts.
        """
        with self._deployed_lock:
            nfts = list(self._deployed_nfts)

        if not nfts:
            log.warning("No deployed NFTs available for repeated execution")
            return {}

        log.info("=== NFT REPEATED EXECUTION START: %d rounds × %d NFTs ===", rounds, len(nfts))

        exec_counter = 0
        stats: dict = {}  # nft_id → {"self": 0, "cross": 0, "fail": 0}

        for nft_info in nfts:
            nft_id = nft_info["nft_id"]
            owner_label = nft_info["label"]
            owner_did = nft_info["owner_did"]
            owner_node = nft_info["owner_node"]

            # Determine opposite node
            if owner_label == "A":
                opp_node, opp_did, opp_label = self.node_b, self.did_b, "B"
            else:
                opp_node, opp_did, opp_label = self.node_a, self.did_a, "A"

            stats[nft_id] = {"self": 0, "cross": 0, "fail": 0}

            log.info(
                "--- NFT %s (owner=%s): starting %d rounds ---",
                nft_id[:12] + "...", owner_label, rounds,
            )

            for r in range(rounds):
                exec_counter += 1
                is_self = (r % 2 == 0)
                round_type = "SELF" if is_self else "CROSS"
                seq_id = f"NFT-REXEC-{exec_counter:04d}"

                ts = datetime.now(tz=timezone.utc).isoformat()
                t0 = time.time()
                status = "SUCCESS"
                req_id: Optional[str] = None
                txn_id: Optional[str] = None
                error: Optional[str] = None

                try:
                    if is_self:
                        result = owner_node.execute_nft(
                            executor_did=owner_did,
                            nft_id=nft_id,
                            data=f"repeated self-exec round {r+1}/{rounds}",
                        )
                    else:
                        result = opp_node.execute_nft(
                            executor_did=opp_did,
                            nft_id=nft_id,
                            data=f"repeated cross-exec round {r+1}/{rounds}",
                        )
                    req_id = result.get("req_id")
                    txn_id = (owner_node if is_self else opp_node).extract_txn_id(result)
                    stats[nft_id]["self" if is_self else "cross"] += 1
                except Exception as exc:
                    status = "FAIL"
                    error = str(exc)
                    stats[nft_id]["fail"] += 1
                    log.warning(
                        "[%s] round %d/%d %s FAIL: %s",
                        seq_id, r + 1, rounds, round_type, exc,
                    )

                duration_ms = int((time.time() - t0) * 1000)

                self.reporter.record_transaction({
                    "id": seq_id,
                    "type": f"NFT_REPEATED_{round_type}",
                    "node": owner_label if is_self else opp_label,
                    "nft_id": nft_id,
                    "round": r + 1,
                    "total_rounds": rounds,
                    "status": status,
                    "req_id": req_id,
                    "transaction_id": txn_id,
                    "duration_ms": duration_ms,
                    "timestamp": ts,
                    "error": error,
                })

                if status == "SUCCESS":
                    log.info(
                        "[%s] round %d/%d %s OK  %dms",
                        seq_id, r + 1, rounds, round_type, duration_ms,
                    )

                # Small settle delay between rounds
                time.sleep(2)

        # Log summary
        for nft_id, s in stats.items():
            log.info(
                "NFT %s: self=%d  cross=%d  fail=%d",
                nft_id[:12] + "...", s["self"], s["cross"], s["fail"],
            )

        total_ok = sum(s["self"] + s["cross"] for s in stats.values())
        total_fail = sum(s["fail"] for s in stats.values())
        log.info(
            "=== NFT REPEATED EXECUTION COMPLETE: %d success, %d fail ===",
            total_ok, total_fail,
        )

        return stats

    def _execute_nft_from_opposite_node(self, nft_info: dict) -> None:
        """Execute NFT from the opposite node after subscription.

        Args:
            nft_info: {"nft_id": str, "owner_did": str, "owner_node": NodeClient, "label": str}
        """
        nft_id = nft_info["nft_id"]
        owner_label = nft_info["label"]

        # Determine the opposite node
        if owner_label == "A":
            executor_node = self.node_b
            executor_did = self.did_b
            executor_label = "B"
        else:
            executor_node = self.node_a
            executor_did = self.did_a
            executor_label = "A"

        exec_seq_id = f"NFT-XEXEC-{nft_id[:8]}"

        log.info(
            "=== NFT Cross-node execution: nft=%s  owner=%s  executor=%s ===",
            nft_id[:12] + "...",
            owner_label,
            executor_label,
        )

        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            # Step 1: Opposite node subscribes to the NFT
            log.info("[%s] Node-%s subscribing to NFT...", exec_seq_id, executor_label)
            executor_node.subscribe_nft(nft_id)
            log.info("[%s] Node-%s successfully subscribed", exec_seq_id, executor_label)

            # Wait for subscription to propagate
            log.info("[%s] Waiting 10 seconds for subscription to settle...", exec_seq_id)
            time.sleep(10)

            # Step 2: Opposite node executes the NFT
            log.info("[%s] Node-%s executing NFT...", exec_seq_id, executor_label)
            result = executor_node.execute_nft(
                executor_did=executor_did,
                nft_id=nft_id,
                data="NFT cross-node execution test",
            )
            req_id = result.get("req_id")
            txn_id = executor_node.extract_txn_id(result)
            # Record that this NFT was executed cross-node on executor_node, so
            # run_verification can assert the chain synced onto that node.
            with self._deployed_lock:
                self._cross_executed[nft_id] = executor_node
            log.info(
                "[%s] Node-%s execution completed: req_id=%s",
                exec_seq_id,
                executor_label,
                req_id,
            )

        except Exception as exc:
            status = "FAIL"
            error = str(exc)
            log.error("[%s] NFT cross-node execution failed: %s", exec_seq_id, exc)

        duration_ms = int((time.time() - t0) * 1000)

        self.reporter.record_transaction(
            {
                "id": exec_seq_id,
                "type": "NFT_EXECUTE_CROSS_NODE",
                "node": executor_label,
                "transaction_id": txn_id,
                "executor_did": executor_did[:20] + "...",
                "nft_id": nft_id,
                "owner_label": owner_label,
                "status": status,
                "req_id": req_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
            }
        )
