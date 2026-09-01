"""
db_validator.py — Per-node PostgreSQL validation queries for the Rubix E2E test runner.

Checks: token duplicates, token sum, stuck-pledged tokens, orphan unpledge records,
and per-DID balance.  Aggregates results into PASS / WARN / FAIL via is_clean().
"""

import logging
import time
from collections import Counter
from typing import Any, Dict, List, Optional, Tuple

import psycopg2

log = logging.getLogger(__name__)


class DBValidator:
    """Runs integrity checks against a single Rubix node's PostgreSQL database.

    Args:
        host:     Postgres hostname (default: localhost).
        port:     Host-mapped Postgres port (5433, 5434, or 5435 for nodeA/B/quorum).
        dbname:   Database name (rubix_a, rubix_b, rubix_q).
        user:     Postgres user (default: rubix).
        password: Postgres password (default: rubixpass).
    """

    def __init__(
        self,
        host: str = "localhost",
        port: int = 5433,
        dbname: str = "rubix_a",
        user: str = "rubix",
        password: str = "rubixpass",
    ) -> None:
        self.conn_params: Dict[str, Any] = {
            "host": host,
            "port": port,
            "dbname": dbname,
            "user": user,
            "password": password,
        }
        self.name = f"{dbname}@{host}:{port}"

    # ------------------------------------------------------------------
    # Internal helper
    # ------------------------------------------------------------------

    def _connect(self) -> "psycopg2.connection":
        """Open and return a new psycopg2 connection."""
        return psycopg2.connect(**self.conn_params)

    # ------------------------------------------------------------------
    # Individual checks
    # ------------------------------------------------------------------

    def check_duplicates(self) -> List[Tuple[str, int]]:
        """Return token_ids that appear more than once in the tokens table.

        Empty list means no duplicates (clean).
        """
        sql = """
            SELECT token_id, COUNT(*) AS cnt
            FROM tokens
            GROUP BY token_id
            HAVING COUNT(*) > 1
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql)
            rows = cur.fetchall()
        if rows:
            log.warning("[%s] Duplicate token_ids found: %s", self.name, rows)
        return rows  # type: ignore[return-value]

    def get_token_sum(self) -> float:
        """Return the total sum of token_value across FREE tokens only (token_status = 0)."""
        sql = "SELECT COALESCE(SUM(token_value), 0) FROM tokens WHERE token_status = 0"
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql)
            result = cur.fetchone()[0]
        return float(result)

    def check_invalid_pledged_tokens(self) -> List[Tuple[str, int]]:
        """Return tokens stuck in pledged status (6 or 7) with no matching
        unpledge_sequence_info record.

        These represent tokens that should have been unpledged after consensus
        but weren't.  Indicates a bug in the quorum callback / unpledge flow.
        """
        sql = """
            SELECT t.token_id, t.token_status
            FROM tokens t
            WHERE t.token_status IN (6, 7)
              AND NOT EXISTS (
                  SELECT 1
                  FROM unpledge_sequence_info u
                  WHERE t.token_id = ANY(u.pledge_tokens)
              )
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql)
            rows = cur.fetchall()
        if rows:
            log.warning("[%s] Invalid pledged tokens (no unpledge record): %s", self.name, rows)
        return rows  # type: ignore[return-value]

    def check_orphan_unpledge_records(self) -> List[str]:
        """Return tx_ids in unpledge_sequence_info that have no matching transactions row.

        Orphan unpledge records indicate the consensus side wrote a record for a
        transaction that was never finalised.
        """
        sql = """
            SELECT u.tx_id
            FROM unpledge_sequence_info u
            WHERE NOT EXISTS (
                SELECT 1
                FROM transactions t
                WHERE t.id = u.tx_id
            )
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql)
            rows = cur.fetchall()
        result = [row[0] for row in rows]
        if result:
            log.warning("[%s] Orphan unpledge records: %s", self.name, result)
        return result

    def check_tokenchain_gaps(self) -> List[str]:
        """Return token_ids in tokenchain with position sequence gaps.

        A clean tokenchain has MAX(position) == COUNT(*) - 1 for every token_id
        (0-indexed chains: positions [0,1,2] have MAX=2 and COUNT=3).
        Empty list means no gaps.  Returns [] with a warning if the table is absent.
        """
        sql = """
            SELECT token_id
            FROM tokenchain
            GROUP BY token_id
            HAVING MAX(position) != COUNT(*) - 1
        """
        try:
            with self._connect() as conn, conn.cursor() as cur:
                cur.execute(sql)
                rows = cur.fetchall()
            result = [row[0] for row in rows]
            if result:
                log.warning("[%s] Tokenchain gaps for token_ids: %s", self.name, result)
            return result
        except Exception as exc:
            # Table may not exist on nodes that own no tokens
            log.warning("[%s] check_tokenchain_gaps skipped: %s", self.name, exc)
            return []

    def wait_for_unpledge_clearance(self, tx_id: str, timeout: int = 30) -> bool:
        """Poll until tx_id is removed from unpledge_sequence_info (max *timeout* seconds).

        Per-tx_id check: returns True when the row is gone, False on timeout.
        Opens a fresh connection each poll to avoid stale reads.
        Called on quorum DBValidator only.
        """
        sql = "SELECT COUNT(*) FROM unpledge_sequence_info WHERE tx_id = %s"
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                with self._connect() as conn, conn.cursor() as cur:
                    cur.execute(sql, (tx_id,))
                    count = cur.fetchone()[0]
                if count == 0:
                    log.info("[%s] tx_id %s cleared from unpledge_sequence_info.", self.name, tx_id)
                    return True
            except Exception as exc:  # noqa: BLE001
                log.warning("[%s] wait_for_unpledge_clearance poll error: %s", self.name, exc)
            time.sleep(1)

        # Timeout — log remaining records for debugging
        try:
            with self._connect() as conn, conn.cursor() as cur:
                cur.execute("SELECT tx_id FROM unpledge_sequence_info")
                remaining = [row[0] for row in cur.fetchall()]
            log.warning(
                "[%s] TIMEOUT: tx_id %s still in unpledge_sequence_info after %ds. "
                "Remaining tx_ids: %s",
                self.name, tx_id, timeout, remaining,
            )
        except Exception as exc:  # noqa: BLE001
            log.warning("[%s] Could not fetch remaining unpledge_sequence_info: %s", self.name, exc)
        return False

    def wait_for_unpledge_table_empty(self, timeout: int = 30) -> bool:
        """Poll until unpledge_sequence_info has zero rows (max *timeout* seconds).

        Table-level check: returns True when empty, False on timeout.
        Opens a fresh connection each poll to avoid stale reads.
        Called on quorum DBValidator only.
        """
        sql = "SELECT COUNT(*) FROM unpledge_sequence_info"
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                with self._connect() as conn, conn.cursor() as cur:
                    cur.execute(sql)
                    count = cur.fetchone()[0]
                if count == 0:
                    log.info("[%s] unpledge_sequence_info table is empty.", self.name)
                    return True
            except Exception as exc:
                log.warning("[%s] wait_for_unpledge_table_empty poll error: %s", self.name, exc)
            time.sleep(1)

        # Timeout — log remaining tx_ids for debugging
        try:
            with self._connect() as conn, conn.cursor() as cur:
                cur.execute("SELECT tx_id FROM unpledge_sequence_info")
                remaining = [row[0] for row in cur.fetchall()]
            log.warning(
                "[%s] TIMEOUT: unpledge_sequence_info not empty after %ds. "
                "Remaining tx_ids: %s",
                self.name, timeout, remaining,
            )
        except Exception as exc:
            log.warning("[%s] Could not fetch remaining unpledge_sequence_info: %s", self.name, exc)
        return False

    def get_did_balance(self, did: str) -> float:
        """Return the sum of token_value for *did* where token_status = 0 (FREE)."""
        sql = """
            SELECT COALESCE(SUM(token_value), 0)
            FROM tokens
            WHERE did = %s
              AND token_status = 0
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql, (did,))
            result = cur.fetchone()[0]
        return float(result)

    def get_free_token_ids(self) -> List[str]:
        """Return token_ids with token_status = 0 (FREE) on this node."""
        sql = "SELECT token_id FROM tokens WHERE token_status = 0"
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql)
            rows = cur.fetchall()
        return [row[0] for row in rows]

    def get_did_ft_token_count(self, did: str, free_only: bool = True) -> int:
        """Return the number of FT tokens owned by *did* in the tokens table.

        FT tokens live in the tokens table with token_type joined to
        token_type.name = 'ft'. This reflects COMMITTED ownership immediately
        after consensus, unlike the balance API view which can lag by minutes
        for same-node (intra-node) FT settlement.

        Args:
            did:       Owner DID.
            free_only: If True (default), count only FREE tokens (token_status = 0).
        """
        sql = """
            SELECT COUNT(*)
            FROM tokens
            WHERE did = %s
              AND token_type = (SELECT id FROM token_type WHERE name = 'ft')
        """
        if free_only:
            sql += " AND token_status = 0"
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql, (did,))
            return int(cur.fetchone()[0])

    def transaction_exists(self, txn_id: str) -> bool:
        """Return True if a row with id = *txn_id* exists in the transactions table."""
        sql = "SELECT 1 FROM transactions WHERE id = %s LIMIT 1"
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql, (txn_id,))
            return cur.fetchone() is not None

    def get_transaction_participants(self, txn_id: str) -> Optional[Tuple[str, str]]:
        """Return (initiator_did, owner_did) for *txn_id*, or None if the row is absent.

        Reads the two participant DIDs out of the transaction's ``info`` JSONB.
        The same transaction id is persisted on every participating node, so the
        row found on any one node is authoritative for "who took part".
        """
        sql = (
            "SELECT info->>'initiator', info->>'owner' "
            "FROM transactions WHERE id = %s LIMIT 1"
        )
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql, (txn_id,))
            row = cur.fetchone()
        if row is None:
            return None
        return (row[0], row[1])

    # ------------------------------------------------------------------
    # Aggregate helpers
    # ------------------------------------------------------------------

    def run_all_checks(self, did: Optional[str] = None) -> Dict[str, Any]:
        """Run all checks and return a consolidated dict.

        Args:
            did: If provided, also run get_did_balance for this DID.

        Returns:
            {
                "duplicates":       [(token_id, count), …],
                "token_sum":        <float>,
                "invalid_pledged":  [(token_id, status), …],
                "orphan_unpledge":  [tx_id, …],
                "did_balance":      <float> | None,
            }
        """
        return {
            "duplicates": self.check_duplicates(),
            "token_sum": self.get_token_sum(),
            "invalid_pledged": self.check_invalid_pledged_tokens(),
            "orphan_unpledge": self.check_orphan_unpledge_records(),
            "did_balance": self.get_did_balance(did) if did else None,
        }

    def is_clean(
        self, checks: Optional[Dict[str, Any]] = None
    ) -> Tuple[str, Optional[str]]:
        """Evaluate check results and return (status, issues_string).

        If *checks* is None, run_all_checks() is called first.

        Status precedence: FAIL > WARN > PASS.
        Multiple issues are joined by "; ".

        Returns:
            ("PASS", None)
            ("WARN", "reason; reason")
            ("FAIL", "reason; reason")
        """
        if checks is None:
            checks = self.run_all_checks()

        issues: List[Tuple[str, str]] = []  # (severity, description)

        if checks.get("duplicates"):
            issues.append(("FAIL", "Duplicate token_ids found"))

        if checks.get("invalid_pledged"):
            issues.append(("WARN", "Tokens in pledged status without unpledge record"))

        if checks.get("orphan_unpledge"):
            issues.append(("WARN", "Orphan unpledge records found"))

        if not issues:
            return ("PASS", None)

        # Worst status wins
        statuses = [s for s, _ in issues]
        overall = "FAIL" if "FAIL" in statuses else "WARN"
        combined = "; ".join(desc for _, desc in issues)
        return (overall, combined)


# ---------------------------------------------------------------------------
# System-level invariant validation (cross-node)
# ---------------------------------------------------------------------------


def validate_system_invariants(
    db_a: "DBValidator",
    db_b: "DBValidator",
    db_q: "DBValidator",
    initial_total: float,
) -> Dict[str, Any]:
    """Run all 5 system invariants after a test case.

    Node scope per invariant:
      1. no_stuck_pledged   — all 3 nodes
      2. no_unpledge_backlog — quorum (db_q) table-level empty check
      3. token_conservation  — cross-node FREE supply vs initial_total
      4. single_free_owner   — cross-node: each token_id FREE on at most 1 node
      5. tokenchain_valid    — all 3 nodes

    Args:
        db_a:          DBValidator for nodeA.
        db_b:          DBValidator for nodeB.
        db_q:          DBValidator for quorum (also used for Invariant 2).
        initial_total: Total FREE token supply measured after setup (sum of all 3 nodes).

    Returns:
        {
            "status": "PASS" | "FAIL",
            "invariants": {
                "no_stuck_pledged": bool,
                "no_unpledge_backlog": bool,
                "token_conservation": bool,
                "single_free_owner": bool,
                "tokenchain_valid": bool,
            },
            "details": {
                # Present only for failing invariants; exact DB output
                "stuck_pledged_a": [...],
                "stuck_pledged_b": [...],
                "stuck_pledged_q": [...],
                "unpledge_backlog": str,
                "current_total": float,
                "expected_total": float,
                "sum_a": float,
                "sum_b": float,
                "sum_q": float,
                "duplicates_a": [...],
                "duplicates_b": [...],
                "duplicates_q": [...],
                "cross_free_violations": {token_id: count},
                "tokenchain_gaps_a": [...],
                "tokenchain_gaps_b": [...],
                "tokenchain_gaps_q": [...],
            },
        }
    """
    details: Dict[str, Any] = {}

    # Invariant 1: no stuck pledged tokens (status 6 or 7 with no unpledge record)
    stuck_a = db_a.check_invalid_pledged_tokens()
    stuck_b = db_b.check_invalid_pledged_tokens()
    stuck_q = db_q.check_invalid_pledged_tokens()
    inv1_pass = not stuck_a and not stuck_b and not stuck_q
    if not inv1_pass:
        if stuck_a:
            details["stuck_pledged_a"] = stuck_a
        if stuck_b:
            details["stuck_pledged_b"] = stuck_b
        if stuck_q:
            details["stuck_pledged_q"] = stuck_q

    # Invariant 2: unpledge table must be empty (quorum node)
    inv2_pass = db_q.wait_for_unpledge_table_empty()
    if not inv2_pass:
        details["unpledge_backlog"] = "unpledge_sequence_info not empty after timeout"

    # Invariant 3: cross-node token conservation
    sum_a = db_a.get_token_sum()
    sum_b = db_b.get_token_sum()
    sum_q = db_q.get_token_sum()
    current_total = sum_a + sum_b + sum_q
    inv3_pass = abs(current_total - initial_total) < 0.001
    if not inv3_pass:
        details["current_total"] = current_total
        details["expected_total"] = initial_total
        details["sum_a"] = sum_a
        details["sum_b"] = sum_b
        details["sum_q"] = sum_q
        log.warning(
            "Token conservation FAILED: expected=%.4f actual=%.4f "
            "(a=%.4f b=%.4f q=%.4f)",
            initial_total, current_total, sum_a, sum_b, sum_q,
        )

    # Invariant 4a: per-node duplicate token_ids (always a bug within same DB)
    dup_a = db_a.check_duplicates()
    dup_b = db_b.check_duplicates()
    dup_q = db_q.check_duplicates()
    per_node_dup_pass = not dup_a and not dup_b and not dup_q
    if not per_node_dup_pass:
        if dup_a:
            details["duplicates_a"] = dup_a
        if dup_b:
            details["duplicates_b"] = dup_b
        if dup_q:
            details["duplicates_q"] = dup_q

    # Invariant 4b: cross-node single FREE owner per token_id
    free_a = set(db_a.get_free_token_ids())
    free_b = set(db_b.get_free_token_ids())
    free_q = set(db_q.get_free_token_ids())
    cross_free_violations: Dict[str, int] = {}
    for tid in free_a | free_b | free_q:
        count = (tid in free_a) + (tid in free_b) + (tid in free_q)
        if count > 1:
            cross_free_violations[tid] = count
    cross_free_pass = len(cross_free_violations) == 0

    inv4_pass = per_node_dup_pass and cross_free_pass
    if cross_free_violations:
        details["cross_free_violations"] = cross_free_violations
        # Enhanced failure logging: show token status per node for violators
        for tid in cross_free_violations:
            node_status = {}
            for label, db in [("a", db_a), ("b", db_b), ("q", db_q)]:
                try:
                    with db._connect() as conn, conn.cursor() as cur:
                        cur.execute("SELECT token_status, did FROM tokens WHERE token_id = %s", (tid,))
                        row = cur.fetchone()
                        node_status[label] = {"status": row[0], "did": row[1]} if row else None
                except Exception:
                    node_status[label] = "error"
            log.warning("FREE on >1 node: token_id=%s  owners=%s", tid, node_status)

    # Invariant 5: tokenchain integrity (no position gaps)
    gaps_a = db_a.check_tokenchain_gaps()
    gaps_b = db_b.check_tokenchain_gaps()
    gaps_q = db_q.check_tokenchain_gaps()
    inv5_pass = not gaps_a and not gaps_b and not gaps_q
    if not inv5_pass:
        if gaps_a:
            details["tokenchain_gaps_a"] = gaps_a
        if gaps_b:
            details["tokenchain_gaps_b"] = gaps_b
        if gaps_q:
            details["tokenchain_gaps_q"] = gaps_q
        # Log position details for gap tokens
        for label, db, gaps in [("a", db_a, gaps_a), ("b", db_b, gaps_b), ("q", db_q, gaps_q)]:
            if gaps:
                try:
                    with db._connect() as conn, conn.cursor() as cur:
                        cur.execute(
                            "SELECT token_id, position FROM tokenchain "
                            "WHERE token_id = ANY(%s) ORDER BY token_id, position",
                            (gaps,)
                        )
                        positions = cur.fetchall()
                    log.warning("Tokenchain gaps on node_%s: %s", label, positions)
                except Exception as exc:
                    log.warning("Could not fetch tokenchain positions for node_%s: %s", label, exc)

    inv_dict = {
        "no_stuck_pledged": inv1_pass,
        "no_unpledge_backlog": inv2_pass,
        "token_conservation": inv3_pass,
        "single_free_owner": inv4_pass,
        "tokenchain_valid": inv5_pass,
    }
    overall = "PASS" if all(inv_dict.values()) else "FAIL"

    if overall == "FAIL":
        log.warning("Invariant validation FAILED: %s  details=%s", inv_dict, details)
    else:
        log.info("Invariant validation PASSED.")

    return {"status": overall, "invariants": inv_dict, "details": details}


# ---------------------------------------------------------------------------
# Transaction persistence validation (cross-node)
# ---------------------------------------------------------------------------


def check_transactions_persisted(
    records: List[Dict[str, Any]],
    node_dbs: Dict[str, "DBValidator"],
    did_to_node: Dict[str, str],
) -> Dict[str, Any]:
    """Assert every recorded-SUCCESS transaction is persisted on every node that took part.

    For each record with status == "SUCCESS" and a non-null ``transaction_id``,
    the on-chain id is looked up in the ``transactions`` table across all node
    DBs. The row's own ``info`` (initiator + owner DIDs) is the authoritative
    source of which nodes MUST hold it: a cross-node transfer requires the row on
    both the sender's and the receiver's node, while a single-node operation
    (mint / deploy / self-execute) requires it on just the one node. This avoids
    guessing direction from heterogeneous per-engine record fields.

    A transaction is a FAILURE when:
      - its id is not found on ANY node DB (never persisted at all), or
      - it is found, but is MISSING on a participant node (initiator or owner).

    Records whose participant DID maps to no known node (e.g. a brand-new
    intra-node secondary DID not provided in *did_to_node*) fall back to
    "must exist on the recording node only", using the record's ``node``/``from``
    hint; if no hint is available the record is reported as a skip, never a FAIL.

    Args:
        records:     The reporter's recorded transactions.
        node_dbs:    {"A": db_a, "B": db_b, "quorum": db_q} — node label -> DBValidator.
        did_to_node: full DID -> node label ("A"/"B"/"quorum"). Secondary DIDs
                     (e.g. did_a2) should map to their hosting node ("A").

    Returns:
        {
            "status": "PASS" | "FAIL",
            "checked": <int>,            # SUCCESS txns with a transaction_id
            "skipped": <int>,            # SUCCESS txns without a usable txn id
            "missing": [                 # one entry per failure
                {"transaction_id": str, "id": str, "type": str,
                 "not_found_anywhere": bool, "missing_on": [node_label, ...]},
                ...
            ],
        }
    """
    missing: List[Dict[str, Any]] = []
    checked = 0
    skipped = 0

    def _node_hint(rec: Dict[str, Any]) -> Optional[str]:
        # Engines variously record the acting node as "node" or "from".
        hint = rec.get("node") or rec.get("from")
        if isinstance(hint, str) and hint in node_dbs:
            return hint
        return None

    def _verify_one(txn_id: str, rec: Dict[str, Any]) -> str:
        """Verify a single on-chain txn id. Returns "checked" | "skipped".

        Appends to the enclosing *missing* list on failure.
        """
        # 1. Find the row on any node; read its true participants.
        participants: Optional[Tuple[str, str]] = None
        for db in node_dbs.values():
            try:
                participants = db.get_transaction_participants(txn_id)
            except Exception as exc:  # noqa: BLE001
                log.warning("tx-persist lookup error for %s on %s: %s",
                            txn_id, getattr(db, "name", "?"), exc)
                participants = None
            if participants is not None:
                break

        if participants is None:
            # Not present on ANY node — the worst failure.
            missing.append({
                "transaction_id": txn_id,
                "id": rec.get("id"),
                "type": rec.get("type", "RBT_TRANSFER"),
                "not_found_anywhere": True,
                "missing_on": sorted(node_dbs.keys()),
            })
            return "checked"

        # 2. Determine required nodes from the row's own DIDs.
        initiator, owner = participants
        required: set = set()
        for did in (initiator, owner):
            if did and did in did_to_node:
                required.add(did_to_node[did])
        if not required:
            # DID(s) not in the map (e.g. unprovided secondary DID): fall back to
            # the recording node hint so we still assert at-least-one persistence.
            hint = _node_hint(rec)
            if hint:
                required.add(hint)
        if not required:
            # No way to attribute this txn to a node — don't manufacture a FAIL.
            return "skipped"

        # 3. Assert the row exists on every required node.
        missing_on: List[str] = []
        for node_label in sorted(required):
            db = node_dbs.get(node_label)
            if db is None:
                continue
            try:
                if not db.transaction_exists(txn_id):
                    missing_on.append(node_label)
            except Exception as exc:  # noqa: BLE001
                log.warning("tx-persist exists-check error for %s on %s: %s",
                            txn_id, node_label, exc)
                missing_on.append(node_label)

        if missing_on:
            missing.append({
                "transaction_id": txn_id,
                "id": rec.get("id"),
                "type": rec.get("type", "RBT_TRANSFER"),
                "not_found_anywhere": False,
                "missing_on": missing_on,
            })
        return "checked"

    for rec in records:
        if rec.get("status") != "SUCCESS":
            continue

        # A single record may carry more than one on-chain txn (e.g. intra-node
        # NFT/SC phases fire a deploy AND an execute). Verify each id present.
        txn_ids = [
            rec.get(key) for key in ("transaction_id", "deploy_transaction_id")
            if rec.get(key)
        ]
        if not txn_ids:
            # SUCCESS without any on-chain id (e.g. a DID setup, or an op that
            # returns no transactionID) — nothing to assert.
            skipped += 1
            continue

        for txn_id in txn_ids:
            outcome = _verify_one(txn_id, rec)
            if outcome == "checked":
                checked += 1
            else:
                skipped += 1

    status = "FAIL" if missing else "PASS"
    if status == "FAIL":
        log.warning(
            "Transaction-persistence check FAILED: %d/%d txns missing. %s",
            len(missing), checked, missing,
        )
    else:
        log.info(
            "Transaction-persistence check PASSED: %d txns present on all "
            "participant nodes (%d skipped).", checked, skipped,
        )

    return {"status": status, "checked": checked, "skipped": skipped, "missing": missing}


# ---------------------------------------------------------------------------
# Fullnode (-fullnode node) table queries
#
# A node started with `-fullnode` subscribes to the rubix_txn pubsub topic,
# validates every published transaction (core/consensus/checks.go
# ValidateTransaction) and persists the result into a SEPARATE set of tables
# from its own wallet: fullnode_transactions / fullnode_rbt / fullnode_ft /
# fullnode_nft / fullnode_smart_contract / fullnode_tokenchain /
# fullnode_tokenchain_index, with validation failures dead-lettered into
# fullnode_invalid_transactions (see core/wallet/fullnode_persistence.go).
#
# There is no HTTP API over these tables — the only fullnode-facing routes are
# libp2p (/rubix/v1/fullnode/sync|recover) — so direct SQL is the intended way
# to observe fullnode behaviour from a test.
# ---------------------------------------------------------------------------


class FullnodeDBValidator(DBValidator):
    """Read-side queries against a `-fullnode` node's fullnode_* tables."""

    # Tables whose row counts are snapshotted for the restart/idempotency check.
    COUNTED_TABLES = (
        "fullnode_transactions",
        "fullnode_rbt",
        "fullnode_tokenchain",
        "fullnode_tokenchain_index",
        "fullnode_invalid_transactions",
    )

    # ---- basic reachability ------------------------------------------- #

    def wait_reachable(self, timeout: int = 120) -> bool:
        """Poll until the fullnode's Postgres accepts a connection AND the node
        has created its schema (fullnode_transactions exists).

        Schema creation happens on node startup, so this doubles as "the node
        process got far enough to migrate", which is a cheaper and earlier
        signal than waiting for a transaction to arrive.
        """
        deadline = time.time() + timeout
        last_exc: Optional[Exception] = None
        while time.time() < deadline:
            try:
                with self._connect() as conn, conn.cursor() as cur:
                    cur.execute("SELECT to_regclass('public.fullnode_transactions')")
                    if cur.fetchone()[0] is not None:
                        return True
            except Exception as exc:  # noqa: BLE001
                last_exc = exc
            time.sleep(2.0)
        log.error("[%s] fullnode schema not reachable within %ds (last error: %s)",
                  self.name, timeout, last_exc)
        return False

    # ---- transaction receipt ------------------------------------------ #

    def fullnode_transaction_exists(self, txn_id: str) -> bool:
        sql = "SELECT 1 FROM fullnode_transactions WHERE id = %s LIMIT 1"
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql, (txn_id,))
            return cur.fetchone() is not None

    def wait_for_fullnode_transaction(
        self, txn_id: str, timeout: int = 180, poll: float = 2.0
    ) -> bool:
        """Poll fullnode_transactions until *txn_id* lands, or *timeout* elapses.

        This is the end-to-end gate for `localnet transaction -> PubSub ->
        fullnode -> validation -> persistence`: the row is only written after
        ValidateTransaction returns nil (core/fullnode.go
        processSingleTransaction), so its presence proves the whole path.
        """
        deadline = time.time() + timeout
        attempts = 0
        while time.time() < deadline:
            attempts += 1
            try:
                if self.fullnode_transaction_exists(txn_id):
                    log.info("[%s] fullnode persisted txn %s after %d poll(s).",
                             self.name, txn_id, attempts)
                    return True
            except Exception as exc:  # noqa: BLE001
                log.warning("[%s] fullnode txn poll error: %s", self.name, exc)
            time.sleep(poll)
        log.error("[%s] fullnode did NOT persist txn %s within %ds (%d polls).",
                  self.name, txn_id, timeout, attempts)
        return False

    def get_fullnode_transaction(self, txn_id: str) -> Optional[Dict[str, Any]]:
        """Return {"id", "info", "signature"} for a persisted fullnode txn.

        `info` and `signature` come back as already-parsed Python objects
        (the columns are JSON / JSONB).
        """
        sql = "SELECT id, info, signature FROM fullnode_transactions WHERE id = %s"
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql, (txn_id,))
            row = cur.fetchone()
        if row is None:
            return None
        return {"id": row[0], "info": row[1], "signature": row[2]}

    # ---- persisted token state ---------------------------------------- #

    def get_fullnode_rbt_tokens(self, token_ids: List[str]) -> Dict[str, Dict[str, Any]]:
        """Return {token_id: {did, transaction_id, token_status, latest_position,
        latest_role, latest_role_name}} for the given RBT tokens."""
        if not token_ids:
            return {}
        sql = """
            SELECT t.token_id, t.did, t.transaction_id, t.token_status,
                   t.latest_position, t.latest_role, r.name
            FROM fullnode_rbt t
            LEFT JOIN token_role r ON r.id = t.latest_role
            WHERE t.token_id = ANY(%s)
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql, (list(token_ids),))
            rows = cur.fetchall()
        return {
            row[0]: {
                "did": row[1],
                "transaction_id": row[2],
                "token_status": row[3],
                "latest_position": row[4],
                "latest_role": row[5],
                "latest_role_name": row[6],
            }
            for row in rows
        }

    def get_fullnode_chain_entries(self, txn_id: str) -> List[Dict[str, Any]]:
        """Return every fullnode_tokenchain row written for *txn_id*, with the
        role resolved to its name via token_role."""
        sql = """
            SELECT tc.token_id, tc.transaction_id, tc.previous_transaction_id,
                   tc.position, tc.role, r.name
            FROM fullnode_tokenchain tc
            LEFT JOIN token_role r ON r.id = tc.role
            WHERE tc.transaction_id = %s
            ORDER BY tc.token_id, tc.position
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql, (txn_id,))
            rows = cur.fetchall()
        return [
            {
                "token_id": row[0],
                "transaction_id": row[1],
                "previous_transaction_id": row[2],
                "position": row[3],
                "role": row[4],
                "role_name": row[5],
            }
            for row in rows
        ]

    # ---- integrity checks ---------------------------------------------- #

    def fullnode_duplicate_chain_entries(self) -> List[Tuple[str, str, int]]:
        """Return (token_id, transaction_id, count) appearing more than once.

        Unlike `tokenchain`, `fullnode_tokenchain` has NO UNIQUE(token_id,
        transaction_id) constraint (core/storage/schema.go) — nothing at the DB
        layer stops the same chain entry being ingested twice. This is the
        query that catches it.
        """
        sql = """
            SELECT token_id, transaction_id, COUNT(*) AS cnt
            FROM fullnode_tokenchain
            GROUP BY token_id, transaction_id
            HAVING COUNT(*) > 1
            ORDER BY cnt DESC
            LIMIT 50
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql)
            return cur.fetchall()  # type: ignore[return-value]

    def fullnode_chain_position_gaps(self) -> List[Tuple[str, int, int]]:
        """Return (token_id, max_position, row_count) for chains that are not a
        contiguous 0..N-1 run."""
        sql = """
            SELECT token_id, MAX(position), COUNT(*)
            FROM fullnode_tokenchain
            GROUP BY token_id
            HAVING MAX(position) <> COUNT(*) - 1 OR MIN(position) <> 0
            LIMIT 50
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql)
            return cur.fetchall()  # type: ignore[return-value]

    def fullnode_chain_link_breaks(self) -> List[Tuple[str, int, str, str]]:
        """Return chain rows whose previous_transaction_id does not equal the
        transaction_id of the row at position-1 (or that violate the
        position 0 <=> empty-prev rule).

        Returns (token_id, position, previous_transaction_id, expected).
        """
        sql = """
            WITH linked AS (
                SELECT token_id, position, transaction_id,
                       COALESCE(previous_transaction_id, '') AS prev,
                       LAG(transaction_id) OVER (
                           PARTITION BY token_id ORDER BY position
                       ) AS expected_prev
                FROM fullnode_tokenchain
            )
            SELECT token_id, position, prev, COALESCE(expected_prev, '')
            FROM linked
            WHERE (position = 0 AND prev <> '')
               OR (position > 0 AND prev <> COALESCE(expected_prev, ''))
            LIMIT 50
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql)
            return cur.fetchall()  # type: ignore[return-value]

    def fullnode_dangling_chain_refs(self) -> List[Tuple[str, int, str, str]]:
        """Return chain rows pointing at a transaction_id / previous_transaction_id
        that is absent from fullnode_transactions.

        Both FKs are DEFERRABLE INITIALLY DEFERRED, so a bad write inside a
        transaction is only caught at COMMIT — this asserts the deferral was
        never turned into a lie.
        """
        sql = """
            SELECT tc.token_id, tc.position, tc.transaction_id,
                   COALESCE(tc.previous_transaction_id, '')
            FROM fullnode_tokenchain tc
            WHERE NOT EXISTS (
                      SELECT 1 FROM fullnode_transactions t WHERE t.id = tc.transaction_id
                  )
               OR (tc.previous_transaction_id IS NOT NULL
                   AND tc.previous_transaction_id <> ''
                   AND NOT EXISTS (
                       SELECT 1 FROM fullnode_transactions t
                       WHERE t.id = tc.previous_transaction_id
                   ))
            LIMIT 50
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql)
            return cur.fetchall()  # type: ignore[return-value]

    def fullnode_pledge_rows_without_prev(self) -> List[Tuple[str, str]]:
        """Return (token_id, transaction_id) pledge-role chain rows that have an
        empty previous_transaction_id.

        A pledge can never be a token's genesis entry — the quorum must already
        have owned the token — so a pledge row at position 0 is a real defect.
        """
        sql = """
            SELECT tc.token_id, tc.transaction_id
            FROM fullnode_tokenchain tc
            JOIN token_role r ON r.id = tc.role
            WHERE r.name = 'pledge'
              AND COALESCE(tc.previous_transaction_id, '') = ''
            LIMIT 50
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql)
            return cur.fetchall()  # type: ignore[return-value]

    def get_fullnode_invalid_transactions(self, limit: int = 20) -> List[Tuple[str, str]]:
        """Return (reason, created_at) rows dead-lettered by the fullnode.

        core/fullnode.go processTxnWithRetry writes here only after ALL retries
        are exhausted on a validation failure, so any row means the fullnode
        definitively rejected a transaction the network accepted.
        """
        sql = """
            SELECT reason, created_at::text
            FROM fullnode_invalid_transactions
            ORDER BY created_at DESC
            LIMIT %s
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql, (limit,))
            return cur.fetchall()  # type: ignore[return-value]

    def fullnode_invalid_reason_histogram(self) -> List[Tuple[str, int]]:
        """Return (reason-class, count) for dead-lettered transactions.

        Reasons are normalised before grouping: DIDs, token IDs and transaction
        hashes are replaced with `<id>`, so 30 rejections sharing one root cause
        report as a single line with count=30 instead of 30 near-identical
        strings that differ only by identifier. That makes the histogram a
        cause histogram, which is what a CI reader needs.
        """
        sql = """
            SELECT regexp_replace(
                       left(reason, 240),
                       '(bafybmi[a-z0-9]+|[A-Za-z0-9]+_[0-9]+|[0-9a-f]{32,})',
                       '<id>', 'g'
                   ) AS reason_class,
                   COUNT(*) AS cnt
            FROM fullnode_invalid_transactions
            GROUP BY reason_class
            ORDER BY cnt DESC
            LIMIT 20
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql)
            return cur.fetchall()  # type: ignore[return-value]

    def fullnode_transaction_rejected(self, txn_id: str) -> Optional[str]:
        """Return the rejection reason if *txn_id* was dead-lettered, else None.

        The `transaction` column holds the marshalled models.Transactions
        struct. That struct carries only `db:` tags, so encoding/json falls back
        to Go FIELD NAMES — the id is at `transaction->>'ID'`, not `->>'id'`.
        """
        sql = """
            SELECT reason
            FROM fullnode_invalid_transactions
            WHERE transaction->>'ID' = %s
            ORDER BY created_at DESC
            LIMIT 1
        """
        with self._connect() as conn, conn.cursor() as cur:
            cur.execute(sql, (txn_id,))
            row = cur.fetchone()
        return row[0] if row else None

    # ---- snapshots ------------------------------------------------------ #

    def fullnode_row_counts(self) -> Dict[str, int]:
        """Row count per fullnode table — the before/after snapshot for the
        restart-idempotency check."""
        counts: Dict[str, int] = {}
        with self._connect() as conn, conn.cursor() as cur:
            for table in self.COUNTED_TABLES:
                cur.execute(f"SELECT COUNT(*) FROM {table}")  # noqa: S608 (fixed list)
                counts[table] = int(cur.fetchone()[0])
        return counts
