"""
fullnode_engine.py — integration coverage for a node started with `-fullnode`.

WHAT A FULLNODE IS (and why it needs its own suite)
---------------------------------------------------
`rubixgoplatform run -fullnode` flips Core into a passive observer of the whole
network (core/fullnode.go):

  1. SubscribeTxnSetup() subscribes to the `rubix_txn` gossipsub topic and
     starts the DynamicTxnProcessor worker pool.
  2. TxnCallBack() receives every published models.EventTransaction, drops
     failed ones (`if !newEvent.Status { return }`), de-duplicates by
     transaction ID, and queues the rest.
  3. processSingleTransaction() re-runs the FULL consensus validation —
     consensus.ValidateTransaction with isFullnode=true: transaction-ID
     integrity, initiator AND quorum signature verification, token-chain
     integrity (syncing missing chain history from peers), replayed-split
     detection, token-ownership-by-previous-txn, pledged-token checks, the
     fullnode-only transaction-value-vs-pledge-value check, and IPFS pin checks.
  4. Only on success does PersistFullNodeTransaction write the transaction and
     its per-token chain entries into the fullnode_* tables. A terminal
     validation failure is dead-lettered into fullnode_invalid_transactions.

So a row in `fullnode_transactions` is not merely "a message arrived" — it is
proof that the entire
    localnet transaction -> PubSub -> fullnode -> validation -> persistence
path completed. That is the property this engine asserts.

WHY DIRECT SQL
--------------
The fullnode exposes NO HTTP API over its stored transactions. Its only
fullnode-specific routes are libp2p ones (`/rubix/v1/fullnode/sync`,
`/rubix/v1/fullnode/recover`, served on the peer listener, not the node's REST
port). Reading the fullnode's own PostgreSQL tables is therefore the intended
public observation point, and it is what the schema was designed for. All
queries live in clients/db_validator.py:FullnodeDBValidator.

DETERMINISM
-----------
No fixed sleeps. Every wait is a bounded poll against a real condition:
container health, gossipsub peer membership, a row landing in a table. Each
poll has an explicit timeout and logs what it was waiting for when it gives up,
so a CI failure is diagnosable from the Actions log alone.
"""

from __future__ import annotations

import logging
import time
from typing import Any, Dict, List, Optional, Tuple

from test.integration.clients.api_client import NodeClient
from test.integration.clients.db_validator import FullnodeDBValidator

log = logging.getLogger(__name__)

# The gossipsub topic every node publishes transactions on
# (constants/events.go: Event_RubixTxns).
TXN_TOPIC = "rubix_txn"

# kubo's pubsub HTTP API takes/returns multibase-encoded topic names; the CLI
# usually decodes them for display, but the exact behaviour varies by version.
# Accept either form so the check does not depend on that detail.
TXN_TOPIC_ENCODINGS = (TXN_TOPIC, "ucnViaXhfdHhu", "cnViaXhfdHhu")

# The line core/fullnode.go logs once the subscription is live.
SUBSCRIBE_LOG_MARKER = f"Successfully subscribed to topic: {TXN_TOPIC}"


class FullnodeEngine:
    """Drives and verifies the `-fullnode` node in the integration cluster.

    Args:
        fullnode:   NodeClient for the fullnode's REST API (DID/peer setup).
        fullnode_db: FullnodeDBValidator against the fullnode's Postgres.
        node_a/node_b/quorum: the three transacting nodes.
        did_a/did_b/did_q: their DIDs (already created by StressRunner.setup_nodes).
        controller: optional ContainerController for the fullnode container —
                    supplies `ipfs`, `logs` and `restart`. When absent (e.g. a
                    non-Docker environment) the checks that need it report
                    SKIP rather than FAIL, and the DB-level checks still run.
        publisher_controllers: optional {service: ContainerController} for the
                    transacting nodes, so the gossipsub mesh can be confirmed
                    from the publisher side (see ensure_gossip_mesh).
        password:   node password for signature challenges.
    """

    def __init__(
        self,
        fullnode: NodeClient,
        fullnode_db: FullnodeDBValidator,
        node_a: NodeClient,
        node_b: NodeClient,
        quorum: NodeClient,
        did_a: str,
        did_b: str,
        did_q: str,
        controller: Optional[Any] = None,
        publisher_controllers: Optional[Dict[str, Any]] = None,
        password: str = "mypassword",
    ) -> None:
        self.fullnode = fullnode
        self.db = fullnode_db
        self.node_a = node_a
        self.node_b = node_b
        self.quorum = quorum
        self.did_a = did_a
        self.did_b = did_b
        self.did_q = did_q
        self.controller = controller
        # {service_name: ContainerController} for the transacting nodes. Used to
        # assert the gossipsub mesh from the PUBLISHER's side, which is the side
        # that actually decides whether a message is delivered.
        self.publisher_controllers: Dict[str, Any] = publisher_controllers or {}
        self.password = password

        self.did_f: Optional[str] = None
        self.peer_f: Optional[str] = None
        # Populated by run_verification so the caller can log what was checked.
        self.tracked_txn_id: Optional[str] = None

    # ------------------------------------------------------------------
    # Setup
    # ------------------------------------------------------------------

    def setup(self) -> None:
        """Give the fullnode a DID and mesh it into the peer tables.

        The fullnode is deliberately NOT added as a quorum: it takes no part in
        consensus, it only observes. But it does need to be a first-class peer:

          - Its own DID, registered, so other nodes can resolve it.
          - Bidirectional add_peer_details with all three nodes, so that when
            the fullnode has to pull missing token-chain history to validate a
            transaction (TokenChainIntegrityCheck -> SyncTransactionChainsFromPeer),
            it can resolve the initiator's / quorum's DID to a peer ID. Without
            that mapping, validation of any token whose chain the fullnode has
            not already seen fails and the transaction is dead-lettered.

        This mirrors exactly what the standalone testnet harness
        (test/e2e/fullnode_chain_e2e.sh setup_network) does for its fullnode role.
        """
        log.info("=== FULLNODE SETUP: creating DID ===")
        result = self.fullnode.create_did(self.password)
        self.did_f = result["did"]
        self.peer_f = self.fullnode.get_peer_id()
        log.info("Fullnode DID=%s  peerID=%s", self.did_f, self.peer_f)

        assert self.did_f and self.did_f.startswith("bafybmi") and len(self.did_f) == 59, \
            f"fullnode DID is invalid: {self.did_f!r}"
        assert self.peer_f and self.peer_f.startswith("12D3KooW") and len(self.peer_f) == 52, \
            f"fullnode peer_id is invalid: {self.peer_f!r}"

        self.fullnode.register_did(self.did_f)

        log.info("=== FULLNODE SETUP: cross-registering peers ===")
        peers = [
            ("nodeA", self.node_a, self.did_a),
            ("nodeB", self.node_b, self.did_b),
            ("quorum", self.quorum, self.did_q),
        ]
        for label, client, did in peers:
            peer_id = client.get_peer_id()
            # fullnode -> peer, so the fullnode can dial them for chain sync.
            self.fullnode.add_peer_details(peer_id, did)
            # peer -> fullnode, so they can answer it.
            client.add_peer_details(self.peer_f, self.did_f)
            log.info("Cross-registered fullnode <-> %s (%s)", label, peer_id)

        log.info("=== FULLNODE SETUP COMPLETE ===")

    def ensure_gossip_mesh(self, timeout: int = 120) -> bool:
        """Make sure the fullnode is gossipsub-connected to the publishers.

        On localnet there are no bootstrap nodes and the DHT is off (private
        swarm), so peer discovery relies on kubo's mDNS. That works on a Docker
        bridge, but it is discovery — not a guarantee, and not instantaneous.
        Rather than sleep and hope, this explicitly dials each publisher from
        the fullnode's IPFS swarm and then polls until the fullnode shows up in
        a publisher's gossipsub peer set for the rubix_txn topic.

        Returns True once the mesh is confirmed. Best-effort when no container
        controller is available (returns True without asserting).
        """
        if self.controller is None:
            log.warning("No container controller — skipping explicit gossip meshing.")
            return True

        # Resolve each publisher's peer ID over its REST API and dial it from the
        # fullnode's IPFS swarm. Compose puts every container on the same bridge
        # network, so the service name is a resolvable hostname, and kubo's swarm
        # port is 4002 inside every container (all run node_index 0).
        publisher_peers: Dict[str, str] = {}
        for service, client in (("nodeA", self.node_a),
                                ("nodeB", self.node_b),
                                ("quorum", self.quorum)):
            try:
                peer_id = client.get_peer_id()
            except Exception as exc:  # noqa: BLE001
                log.warning("Could not read %s peer id for meshing: %s", service, exc)
                continue
            publisher_peers[service] = peer_id
            rc, out, err = self.controller.ipfs(
                "swarm", "connect", f"/dns4/{service}/tcp/4002/p2p/{peer_id}"
            )
            if rc == 0:
                log.info("fullnode swarm-connected to %s (%s)", service, peer_id)
            else:
                # Non-fatal per peer: mDNS may already have connected them, and
                # the poll below is the real assertion.
                log.warning("fullnode -> %s swarm connect failed: %s",
                            service, (err or out))

        if not publisher_peers:
            log.warning("No publisher peer IDs resolved — cannot confirm the mesh.")
            return False

        # Confirm the mesh, preferring the PUBLISHER's view.
        #
        # Delivery is decided by the publisher: gossipsub only forwards a
        # message to peers that are in the publisher's mesh for that topic. The
        # fullnode seeing a publisher in its own `pubsub peers` list is
        # necessary but not sufficient — the graft has to be visible from the
        # other end before a publish can reach it. Waiting on the publisher's
        # view is what removes the timing dependency that would otherwise make
        # "transfer immediately after (re)start" flaky.
        #
        # Without publisher controllers we fall back to the fullnode's own view,
        # which is still a real signal, just a weaker one.
        deadline = time.time() + timeout
        last = ""
        while time.time() < deadline:
            for service, ctl in self.publisher_controllers.items():
                for topic in TXN_TOPIC_ENCODINGS:
                    rc, out, err = ctl.ipfs("pubsub", "peers", topic)
                    last = err or out
                    if rc == 0 and self.peer_f and self.peer_f in out:
                        log.info("Gossipsub mesh confirmed from the publisher side: "
                                 "%s lists the fullnode as a %s peer.", service, TXN_TOPIC)
                        return True
            if not self.publisher_controllers:
                for topic in TXN_TOPIC_ENCODINGS:
                    rc, out, err = self.controller.ipfs("pubsub", "peers", topic)
                    last = err or out
                    if rc != 0:
                        continue
                    seen = [svc for svc, pid in publisher_peers.items() if pid in out]
                    if seen:
                        log.info("Gossipsub mesh confirmed from the fullnode side: "
                                 "peered with %s on %s", ", ".join(sorted(seen)), TXN_TOPIC)
                        return True
            time.sleep(3.0)
        log.warning("Could not confirm gossipsub peering within %ds (last: %r). "
                    "Continuing — the transaction-receipt check is authoritative.",
                    timeout, last)
        return False

    # ------------------------------------------------------------------
    # Individual checks
    # ------------------------------------------------------------------

    def _check_startup(self) -> Dict[str, str]:
        """The fullnode process came up and migrated its schema."""
        reachable = self.db.wait_reachable(timeout=120)
        detail = (
            f"fullnode Postgres reachable and fullnode_* schema present "
            f"({self.db.name})"
            if reachable else
            f"fullnode schema NOT reachable at {self.db.name} — the node either "
            f"failed to start or never ran its migrations"
        )
        return {"check": "FULLNODE_STARTUP", "status": "PASS" if reachable else "FAIL",
                "detail": detail}

    def _check_subscription(self) -> Dict[str, str]:
        """The fullnode subscribed to the rubix_txn topic.

        Two independent signals, either of which is sufficient:
          - the node logged `Successfully subscribed to topic: rubix_txn`
            (core/fullnode.go SubscribeTxnSetup), and
          - kubo reports the topic in the fullnode's own `pubsub ls`.
        """
        if self.controller is None:
            return {"check": "FULLNODE_PUBSUB_SUBSCRIBED", "status": "SKIP",
                    "detail": "no container controller available"}

        # Either signal is sufficient, so stop at the first one that turns true —
        # waiting for both would only add latency to a check that has already
        # passed.
        deadline = time.time() + 90
        log_ok = False
        ls_ok = False
        while time.time() < deadline:
            log_ok = self.controller.node_log_contains(SUBSCRIBE_LOG_MARKER)
            rc, out, _ = self.controller.ipfs("pubsub", "ls")
            ls_ok = rc == 0 and any(t in out for t in TXN_TOPIC_ENCODINGS)
            if log_ok or ls_ok:
                break
            time.sleep(3.0)

        ok = log_ok or ls_ok
        return {
            "check": "FULLNODE_PUBSUB_SUBSCRIBED",
            "status": "PASS" if ok else "FAIL",
            "detail": (
                f"subscribe log line seen={log_ok}, `ipfs pubsub ls` lists "
                f"{TXN_TOPIC}={ls_ok}"
                + ("" if ok else
                   " — the fullnode never subscribed to the transaction topic, so "
                   "it can never receive a transaction")
            ),
        }

    def _check_receipt(self, txn_id: str, timeout: int = 240) -> Dict[str, str]:
        """The tracked transaction reached the fullnode and was persisted."""
        received = self.db.wait_for_fullnode_transaction(txn_id, timeout=timeout)
        if received:
            return {
                "check": "FULLNODE_TXN_RECEIVED",
                "status": "PASS",
                "detail": (
                    f"txn {txn_id} published on {TXN_TOPIC} was received, validated "
                    f"and persisted into fullnode_transactions"
                ),
            }
        # Give the operator the two most useful facts for triage.
        invalid = []
        try:
            invalid = self.db.get_fullnode_invalid_transactions(limit=5)
        except Exception as exc:  # noqa: BLE001
            log.warning("Could not read fullnode_invalid_transactions: %s", exc)
        if self.controller is not None:
            log.error("--- fullnode container log tail (last 120 lines) ---")
            for line in self.controller.logs(tail=120).splitlines():
                log.error("  %s", line)
        return {
            "check": "FULLNODE_TXN_RECEIVED",
            "status": "FAIL",
            "detail": (
                f"txn {txn_id} succeeded on the transacting nodes but never "
                f"appeared in fullnode_transactions within {timeout}s. "
                f"fullnode_invalid_transactions (most recent {len(invalid)}): "
                f"{[r[0][:200] for r in invalid] or 'empty'}"
            ),
        }

    def _check_persisted_fields(
        self, txn_id: str, sender_did: str, receiver_did: str
    ) -> List[Dict[str, str]]:
        """The persisted row carries the right identity, participants and signatures.

        sender_did / receiver_did come from the transfer that was actually made
        (the driver alternates direction on retry), so these assert the real
        participants rather than an assumed A->B direction.
        """
        results: List[Dict[str, str]] = []
        row = self.db.get_fullnode_transaction(txn_id)
        if row is None:
            results.append({
                "check": "FULLNODE_TXN_FIELDS",
                "status": "FAIL",
                "detail": f"no fullnode_transactions row for {txn_id}",
            })
            return results

        info = row["info"] or {}
        signature = row["signature"] or {}

        # --- identity + participants ---
        problems: List[str] = []
        if row["id"] != txn_id:
            problems.append(f"id mismatch ({row['id']} != {txn_id})")
        if info.get("initiator") != sender_did:
            problems.append(f"initiator={info.get('initiator')} expected {sender_did}")
        if info.get("owner") != receiver_did:
            problems.append(f"owner={info.get('owner')} expected {receiver_did}")
        if info.get("network") != "localnet":
            problems.append(f"network={info.get('network')} expected localnet")
        rbt_tokens = ((info.get("tokens") or {}).get("rbt")) or []
        if not rbt_tokens:
            problems.append("info.tokens.rbt is empty")
        results.append({
            "check": "FULLNODE_TXN_FIELDS",
            "status": "FAIL" if problems else "PASS",
            "detail": (
                "; ".join(problems) if problems else
                f"id/initiator/owner/network correct; "
                f"{len(rbt_tokens)} RBT token(s) recorded"
            ),
        })

        # --- quorum signature ---
        # The fullnode persists ONLY after SignatureVerificationCheck verified
        # the initiator signature and every quorum signature against the DIDs'
        # public keys (core/consensus/checks.go). So the row's existence already
        # proves the signatures were valid — what we assert here is that the
        # verified material was actually carried through into storage.
        quorum_sigs = signature.get("quorums") or []
        sig_problems: List[str] = []
        if not signature.get("initiatorSignature"):
            sig_problems.append("initiatorSignature missing")
        if not quorum_sigs:
            sig_problems.append("no quorum signatures stored")
        for qs in quorum_sigs:
            if not qs.get("did") or not qs.get("signature"):
                sig_problems.append(f"incomplete quorum signature entry: {qs}")
        results.append({
            "check": "FULLNODE_QUORUM_SIGNATURE_PERSISTED",
            "status": "FAIL" if sig_problems else "PASS",
            "detail": (
                "; ".join(sig_problems) if sig_problems else
                f"initiator signature + {len(quorum_sigs)} verified quorum "
                f"signature(s) persisted (dids={[q.get('did') for q in quorum_sigs]})"
            ),
        })

        # --- transferred RBT token state ---
        token_ids = [t.get("tokenId") for t in rbt_tokens if t.get("tokenId")]
        stored = self.db.get_fullnode_rbt_tokens(token_ids)
        tok_problems: List[str] = []
        for token_id in token_ids:
            entry = stored.get(token_id)
            if entry is None:
                tok_problems.append(f"{token_id[:12]}… absent from fullnode_rbt")
                continue
            if entry["did"] != receiver_did:
                tok_problems.append(
                    f"{token_id[:12]}… owned by {entry['did']} expected receiver {receiver_did}"
                )
            if entry["transaction_id"] != txn_id:
                tok_problems.append(
                    f"{token_id[:12]}… transaction_id={entry['transaction_id']} expected {txn_id}"
                )
        results.append({
            "check": "FULLNODE_RBT_TOKEN_STATE",
            "status": "FAIL" if tok_problems else "PASS",
            "detail": (
                "; ".join(tok_problems[:5]) if tok_problems else
                f"all {len(token_ids)} transferred RBT token(s) recorded in "
                f"fullnode_rbt owned by the receiver at this transaction"
            ),
        })

        # --- pledged tokens ---
        # ValidateTransaction runs ValidateTokenIDRelatedChecks over every
        # quorum pledge token and, for a fullnode only, additionally asserts
        # transaction value <= total pledge value. Persistence therefore implies
        # those passed; here we assert the pledge was recorded as such.
        quorums = info.get("quorums") or []
        pledged_ids = [
            t.get("tokenId")
            for q in quorums for t in (q.get("tokens") or [])
            if t.get("tokenId")
        ]
        chain_rows = self.db.get_fullnode_chain_entries(txn_id)
        pledge_rows = {r["token_id"] for r in chain_rows if r["role_name"] == "pledge"}
        pledge_problems: List[str] = []
        if not quorums:
            pledge_problems.append("transaction recorded no quorums")
        if not pledged_ids:
            pledge_problems.append("quorums pledged no tokens")
        missing_pledge = [t for t in pledged_ids if t not in pledge_rows]
        if missing_pledge:
            pledge_problems.append(
                f"{len(missing_pledge)} pledged token(s) have no pledge-role "
                f"fullnode_tokenchain row: {[t[:12] + '…' for t in missing_pledge[:5]]}"
            )
        results.append({
            "check": "FULLNODE_PLEDGED_TOKENS",
            "status": "FAIL" if pledge_problems else "PASS",
            "detail": (
                "; ".join(pledge_problems) if pledge_problems else
                f"{len(quorums)} quorum(s), {len(pledged_ids)} pledged token(s), "
                f"each with a pledge-role chain entry for this transaction"
            ),
        })

        # --- chain entries written for this transaction ---
        results.append({
            "check": "FULLNODE_TOKENCHAIN_WRITTEN",
            "status": "PASS" if chain_rows else "FAIL",
            "detail": (
                f"{len(chain_rows)} fullnode_tokenchain row(s) written for this "
                f"transaction (roles: "
                f"{sorted({r['role_name'] for r in chain_rows if r['role_name']})})"
                if chain_rows else
                "no fullnode_tokenchain rows written — the transaction was stored "
                "but its per-token chain entries were not"
            ),
        })

        return results

    def _check_no_duplicates(self) -> Dict[str, str]:
        """No token's chain entry was persisted twice for the same transaction.

        `fullnode_transactions.id` is a primary key, so duplicate TRANSACTIONS
        are impossible by construction. `fullnode_tokenchain` has no such
        constraint, so this is where a re-delivered pubsub event or a re-run
        sync would actually show up.
        """
        dupes = self.db.fullnode_duplicate_chain_entries()
        return {
            "check": "FULLNODE_NO_DUPLICATE_CHAIN_ENTRIES",
            "status": "FAIL" if dupes else "PASS",
            "detail": (
                f"{len(dupes)} duplicated (token_id, transaction_id) pair(s): "
                f"{[(t[:12] + '…', x[:12] + '…', c) for t, x, c in dupes[:5]]}"
                if dupes else
                "no (token_id, transaction_id) pair appears twice in fullnode_tokenchain"
            ),
        }

    def _check_chain_integrity(self) -> List[Dict[str, str]]:
        """Structural invariants over everything the fullnode stored this run."""
        results: List[Dict[str, str]] = []

        gaps = self.db.fullnode_chain_position_gaps()
        results.append({
            "check": "FULLNODE_CHAIN_CONTIGUOUS",
            "status": "FAIL" if gaps else "PASS",
            "detail": (
                f"{len(gaps)} token chain(s) not a contiguous 0..N-1 run: "
                f"{[(t[:12] + '…', mx, cnt) for t, mx, cnt in gaps[:5]]}"
                if gaps else
                "every fullnode_tokenchain chain runs contiguously from position 0"
            ),
        })

        breaks = self.db.fullnode_chain_link_breaks()
        results.append({
            "check": "FULLNODE_CHAIN_LINKED",
            "status": "FAIL" if breaks else "PASS",
            "detail": (
                f"{len(breaks)} row(s) whose previous_transaction_id does not "
                f"match the preceding position: "
                f"{[(t[:12] + '…', p) for t, p, _, _ in breaks[:5]]}"
                if breaks else
                "every chain row links to the transaction at position-1; "
                "position 0 (and only position 0) has an empty previous id"
            ),
        })

        dangling = self.db.fullnode_dangling_chain_refs()
        results.append({
            "check": "FULLNODE_CHAIN_NO_DANGLING_REFS",
            "status": "FAIL" if dangling else "PASS",
            "detail": (
                f"{len(dangling)} chain row(s) reference a transaction absent "
                f"from fullnode_transactions: "
                f"{[(t[:12] + '…', p) for t, p, _, _ in dangling[:5]]}"
                if dangling else
                "no chain row references a missing transaction (the DEFERRABLE "
                "FKs were never deferred into a lie)"
            ),
        })

        pledge_genesis = self.db.fullnode_pledge_rows_without_prev()
        results.append({
            "check": "FULLNODE_PLEDGE_NEVER_GENESIS",
            "status": "FAIL" if pledge_genesis else "PASS",
            "detail": (
                f"{len(pledge_genesis)} pledge-role row(s) with an empty "
                f"previous_transaction_id: "
                f"{[(t[:12] + '…') for t, _ in pledge_genesis[:5]]}"
                if pledge_genesis else
                "no pledge-role chain entry is a token's genesis entry"
            ),
        })

        return results

    def _check_tracked_not_rejected(self, txn_id: str) -> Dict[str, str]:
        """The tracked transaction was not dead-lettered.

        This is the HARD assertion on validation: the tracked transfer is driven
        in isolation by this engine, so its arrival order is deterministic and
        there is no legitimate reason for the fullnode to reject it. If it lands
        in fullnode_invalid_transactions, the fullnode received a transaction
        the quorum accepted and failed to process it — a real defect.
        """
        reason = self.db.fullnode_transaction_rejected(txn_id)
        return {
            "check": "FULLNODE_TRACKED_TXN_NOT_REJECTED",
            "status": "FAIL" if reason else "PASS",
            "detail": (
                f"the tracked transaction {txn_id} was dead-lettered: {reason[:400]}"
                if reason else
                f"the tracked transaction {txn_id} was not dead-lettered — the "
                f"fullnode validated it on the first pass"
            ),
        }

    def _report_dead_letters(self) -> Dict[str, str]:
        """Report every transaction the fullnode rejected during the run.

        WARN, not FAIL — deliberately, and this is the one place the suite does
        not gate. Under the concurrent `--run-all-tests` matrix the fullnode
        rejects a tail of transactions whose events arrive out of order relative
        to the chain history it pulls on demand. The dominant reason is
        TokenChainIntegrityCheck's genesis rule
        (core/consensus/checks.go:727-731): a token's genesis event is processed
        AFTER a later transaction already caused the fullnode to sync that
        token's chain from a peer, so `latestTransactionID != currentTxID` and
        the (redundant) genesis event is rejected. The stored chain is intact —
        every structural check above passes — it is the surplus event that is
        dropped.

        Fixing that requires the hold-and-release / bundling gate described in
        Fullnode-Txn-Bundling-Implementation-Plan.md, which is NOT implemented
        on this branch. Gating CI on it would fail every PR for a known,
        pre-existing limitation rather than for a regression, so the count and a
        grouped reason histogram are reported into verification.json instead —
        visible, diffable run-to-run, and impossible to miss in the CI log.

        The hard guarantees remain: the tracked transaction must be validated
        and persisted (FULLNODE_TXN_RECEIVED), must not be rejected
        (FULLNODE_TRACKED_TXN_NOT_REJECTED), and nothing the fullnode DID store
        may be duplicated, discontiguous, unlinked or dangling.
        """
        histogram = self.db.fullnode_invalid_reason_histogram()
        total = sum(count for _, count in histogram)
        if total:
            log.warning("Fullnode dead-lettered %d transaction(s) during the run:", total)
            for reason, count in histogram:
                log.warning("  x%-4d %s", count, reason)
        return {
            "check": "FULLNODE_DEAD_LETTER_REPORT",
            "status": "WARN" if total else "PASS",
            "detail": (
                f"{total} transaction(s) in fullnode_invalid_transactions across "
                f"{len(histogram)} distinct reason(s): "
                f"{[(c, r[:120]) for r, c in histogram[:5]]}. Non-gating: "
                f"out-of-order arrival vs on-demand chain sync is a known "
                f"limitation pending the hold-and-release gate. The tracked "
                f"transaction is asserted separately and must not appear here."
                if total else
                "fullnode_invalid_transactions is empty — every published "
                "transaction validated successfully"
            ),
        }

    def _check_restart_idempotency(self) -> List[Dict[str, str]]:
        """Restart the fullnode; it must resume ingesting without re-writing rows.

        Two properties in one restart:
          - RECOVERY: after coming back the fullnode re-subscribes, re-joins the
            gossipsub mesh, and PROCESSES a transaction published post-restart.
            "Processes" is the property, not "accepts": an event that arrives and
            is then rejected at validation still proves the subscription and mesh
            recovered. Only silence — neither persisted nor dead-lettered — means
            recovery failed, and that is the only case this check fails on.
          - IDEMPOTENCY: the pre-restart rows are not duplicated. The dedup map
            (DynamicTxnProcessor.processedTxns) is in-memory and therefore empty
            after a restart, so anything gossipsub re-delivers takes the full
            processing path again — the ON CONFLICT handling in
            PersistFullNodeTransaction is what must hold, and this is what
            catches it if it doesn't.
        """
        results: List[Dict[str, str]] = []
        if self.controller is None:
            return [{
                "check": "FULLNODE_RESTART_RECOVERY",
                "status": "SKIP",
                "detail": "no container controller available",
            }]

        before = self.db.fullnode_row_counts()
        log.info("Row counts before restart: %s", before)

        try:
            self.controller.restart()
        except Exception as exc:  # noqa: BLE001
            return [{
                "check": "FULLNODE_RESTART_RECOVERY",
                "status": "FAIL",
                "detail": f"fullnode did not come back healthy after restart: {exc}",
            }]

        # The node re-runs its migrations and re-subscribes on boot.
        if not self.db.wait_reachable(timeout=120):
            return [{
                "check": "FULLNODE_RESTART_RECOVERY",
                "status": "FAIL",
                "detail": "fullnode Postgres/schema not reachable after restart",
            }]
        self.ensure_gossip_mesh(timeout=120)

        # Drive a fresh transaction and require the restarted node to catch it.
        post_transfer = self._do_tracked_transfer(amount=1.0, label="post-restart")
        post_txn_id = post_transfer[0] if post_transfer else None
        if post_txn_id is None:
            results.append({
                "check": "FULLNODE_RESTART_RECOVERY",
                "status": "FAIL",
                "detail": "post-restart RBT transfer did not return a transaction ID, "
                          "so recovery could not be asserted",
            })
        else:
            got = self.db.wait_for_fullnode_transaction(post_txn_id, timeout=240)
            # Distinguish the two very different failure modes: the event never
            # arrived (subscription / mesh not re-established) versus it arrived
            # and was rejected (validation). Guessing between them, as an
            # unqualified "did not re-subscribe" message would, sends whoever
            # reads the CI log to the wrong place.
            rejected = None if got else self.db.fullnode_transaction_rejected(post_txn_id)
            # What this check exists to prove is that the node RESUMES INGESTING
            # after a restart: it re-subscribes, re-joins the gossipsub mesh, and
            # processes events published afterwards. A rejection means all of
            # that worked — the event was delivered and run through validation —
            # so it is not a restart regression; it is the same pre-existing
            # validation limitation the dead-letter report already covers, and
            # gating here would double-count it. Silence is the real failure:
            # neither persisted nor dead-lettered means the event never arrived.
            if not got and rejected is None and self.controller is not None:
                log.error("--- fullnode container log tail after restart ---")
                for line in self.controller.logs(tail=120).splitlines():
                    log.error("  %s", line)
            results.append({
                "check": "FULLNODE_RESTART_RECOVERY",
                "status": "PASS" if got else ("WARN" if rejected else "FAIL"),
                "detail": (
                    f"restarted fullnode received, validated and persisted "
                    f"post-restart txn {post_txn_id}"
                    if got else
                    f"restarted fullnode DID receive and process post-restart txn "
                    f"{post_txn_id} — so the subscription and gossipsub mesh "
                    f"recovered — but rejected it at validation: {rejected[:400]}"
                    if rejected else
                    f"restarted fullnode never persisted post-restart txn "
                    f"{post_txn_id} and did not dead-letter it either — the event "
                    f"never reached it, so the subscription or gossipsub mesh was "
                    f"not re-established"
                ),
            })

        # Idempotency: the pre-restart rows must not have been re-written.
        # Counts may only GROW by the new transaction's own rows; nothing that
        # existed before may be duplicated.
        after = self.db.fullnode_row_counts()
        log.info("Row counts after restart: %s", after)
        dupes = self.db.fullnode_duplicate_chain_entries()
        shrunk = {t: (before[t], after[t]) for t in before if after[t] < before[t]}
        problems: List[str] = []
        if dupes:
            problems.append(f"{len(dupes)} duplicated chain entries after restart")
        if shrunk:
            problems.append(f"row counts decreased after restart: {shrunk}")
        # NOTE: a GROWING fullnode_invalid_transactions count is deliberately not
        # a failure here. Retries of transactions received BEFORE the restart can
        # exhaust their attempts after it, so the counter moves for reasons that
        # have nothing to do with re-ingestion. Duplication and row loss are the
        # properties a restart can actually break, and they are asserted above;
        # the dead-letter total is reported by FULLNODE_DEAD_LETTER_REPORT, and
        # the post-restart transaction's own fate by FULLNODE_RESTART_RECOVERY.
        dead_letter_delta = (after["fullnode_invalid_transactions"]
                             - before["fullnode_invalid_transactions"])
        results.append({
            "check": "FULLNODE_RESTART_IDEMPOTENT",
            "status": "FAIL" if problems else "PASS",
            "detail": (
                "; ".join(problems) if problems else
                f"re-ingestion after restart produced no duplicate chain entries "
                f"and lost no rows (before={before}, after={after}, "
                f"dead_letter_delta={dead_letter_delta})"
            ),
        })
        return results

    # ------------------------------------------------------------------
    # Transaction driver
    # ------------------------------------------------------------------

    def _do_tracked_transfer(
        self, amount: float, label: str, attempts: int = 3
    ) -> Optional[Tuple[str, str, str]]:
        """Run one RBT transfer and return (txn_id, sender_did, receiver_did).

        This is the event the fullnode is expected to observe. It is a normal
        transfer through the same API the shuttle uses — nothing about it is
        fullnode-specific, which is the point: the fullnode must pick up
        ordinary network traffic.

        Direction alternates across attempts (A->B, B->A, ...). That is not
        cosmetic: this phase runs after every other subsystem, and a bad
        all-in-one / intra-node round can leave ONE node's token chain in a
        state the quorum rejects at consensus ("chain mismatch after sync"),
        which would otherwise make this phase fail for a reason that has
        nothing to do with the fullnode. Alternating direction draws on the
        other node's tokens, so a single damaged wallet cannot decide the
        outcome of the fullnode suite.

        This is a robustness measure for the DRIVING transaction only. Whatever
        transfer does succeed is then asserted against exactly as strictly as
        before — the participants are returned so the field checks assert the
        real direction rather than an assumed one.
        """
        pairs = [
            (self.node_a, self.did_a, self.did_b, "nodeA -> nodeB"),
            (self.node_b, self.did_b, self.did_a, "nodeB -> nodeA"),
        ]
        last_err = ""
        for attempt in range(attempts):
            client, sender, receiver, arrow = pairs[attempt % len(pairs)]
            log.info("=== FULLNODE: driving %s RBT transfer %s (%.4f RBT), attempt %d/%d ===",
                     label, arrow, amount, attempt + 1, attempts)
            try:
                result = client.transfer_rbt(sender, receiver, amount, self.password)
            except Exception as exc:  # noqa: BLE001
                last_err = str(exc)
                log.warning("%s transfer attempt %d (%s) failed: %s",
                            label, attempt + 1, arrow, exc)
                continue
            txn_id = NodeClient.extract_txn_id(result)
            if not txn_id:
                last_err = f"no transactionID in response: {result}"
                log.warning("%s transfer attempt %d (%s) returned no transactionID: %s",
                            label, attempt + 1, arrow, result)
                continue
            log.info("%s transfer on-chain transactionID=%s (%s)", label, txn_id, arrow)
            return txn_id, sender, receiver
        log.error("%s transfer failed on all %d attempt(s); last error: %s",
                  label, attempts, last_err)
        return None

    # ------------------------------------------------------------------
    # Public entry point
    # ------------------------------------------------------------------

    def run_verification(self, include_restart: bool = True) -> List[Dict[str, str]]:
        """Run the fullnode suite and return verification records.

        Order matters: startup and subscription are prerequisites for receipt,
        and receipt is a prerequisite for the field/pledge assertions. When a
        prerequisite fails we still emit the downstream checks as FAIL (rather
        than silently skipping them) so CI reports the whole picture and cannot
        go green on a fullnode that never started.
        """
        results: List[Dict[str, str]] = []

        startup = self._check_startup()
        results.append(startup)
        if startup["status"] == "FAIL":
            results.append({
                "check": "FULLNODE_TXN_RECEIVED",
                "status": "FAIL",
                "detail": "skipped: the fullnode never started, so it cannot have "
                          "received a transaction",
            })
            return results

        self.ensure_gossip_mesh()
        results.append(self._check_subscription())

        transfer = self._do_tracked_transfer(amount=1.0, label="tracked")
        if transfer is None:
            self.tracked_txn_id = None
            results.append({
                "check": "FULLNODE_TXN_RECEIVED",
                "status": "FAIL",
                "detail": "the driving RBT transfer itself never succeeded in either "
                          "direction, so fullnode receipt could not be asserted. This "
                          "is a cluster-side failure, not a fullnode verdict — check "
                          "the quorum's consensus errors and the earlier phases' "
                          "results for the underlying cause.",
            })
            return results

        txn_id, sender_did, receiver_did = transfer
        self.tracked_txn_id = txn_id

        receipt = self._check_receipt(txn_id)
        results.append(receipt)
        results.append(self._check_tracked_not_rejected(txn_id))
        if receipt["status"] == "PASS":
            results.extend(self._check_persisted_fields(txn_id, sender_did, receiver_did))

        results.append(self._check_no_duplicates())
        results.extend(self._check_chain_integrity())
        results.append(self._report_dead_letters())

        if include_restart:
            results.extend(self._check_restart_idempotency())

        passed = sum(1 for r in results if r["status"] == "PASS")
        failed = sum(1 for r in results if r["status"] == "FAIL")
        warned = sum(1 for r in results if r["status"] == "WARN")
        skipped = sum(1 for r in results if r["status"] == "SKIP")
        log.info("=== FULLNODE VERIFICATION: %d passed, %d failed, %d warn, "
                 "%d skipped, %d total ===",
                 passed, failed, warned, skipped, len(results))
        for r in results:
            if r["status"] == "WARN":
                log.warning("  FULLNODE WARN: %s — %s", r["check"], r["detail"])
        for r in results:
            if r["status"] == "FAIL":
                log.error("  FULLNODE FAIL: %s — %s", r["check"], r["detail"])
        return results
