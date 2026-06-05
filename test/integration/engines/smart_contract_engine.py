"""
smart_contract_engine.py — Smart contract deployment and execution engine for stress testing.

Workflow:
  - Deploy smart contracts from nodeA
  - Subscribe to contracts from nodeB
  - Execute contracts from nodeB (cross-node execution)
"""

from __future__ import annotations

import logging
import threading
import time
from datetime import datetime, timezone
from typing import TYPE_CHECKING, Optional

if TYPE_CHECKING:
    from test.integration.clients.api_client import NodeClient
    from test.integration.config import StressConfig
    from test.integration.engines.reporter import StressReporter

try:
    from test.integration.engines.file_selector import select_smart_contract_files
except ImportError:
    import sys
    import os
    sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))
    from test.integration.engines.file_selector import select_smart_contract_files

try:
    from test.integration.engines.callback_receiver import CallbackReceiver
except ImportError:
    import sys
    import os
    sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))
    from test.integration.engines.callback_receiver import CallbackReceiver

log = logging.getLogger(__name__)


class SmartContractEngine:
    """Runs smart contract deployment and cross-node execution phases.

    Args:
        node_a, node_b: NodeClient instances.
        did_a, did_b:   DIDs for nodeA and nodeB respectively.
        config:         StressConfig.
        reporter:       StressReporter for recording SC operations.
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

        self._sc_counter = 0
        self._counter_lock = threading.Lock()

        # Track deployed smart contracts for cross-node execution
        self._deployed_contracts = []  # List of {"sc_id": str, "deployer_did": str, "deployer_node": NodeClient, "label": str}
        self._deployed_lock = threading.Lock()

    # ------------------------------------------------------------------
    # Deployment phase
    # ------------------------------------------------------------------

    def run_deployment(self, sc_count: int) -> None:
        """Deploy smart contracts from nodeA.

        Args:
            sc_count: Number of smart contracts to deploy
        """
        log.info("=== SMART CONTRACT DEPLOYMENT START ===")
        log.info("Deploying %d smart contracts from nodeA", sc_count)

        for i in range(sc_count):
            self._deploy_smart_contract()

        log.info(
            "=== SMART CONTRACT DEPLOYMENT COMPLETE: %d contracts deployed ===",
            self._sc_counter,
        )

    def _deploy_smart_contract(self) -> None:
        """Deploy a single smart contract from nodeA."""
        with self._counter_lock:
            self._sc_counter += 1
            sc_counter = self._sc_counter
            sc_seq_id = f"SC-{sc_counter:05d}"

        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        sc_id: Optional[str] = None
        deploy_req_id: Optional[str] = None
        deploy_txn_id: Optional[str] = None
        error: Optional[str] = None
        wasm_file: Optional[str] = None
        source_file: Optional[str] = None

        try:
            # Step 1: Select random .wasm and .rs files
            wasm_path, source_path = select_smart_contract_files()
            wasm_file = wasm_path
            source_file = source_path

            # Step 2: Generate smart contract
            result = self.node_a.create_smart_contract(self.did_a, wasm_path, source_path)
            sc_id = result.get("smartContractId")

            # Step 3: Deploy smart contract (first execution)
            if sc_id:
                log.info(
                    "[%s] Deploying smart contract %s to blockchain...",
                    sc_seq_id,
                    sc_id[:12] + "...",
                )
                deploy_result = self.node_a.deploy_smart_contract(
                    initiator_did=self.did_a,
                    sc_id=sc_id,
                    data="deployment",
                )
                deploy_req_id = deploy_result.get("req_id")
                deploy_txn_id = self.node_a.extract_txn_id(deploy_result)
                log.info(
                    "[%s] Smart contract deployed, deploy_req_id=%s",
                    sc_seq_id,
                    deploy_req_id,
                )

                # Save deployed contract info for cross-node execution
                with self._deployed_lock:
                    self._deployed_contracts.append({
                        "sc_id": sc_id,
                        "deployer_did": self.did_a,
                        "deployer_node": self.node_a,
                        "label": "A",
                    })
            else:
                log.warning("[%s] No SC ID returned, skipping deployment", sc_seq_id)

        except Exception as exc:
            status = "FAIL"
            error = str(exc)
            log.error("[%s] Smart contract deployment failed: %s", sc_seq_id, exc)

        duration_ms = int((time.time() - t0) * 1000)

        self.reporter.record_transaction(
            {
                "id": sc_seq_id,
                "type": "SC_DEPLOY",
                "node": "A",
                "did": self.did_a[:20] + "...",
                "status": status,
                "transaction_id": deploy_txn_id,
                "sc_id": sc_id,
                "deploy_req_id": deploy_req_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
                "wasm": wasm_file,
                "source": source_file,
            }
        )

        if status == "SUCCESS":
            log.info(
                "[%s] Node-A  sc=%s  deploy_req=%s  %dms",
                sc_seq_id,
                sc_id[:12] + "..." if sc_id else "N/A",
                deploy_req_id,
                duration_ms,
            )
        else:
            log.warning(
                "[%s] Node-A  FAIL  %dms  error=%s",
                sc_seq_id,
                duration_ms,
                error,
            )

    # ------------------------------------------------------------------
    # Cross-node execution phase
    # ------------------------------------------------------------------

    def run_cross_node_execution(self) -> None:
        """Execute deployed smart contracts from nodeB (cross-node execution).

        For each contract deployed by nodeA:
        1. NodeB subscribes to the contract
        2. NodeB executes the contract
        """
        with self._deployed_lock:
            if not self._deployed_contracts:
                log.warning("No deployed smart contracts available for execution")
                return

            contracts_to_execute = list(self._deployed_contracts)

        log.info("=== SMART CONTRACT CROSS-NODE EXECUTION START ===")
        log.info("Executing %d smart contracts from nodeB", len(contracts_to_execute))

        # Wait for smart contract deployments to settle on blockchain
        log.info("Waiting 5 seconds for smart contract deployments to settle...")
        time.sleep(5)

        for contract_info in contracts_to_execute:
            self._execute_from_opposite_node(contract_info)

        # Wait for any pending P2P sync operations to complete before teardown
        log.info("Waiting 5 seconds for cross-node sync operations to finalize...")
        time.sleep(5)

        log.info("=== SMART CONTRACT CROSS-NODE EXECUTION COMPLETE ===")

    def _execute_from_opposite_node(self, contract_info: dict) -> None:
        """Execute smart contract from nodeB after subscription.

        Args:
            contract_info: {"sc_id": str, "deployer_did": str, "deployer_node": NodeClient, "label": str}
        """
        sc_id = contract_info["sc_id"]
        deployer_label = contract_info["label"]

        exec_seq_id = f"EXEC-{sc_id[:8]}"

        log.info(
            "=== Cross-node execution: sc=%s  deployed_by=%s  executing_from=%s ===",
            sc_id[:12] + "...",
            deployer_label,
            "B",
        )

        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            # Step 1: NodeB subscribes to the smart contract
            log.info("[%s] NodeB subscribing to smart contract...", exec_seq_id)
            self.node_b.subscribe_smart_contract(sc_id)
            log.info("[%s] NodeB successfully subscribed", exec_seq_id)

            # Wait for subscription to propagate on blockchain
            log.info("[%s] Waiting 10 seconds for subscription to settle...", exec_seq_id)
            time.sleep(10)

            # Step 2: NodeB executes the smart contract
            log.info("[%s] NodeB executing smart contract...", exec_seq_id)
            result = self.node_b.execute_smart_contract(
                executor_did=self.did_b,
                sc_id=sc_id,
                data="cross_node_execution",
            )
            req_id = result.get("req_id")
            txn_id = self.node_b.extract_txn_id(result)
            log.info(
                "[%s] NodeB execution completed: req_id=%s",
                exec_seq_id,
                req_id,
            )

        except Exception as exc:
            status = "FAIL"
            error = str(exc)
            log.error("[%s] Cross-node execution failed: %s", exec_seq_id, exc)

        duration_ms = int((time.time() - t0) * 1000)

        self.reporter.record_transaction(
            {
                "id": exec_seq_id,
                "type": "SC_EXECUTE_CROSS_NODE",
                "node": "B",
                "transaction_id": txn_id,
                "executor_did": self.did_b[:20] + "...",
                "sc_id": sc_id,
                "deployed_by": deployer_label,
                "status": status,
                "req_id": req_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
            }
        )

    # ------------------------------------------------------------------
    # Repeated Execution Phase (mixed self + cross-node)
    # ------------------------------------------------------------------

    def run_repeated_executions(self, rounds: int) -> dict:
        """Execute each SC multiple rounds, alternating self-execute and cross-node execute.

        For round i (0-based):
          - even rounds → self-execute on deployer node (nodeA)
          - odd rounds  → cross-node execute from subscriber node (nodeB)

        Assumes nodeB has already subscribed to each SC from the prior cross-node execution phase.

        Args:
            rounds: Number of execution rounds per smart contract.

        Returns:
            Dict mapping sc_id → {"self": int, "cross": int, "fail": int} with actual success counts.
        """
        with self._deployed_lock:
            contracts = list(self._deployed_contracts)

        if not contracts:
            log.warning("No deployed smart contracts available for repeated execution")
            return {}

        log.info(
            "=== SC REPEATED EXECUTION START: %d rounds × %d contracts ===",
            rounds, len(contracts),
        )

        exec_counter = 0
        stats: dict = {}  # sc_id → {"self": 0, "cross": 0, "fail": 0}

        for contract_info in contracts:
            sc_id = contract_info["sc_id"]
            deployer_did = contract_info["deployer_did"]
            deployer_node = contract_info["deployer_node"]

            stats[sc_id] = {"self": 0, "cross": 0, "fail": 0}

            log.info(
                "--- SC %s (deployer=A): starting %d rounds ---",
                sc_id[:12] + "...", rounds,
            )

            for r in range(rounds):
                exec_counter += 1
                is_self = (r % 2 == 0)
                round_type = "SELF" if is_self else "CROSS"
                seq_id = f"SC-REXEC-{exec_counter:04d}"

                ts = datetime.now(tz=timezone.utc).isoformat()
                t0 = time.time()
                status = "SUCCESS"
                req_id: Optional[str] = None
                txn_id: Optional[str] = None
                error: Optional[str] = None

                try:
                    if is_self:
                        result = deployer_node.execute_smart_contract(
                            executor_did=deployer_did,
                            sc_id=sc_id,
                            data=f"repeated self-exec round {r+1}/{rounds}",
                        )
                    else:
                        result = self.node_b.execute_smart_contract(
                            executor_did=self.did_b,
                            sc_id=sc_id,
                            data=f"repeated cross-exec round {r+1}/{rounds}",
                        )
                    req_id = result.get("req_id")
                    txn_id = (deployer_node if is_self else self.node_b).extract_txn_id(result)
                    stats[sc_id]["self" if is_self else "cross"] += 1
                except Exception as exc:
                    status = "FAIL"
                    error = str(exc)
                    stats[sc_id]["fail"] += 1
                    log.warning(
                        "[%s] round %d/%d %s FAIL: %s",
                        seq_id, r + 1, rounds, round_type, exc,
                    )

                duration_ms = int((time.time() - t0) * 1000)

                self.reporter.record_transaction({
                    "id": seq_id,
                    "type": f"SC_REPEATED_{round_type}",
                    "node": "A" if is_self else "B",
                    "sc_id": sc_id,
                    "round": r + 1,
                    "total_rounds": rounds,
                    "status": status,
                    "transaction_id": txn_id,
                    "req_id": req_id,
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
        for sc_id, s in stats.items():
            log.info(
                "SC %s: self=%d  cross=%d  fail=%d",
                sc_id[:12] + "...", s["self"], s["cross"], s["fail"],
            )

        total_ok = sum(s["self"] + s["cross"] for s in stats.values())
        total_fail = sum(s["fail"] for s in stats.values())
        log.info(
            "=== SC REPEATED EXECUTION COMPLETE: %d success, %d fail ===",
            total_ok, total_fail,
        )

        return stats

    # ------------------------------------------------------------------
    # Self-execution phase
    # ------------------------------------------------------------------

    def run_self_execute(self) -> None:
        """Execute smart contract on the same node that deployed it (self-execution).

        Uses the first deployed contract and executes it with the same owner.
        """
        with self._deployed_lock:
            if not self._deployed_contracts:
                log.warning("No deployed smart contracts available for self-execution")
                return

            contract_info = self._deployed_contracts[0]

        sc_id = contract_info["sc_id"]
        deployer_did = contract_info["deployer_did"]
        deployer_node = contract_info["deployer_node"]
        label = contract_info["label"]

        log.info(
            "=== Smart Contract Self-Execute: sc=%s  owner=%s (Node-%s) ===",
            sc_id[:12] + "...",
            deployer_did[:20] + "...",
            label,
        )

        # Wait for smart contract deployment to settle on blockchain
        log.info("Waiting 5 seconds for smart contract deployment to settle...")
        time.sleep(5)

        ts = datetime.now(tz=timezone.utc).isoformat()
        t0 = time.time()
        status = "SUCCESS"
        req_id: Optional[str] = None
        txn_id: Optional[str] = None
        error: Optional[str] = None

        try:
            result = deployer_node.execute_smart_contract(
                executor_did=deployer_did,
                sc_id=sc_id,
                data="Smart contract self-execution test",
            )
            req_id = result.get("req_id")
            txn_id = deployer_node.extract_txn_id(result)
            log.info(
                "Smart contract self-execution completed: req_id=%s",
                req_id,
            )
        except Exception as exc:
            status = "FAIL"
            error = str(exc)
            log.error("Smart contract self-execution failed: %s", exc)

        duration_ms = int((time.time() - t0) * 1000)

        self.reporter.record_transaction(
            {
                "id": "SC-EXEC-SELF-001",
                "type": "SC_EXECUTE_SELF",
                "node": label,
                "did": deployer_did[:20] + "...",
                "sc_id": sc_id,
                "status": status,
                "req_id": req_id,
                "transaction_id": txn_id,
                "duration_ms": duration_ms,
                "timestamp": ts,
                "error": error,
            }
        )

    # ------------------------------------------------------------------
    # Smart Contract Verification Phase
    # ------------------------------------------------------------------

    def run_verification(self, cross_executed: bool = False, expected_min_chain: int = 0) -> list:
        """Run verification checks using all smart contract query APIs.

        Exercises:
          - GET /rubix/v1/smart_contracts  (list all SCs on both nodes)
          - GET /rubix/v1/smart_contracts/{id}/chain  (chain per SC on deployer + executor)
          - POST /rubix/v1/smart_contracts/register_callback  (register callback on subscriber)
          - GET /rubix/v1/tx/{tx_id}  (lookup individual transactions)
          - GET /rubix/v1/tx  (list all transactions on both nodes)

        Args:
            cross_executed: If True, verify chain sync between deployer (A) and executor (B).
            expected_min_chain: If > 0, override the minimum expected chain length per SC.
                                Useful when repeated executions increase the chain length.

        Returns:
            List of verification result dicts: {"check": str, "status": "PASS"|"FAIL", "detail": str}
        """
        log.info("=== SMART CONTRACT VERIFICATION START ===")
        results = []

        # --- 1. List smart contracts on both nodes ---
        try:
            scs_a = self.node_a.list_smart_contracts()
            results.append({
                "check": "SC_LIST_NODE_A",
                "status": "PASS",
                "detail": f"nodeA has {len(scs_a)} smart contracts",
            })
        except Exception as exc:
            results.append({
                "check": "SC_LIST_NODE_A",
                "status": "FAIL",
                "detail": str(exc),
            })
            scs_a = []

        try:
            scs_b = self.node_b.list_smart_contracts()
            results.append({
                "check": "SC_LIST_NODE_B",
                "status": "PASS",
                "detail": f"nodeB has {len(scs_b)} smart contracts",
            })
        except Exception as exc:
            results.append({
                "check": "SC_LIST_NODE_B",
                "status": "FAIL",
                "detail": str(exc),
            })
            scs_b = []

        # --- 2. Get chain for each deployed SC on deployer node ---
        with self._deployed_lock:
            deployed_snapshot = list(self._deployed_contracts)

        for contract_info in deployed_snapshot:
            sc_id = contract_info["sc_id"]
            short_id = sc_id[:12] + "..."

            # Chain on deployer (nodeA)
            try:
                chain_a = self.node_a.get_smart_contract_chain(sc_id)
                chain_len = len(chain_a) if chain_a else 0
                # Use caller-provided minimum, or default: deploy=1, cross-execute=2
                expected_min = expected_min_chain if expected_min_chain > 0 else (2 if cross_executed else 1)
                passed = chain_len >= expected_min
                results.append({
                    "check": f"SC_CHAIN_{short_id}_NODE_A",
                    "status": "PASS" if passed else "FAIL",
                    "detail": f"chain has {chain_len} entries (expected >= {expected_min})",
                })
            except Exception as exc:
                results.append({
                    "check": f"SC_CHAIN_{short_id}_NODE_A",
                    "status": "FAIL",
                    "detail": str(exc),
                })

            # Chain on executor (nodeB) — only if cross-executed
            if cross_executed:
                try:
                    chain_b = self.node_b.get_smart_contract_chain(sc_id)
                    chain_len_b = len(chain_b) if chain_b else 0
                    passed = chain_len_b >= expected_min
                    results.append({
                        "check": f"SC_CHAIN_{short_id}_NODE_B",
                        "status": "PASS" if passed else "FAIL",
                        "detail": f"chain has {chain_len_b} entries (expected >= {expected_min}, sync verification)",
                    })

                    # Compare chain lengths between nodes
                    if chain_a and chain_b:
                        synced = len(chain_a) == len(chain_b)
                        results.append({
                            "check": f"SC_CHAIN_SYNC_{short_id}",
                            "status": "PASS" if synced else "FAIL",
                            "detail": f"nodeA={len(chain_a)} vs nodeB={len(chain_b)} entries",
                        })
                except Exception as exc:
                    results.append({
                        "check": f"SC_CHAIN_{short_id}_NODE_B",
                        "status": "FAIL",
                        "detail": str(exc),
                    })

        # --- 3. Register callback (test the API call succeeds) ---
        if deployed_snapshot:
            sc_id = deployed_snapshot[0]["sc_id"]
            short_id = sc_id[:12] + "..."
            try:
                self.node_b.register_smart_contract_callback(
                    sc_id, "http://localhost:9999/test-callback"
                )
                results.append({
                    "check": f"SC_REGISTER_CALLBACK_{short_id}",
                    "status": "PASS",
                    "detail": "callback registered on nodeB",
                })
            except Exception as exc:
                results.append({
                    "check": f"SC_REGISTER_CALLBACK_{short_id}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- 4. List all transactions on both nodes ---
        for node, label in [(self.node_a, "A"), (self.node_b, "B")]:
            try:
                txns = node.list_transactions()
                results.append({
                    "check": f"SC_TX_LIST_NODE_{label}",
                    "status": "PASS",
                    "detail": f"node{label} has {len(txns)} transactions",
                })
            except Exception as exc:
                results.append({
                    "check": f"SC_TX_LIST_NODE_{label}",
                    "status": "FAIL",
                    "detail": str(exc),
                })

        # --- Log summary ---
        passed = sum(1 for r in results if r["status"] == "PASS")
        failed = sum(1 for r in results if r["status"] == "FAIL")
        log.info(
            "=== SMART CONTRACT VERIFICATION COMPLETE: %d passed, %d failed ===",
            passed, failed,
        )
        for r in results:
            level = log.info if r["status"] == "PASS" else log.warning
            level("  [%s] %s: %s", r["status"], r["check"], r["detail"])

        return results

    # ------------------------------------------------------------------
    # Callback delivery verification (end-to-end HTTP trigger check)
    # ------------------------------------------------------------------

    def run_callback_delivery_check(self, wait_timeout: float = 20.0) -> list:
        """Verify that nodeB actually POSTs to the registered callback URL
        after an SC execute event lands.

        Preconditions:
          - nodeB has already subscribed to at least one of the SCs deployed
            by nodeA (satisfied by `run_cross_node_execution`).

        Flow:
          1. Start an HTTP receiver on the host.
          2. Register the receiver URL as the callback for the target SC on nodeB.
          3. Trigger a fresh self-execute from nodeA (publishes pubsub event).
          4. Wait up to `wait_timeout` for a matching POST on the receiver.
          5. Assert the payload contains the right `smart_contract_hash`.

        Returns a list of verification result dicts compatible with the other
        SC verification methods.
        """
        results: list = []
        log.info("=== SMART CONTRACT CALLBACK DELIVERY CHECK START ===")

        with self._deployed_lock:
            deployed_snapshot = list(self._deployed_contracts)
        if not deployed_snapshot:
            results.append({
                "check": "SC_CALLBACK_DELIVERY",
                "status": "FAIL",
                "detail": "no SCs deployed — cannot run delivery check",
            })
            return results

        target = deployed_snapshot[0]
        sc_id: str = target["sc_id"]
        deployer_did: str = target["deployer_did"]
        deployer_node = target["deployer_node"]
        short_id = sc_id[:12] + "..."

        rx = CallbackReceiver()
        try:
            rx.start()
            # "host" mode (native nodes on this machine) reaches the receiver via
            # 127.0.0.1; "docker" mode (containerised nodes) via host.docker.internal.
            if getattr(self.cfg, "callback_url_mode", "docker") == "host":
                receiver_url = rx.url_for_host()
            else:
                receiver_url = rx.url_for_docker()
            log.info(
                "Callback receiver URL (mode=%s) : %s",
                getattr(self.cfg, "callback_url_mode", "docker"), receiver_url,
            )

            # Step 1: register receiver URL on nodeB (upserts into call_back_urls).
            try:
                self.node_b.register_smart_contract_callback(sc_id, receiver_url)
                results.append({
                    "check": f"SC_CALLBACK_REGISTER_{short_id}",
                    "status": "PASS",
                    "detail": f"callback url registered on nodeB → {receiver_url}",
                })
            except Exception as exc:
                results.append({
                    "check": f"SC_CALLBACK_REGISTER_{short_id}",
                    "status": "FAIL",
                    "detail": f"register_smart_contract_callback raised: {exc}",
                })
                return results

            # Step 2: trigger a fresh execute from nodeA (deployer) — this
            # publishes a pubsub event that nodeB's subscriber consumes.
            rx.clear()
            trigger_status = "SUCCESS"
            trigger_error: Optional[str] = None
            try:
                deployer_node.execute_smart_contract(
                    executor_did=deployer_did,
                    sc_id=sc_id,
                    data="callback_delivery_check",
                )
            except Exception as exc:
                trigger_status = "FAIL"
                trigger_error = str(exc)

            results.append({
                "check": f"SC_CALLBACK_TRIGGER_EXECUTE_{short_id}",
                "status": "PASS" if trigger_status == "SUCCESS" else "FAIL",
                "detail": (
                    "nodeA self-execute OK"
                    if trigger_status == "SUCCESS"
                    else f"nodeA self-execute failed: {trigger_error}"
                ),
            })
            if trigger_status != "SUCCESS":
                return results

            # Step 3: wait for a POST whose body carries the expected SC hash.
            def _matches(ev: dict) -> bool:
                body = ev.get("body") or {}
                return bool(
                    isinstance(body, dict)
                    and body.get("smart_contract_hash") == sc_id
                )

            log.info(
                "Waiting up to %.1fs for callback POST for SC %s ...",
                wait_timeout, short_id,
            )
            hit = rx.wait_for_match(_matches, timeout=wait_timeout)

            if hit is None:
                all_events = rx.captured()
                results.append({
                    "check": f"SC_CALLBACK_DELIVERED_{short_id}",
                    "status": "FAIL",
                    "detail": (
                        f"no matching POST received within {wait_timeout:.0f}s "
                        f"(total POSTs seen: {len(all_events)})"
                    ),
                })
                return results

            body = hit.get("body") or {}
            detail_bits = [
                f"smart_contract_hash={body.get('smart_contract_hash', '')[:20]}...",
                f"initiator_did={str(body.get('initiator_did') or '')[:20]}...",
                f"data_len={len(str(body.get('smart_contract_data') or ''))}",
                f"from={hit.get('client')}",
            ]
            results.append({
                "check": f"SC_CALLBACK_DELIVERED_{short_id}",
                "status": "PASS",
                "detail": "  ".join(detail_bits),
            })

            # Extra assertion: initiator_did matches the deployer (since deployer
            # was the one executing in this trigger step).
            initiator_ok = str(body.get("initiator_did") or "") == deployer_did
            results.append({
                "check": f"SC_CALLBACK_INITIATOR_{short_id}",
                "status": "PASS" if initiator_ok else "FAIL",
                "detail": (
                    "initiator_did matches deployer DID"
                    if initiator_ok
                    else f"expected {deployer_did[:20]}... "
                         f"got {str(body.get('initiator_did'))[:20]}..."
                ),
            })
        finally:
            rx.stop()

        passed = sum(1 for r in results if r["status"] == "PASS")
        failed = sum(1 for r in results if r["status"] == "FAIL")
        log.info(
            "=== SMART CONTRACT CALLBACK DELIVERY CHECK COMPLETE: %d passed, %d failed ===",
            passed, failed,
        )
        for r in results:
            level = log.info if r["status"] == "PASS" else log.warning
            level("  [%s] %s: %s", r["status"], r["check"], r["detail"])
        return results

    @property
    def total_contracts(self) -> int:
        """Total number of smart contracts deployed so far."""
        with self._counter_lock:
            return self._sc_counter
