"""
ft_parts.py — FT creation out of PART RBTs (fractional denominations).

Minting an FT burns whole RBT: ``createFTs`` locks ``token_count`` RBT worth of
tokens, ``parts.CollectRBTTokens`` assembles them, ``BatchRBTs`` groups them
into batches summing to exactly 1.0, and ``FTGenesisTxn`` flips each parent RBT
to BurntForFT via ``UpdateTokenInfo``.

Nothing says those parents have to be WHOLE tokens. A wallet holding only
fractional RBT (0.5 / 0.1 / 0.05 …) must be able to mint just the same, with
several part tokens burnt per FT batch. That is the path this suite drives, and
it is the path where the accounting has been wrong:

  - ``UpdateTokenInfo`` INCREMENTED ``token_denom`` for every RBT it burnt, when
    burning moves a token OUT of Free and must DECREMENT it.
  - It keyed that write on ``token.TokenValue`` while never copying the value
    from ``tokenInfo`` — so the write landed on denom 0, adding a phantom
    ``denom=0`` row AND missing the real denomination entirely.

Both are fixed in ``core/wallet/token_chain.go`` (``UpdateTokenInfo`` now
decrements, floors at 0, and skips ``token_value = 0``). The damage they caused
is invisible to the existing FT suite: it asserts FT counts and balances, never
``token_denom``, and it mints from a wallet full of whole tokens where a single
1.000 parent is burnt per batch — one wrong counter row that the wallet's
hundreds of other whole tokens hide. Burning PARTS makes it loud: the parts
wallet here holds a handful of tokens at two denominations, so a counter that
still advertises the burnt ones is both measurable and fatal to the next spend.

Why token_denom matters at all: it is not a statistic. ``lockTokensForSplitOnce``
picks WHICH denominations to select from ``token_denom`` and only then reads the
matching rows out of ``tokens``. When the counter over-reports, selection asks
for tokens that are no longer Free, gets fewer rows back than it planned for (or
none), and the operation dies with ``lockSelectedTokens: no tokens provided`` —
attributed to whatever transaction happened to run next, not to the FT mint that
corrupted the counter. FTPARTS_SPEND_AFTER_MINT reproduces exactly that.

Isolation: the suite creates its OWN DID on node A and funds it only with
sub-1.0 transfers, so by construction the wallet holds parts and no whole token.
That gives an exact expected state for every assertion — impossible against
``did_a``, whose balance every other phase is concurrently spending, and where a
1.000 token would be picked in preference to the parts we care about.

Cases:
  - parts-only wallet: the funded DID holds fractional RBT and no 1.0 token
  - mint from parts:   an FT mint backed by several part tokens succeeds
  - burn accounting:   free balance falls by exactly the RBT minted, and that
                       same value shows up as BurntForFT
  - denom consistency: token_denom agrees with the real Free rows at every
                       denomination (the decrement that was an increment)
  - no phantom row:    no denom=0 row for the minting DID
  - repeat mint:       a second mint from the same parts wallet still succeeds
  - spend after mint:  RBT left over after the burns is still spendable — the
                       failure the stale counter actually produced

Results use the same {"check", "status", "detail"} shape as the other engines so
they fold into verification.json and the runner's exit-on-fail gate.
"""

from __future__ import annotations

import logging
import time
import uuid
from typing import Any, Dict, List, Optional, Tuple

from test.integration.clients.api_client import NodeClient

log = logging.getLogger(__name__)

# Funding tranches for the parts wallet, did_a -> the suite's own DID. EVERY
# amount is < 1.0, which is what guarantees a parts-only wallet: a transfer
# splits the sender's whole token and hands over the fraction, so the receiver
# can never end up holding a 1.000 token. The 0.7 / 0.3 tranches land at the 0.1
# denomination as well as 0.5, so collection has to span two denominations
# rather than repeatedly taking the same one.
_FUND_TRANCHES: Tuple[float, ...] = (0.5, 0.5, 0.5, 0.5, 0.7, 0.3)
_FUND_TOTAL = round(sum(_FUND_TRANCHES), 3)   # 3.0

# RBT burnt per mint. 1 whole RBT is the minimum a mint can consume (BatchRBTs
# requires batches summing to exactly 1.0) and is deliberately more than any
# single token in the parts wallet, so the mint MUST assemble several parts.
_MINT_RBT = 1

# FTs created per mint. Bounded by token_count * 1000 (createFTs); 10 keeps each
# FT worth 0.1 RBT and the genesis batch small.
_MINT_FT_COUNT = 10

# Spent back to did_a after the mints, to prove the leftover parts are still
# selectable. Smaller than the balance the mints leave behind.
_SPEND_AFTER_MINT = 0.5

# node A must hold this much free RBT before the suite funds its wallet. The
# suite runs late under --run-all-tests, after the other phases have spent from
# did_a; an unfunded wallet is a harness budgeting problem, not an FT defect.
_MIN_REQUIRED_BALANCE = _FUND_TOTAL + 2.0

# Sums of token_value carry float representation noise; compare with a tolerance
# far below the smallest denomination in play (0.001).
_EPSILON = 1e-9

# Settle time after a consensus round before reading state back.
_SETTLE = 6

# token_status values (constants/constants.go): Free=0, BurntForFT=9.
_STATUS_FREE = 0
_STATUS_BURNT_FOR_FT = 9

# token_type ids are lowercase names in the token_type table.
_TYPE_RBT = "rbt"
_TYPE_FT = "ft"


class FTPartsEngine:
    """Asserts FT minting works, and accounts correctly, when backed by parts."""

    def __init__(
        self,
        node_a: NodeClient,
        node_b: NodeClient,
        quorum: NodeClient,
        did_a: str,
        db_a: Any,
        password: str = "mypassword",
    ) -> None:
        self.node_a = node_a
        self.node_b = node_b
        self.quorum = quorum
        self.did_a = did_a
        self.db_a = db_a
        self.password = password

        # The suite's own DID on node A, funded with parts only.
        self.parts_did: Optional[str] = None

        # FT series minted here, so the balance check knows what to look for.
        self._minted: List[Dict[str, Any]] = []

    # ---------------------------------------------------------------- helpers

    def _query(self, sql: str, params: tuple = ()) -> List[tuple]:
        """Run a read-only query against node A's DB.

        Connects by literal IPv4 rather than reusing db_a._connect(): in native
        mode Postgres binds 127.0.0.1 only, while "localhost" resolves to ::1
        first on some hosts, which fails with a misleading 'database does not
        exist'. Copying the params and pinning the host keeps this suite working
        in both native and Docker modes.
        """
        import psycopg2

        params_ipv4 = dict(self.db_a.conn_params)
        if params_ipv4.get("host") == "localhost":
            params_ipv4["host"] = "127.0.0.1"

        with psycopg2.connect(**params_ipv4) as conn, conn.cursor() as cur:
            cur.execute(sql, params)
            return cur.fetchall()

    def _free_balance(self, did: str) -> float:
        """Sum of Free RBT held by *did*, straight from the DB."""
        rows = self._query(
            """
            SELECT COALESCE(SUM(token_value), 0)
              FROM tokens
             WHERE did = %s
               AND token_status = %s
               AND token_type = (SELECT id FROM token_type WHERE name = %s)
            """,
            (did, _STATUS_FREE, _TYPE_RBT),
        )
        return float(rows[0][0])

    def _free_rows_by_denom(self, did: str) -> List[Tuple[float, int]]:
        """(denomination, count) of the Free RBT rows held by *did*."""
        rows = self._query(
            """
            SELECT token_value, COUNT(*)
              FROM tokens
             WHERE did = %s
               AND token_status = %s
               AND token_type = (SELECT id FROM token_type WHERE name = %s)
             GROUP BY token_value
             ORDER BY token_value DESC
            """,
            (did, _STATUS_FREE, _TYPE_RBT),
        )
        return [(float(v), int(n)) for v, n in rows]

    def _burnt_for_ft(self, did: str) -> Tuple[float, int]:
        """(summed value, row count) of RBT *did* has burnt for FT minting."""
        rows = self._query(
            """
            SELECT COALESCE(SUM(token_value), 0), COUNT(*)
              FROM tokens
             WHERE did = %s
               AND token_status = %s
               AND token_type = (SELECT id FROM token_type WHERE name = %s)
            """,
            (did, _STATUS_BURNT_FOR_FT, _TYPE_RBT),
        )
        return float(rows[0][0]), int(rows[0][1])

    def _ft_token_count(self, did: str) -> int:
        """Number of Free FT tokens held by *did* in the tokens table."""
        rows = self._query(
            """
            SELECT COUNT(*)
              FROM tokens
             WHERE did = %s
               AND token_status = %s
               AND token_type = (SELECT id FROM token_type WHERE name = %s)
            """,
            (did, _STATUS_FREE, _TYPE_FT),
        )
        return int(rows[0][0])

    def _denom_rows(self, did: str) -> List[Tuple[float, int]]:
        """Raw (denom, count) rows of token_denom for *did*."""
        rows = self._query(
            "SELECT denom, count FROM token_denom WHERE did = %s ORDER BY denom DESC",
            (did,),
        )
        return [(float(d), int(c)) for d, c in rows]

    def _denom_drift(self, did: str) -> List[tuple]:
        """Rows where token_denom disagrees with the real count of Free rows.

        Returns (denom, counter, actual, drift) per mismatch; empty when the
        counter is consistent. A FULL OUTER JOIN so a counter row with no
        matching Free tokens and Free tokens with no counter row are both
        caught — the burn defect produces the former.
        """
        return self._query(
            """
            SELECT COALESCE(d.denom, f.denom),
                   COALESCE(d.count, 0),
                   COALESCE(f.free_rows, 0),
                   COALESCE(d.count, 0) - COALESCE(f.free_rows, 0) AS drift
              FROM (SELECT denom, count FROM token_denom WHERE did = %s) d
              FULL OUTER JOIN (
                    SELECT token_value AS denom, COUNT(*) AS free_rows
                      FROM tokens
                     WHERE did = %s
                       AND token_type = (SELECT id FROM token_type WHERE name = %s)
                       AND token_status = %s
                     GROUP BY token_value
              ) f ON f.denom = d.denom
             WHERE COALESCE(d.count, 0) - COALESCE(f.free_rows, 0) <> 0
             ORDER BY 1 DESC
            """,
            (did, did, _TYPE_RBT, _STATUS_FREE),
        )

    def _status_breakdown(self, did: str, denom: float) -> str:
        """Where this denomination's non-Free tokens went, as 'status=N n=C'.

        A drift means the counter still claims tokens that have left Free, so
        the statuses they moved to name the path that failed to decrement.
        """
        rows = self._query(
            """
            SELECT token_status, COUNT(*)
              FROM tokens
             WHERE did = %s
               AND token_value = %s
               AND token_type = (SELECT id FROM token_type WHERE name = %s)
               AND token_status <> %s
             GROUP BY token_status
             ORDER BY token_status
            """,
            (did, denom, _TYPE_RBT, _STATUS_FREE),
        )
        if not rows:
            return "no non-Free rows"
        return ", ".join(f"status={s} n={n}" for s, n in rows)

    def _mint(self, label: str, start_index: int) -> Dict[str, Any]:
        """Mint one FT series from the parts wallet. Raises on failure.

        The FT id is ``<ft_name>_<did>_<index>``, so the name must be unique per
        creator and must not contain an underscore of its own.
        """
        ft_name = f"FTPARTS{label}{uuid.uuid4().hex[:8]}".upper()
        result = self.node_a.mint_ft(
            did=self.parts_did,
            ft_name=ft_name,
            ft_count=_MINT_FT_COUNT,
            token_count=_MINT_RBT,
            ft_num_start_index=start_index,
        )
        record = {"ft_name": ft_name, "result": result}
        self._minted.append(record)
        return record

    # ------------------------------------------------------------------- run

    def run(self) -> List[Dict[str, str]]:
        """Run every parts-mint check. Never raises — failures become results."""
        results: List[Dict[str, str]] = []

        # Under --run-all-tests this suite runs late, after the shuttle and the
        # asset phases have spent from did_a. With nothing left to fund the
        # parts wallet, every check below would fail for a reason that has
        # nothing to do with FT accounting. Separate the two up front.
        balance = self._free_balance(self.did_a)
        if balance < _MIN_REQUIRED_BALANCE:
            log.warning(
                "FT parts: skipping — node A holds %s RBT, need >= %s",
                balance, _MIN_REQUIRED_BALANCE,
            )
            return [{
                "check": "FTPARTS_PRECONDITION",
                "status": "SKIP",
                "detail": (
                    f"node A holds {balance} free RBT for did={self.did_a}, below "
                    f"the {_MIN_REQUIRED_BALANCE} needed to fund a parts wallet. "
                    "Earlier phases drained it; raise min_balance_buffer in the "
                    "config rather than reading this as an FT failure."
                ),
            }]

        try:
            results.extend(self._setup_parts_wallet())
        except Exception as exc:  # noqa: BLE001 - report, never abort the suite
            log.error("FT parts: parts wallet setup failed: %s", exc)
            return results + [{
                "check": "FTPARTS_SETUP",
                "status": "FAIL",
                "detail": f"could not build the parts wallet: {exc}",
            }]

        for phase in (
            self._check_mint_from_parts,
            self._check_repeat_mint,
            self._check_spend_after_mint,
        ):
            try:
                results.extend(phase())
            except Exception as exc:  # noqa: BLE001 - report, never abort the suite
                log.error("FT parts phase %s failed: %s", phase.__name__, exc)
                results.append({
                    "check": f"FTPARTS_{phase.__name__.strip('_').upper()}",
                    "status": "FAIL",
                    "detail": f"phase raised: {exc}",
                })

        return results

    # ---------------------------------------------------------------- phases

    def _setup_parts_wallet(self) -> List[Dict[str, str]]:
        """Create a DID on node A and fund it with fractional RBT only."""
        log.info("=== FT PARTS: creating a dedicated DID on node A ===")

        created = self.node_a.create_did(self.password)
        self.parts_did = created["did"]

        # Give IPFS a moment to propagate the new DID object before RegisterDID
        # (same wait the intra-node engine needs).
        time.sleep(5)
        self.node_a.register_did(self.parts_did)

        # The funding transfers run through the quorum, which has to resolve the
        # new DID to node A's peer — so broadcast the mapping first.
        peer_a = self.node_a.get_peer_id()
        self.node_b.add_peer_details(peer_a, self.parts_did)
        self.quorum.add_peer_details(peer_a, self.parts_did)

        log.info(
            "=== FT PARTS: funding %s with %s (tranches %s) ===",
            self.parts_did, _FUND_TOTAL, _FUND_TRANCHES,
        )
        for i, amount in enumerate(_FUND_TRANCHES, start=1):
            self.node_a.transfer_rbt(
                sender_did=self.did_a,
                receiver_did=self.parts_did,
                amount=amount,
                password=self.password,
            )
            log.info("FT parts: funded tranche %d/%d (%s RBT)",
                     i, len(_FUND_TRANCHES), amount)
            time.sleep(2)

        time.sleep(_SETTLE)

        balance = self._free_balance(self.parts_did)
        by_denom = self._free_rows_by_denom(self.parts_did)
        wholes = [(d, n) for d, n in by_denom if d >= 1.0]
        breakdown = ", ".join(f"{d}x{n}" for d, n in by_denom) or "<empty>"

        if wholes:
            status, detail = "FAIL", (
                f"parts wallet {self.parts_did} holds whole tokens ({wholes}); "
                f"every funding tranche was < 1.0 so a 1.000 token cannot have "
                f"been transferred — the mint below would not exercise the parts "
                f"path. Free rows: {breakdown}"
            )
        elif abs(balance - _FUND_TOTAL) >= _EPSILON:
            status, detail = "FAIL", (
                f"parts wallet holds {balance} RBT, expected {_FUND_TOTAL} from "
                f"tranches {_FUND_TRANCHES}; funding did not land as sent. "
                f"Free rows: {breakdown}"
            )
        else:
            status, detail = "PASS", (
                f"parts wallet holds {balance} RBT across {len(by_denom)} "
                f"fractional denomination(s) and no whole token: {breakdown}"
            )

        return [{
            "check": "FTPARTS_WALLET_PARTS_ONLY",
            "status": status,
            "detail": detail,
        }]

    def _check_mint_from_parts(self) -> List[Dict[str, str]]:
        """Mint an FT series backed by part RBTs and audit what it consumed."""
        out: List[Dict[str, str]] = []

        balance_before = self._free_balance(self.parts_did)
        burnt_before, burnt_rows_before = self._burnt_for_ft(self.parts_did)
        log.info(
            "=== FT PARTS: minting %d FTs from %s RBT of parts (free=%s) ===",
            _MINT_FT_COUNT, _MINT_RBT, balance_before,
        )

        try:
            record = self._mint("A", start_index=0)
        except Exception as exc:  # noqa: BLE001 - reported as a result
            return [{
                "check": "FTPARTS_MINT_FROM_PARTS",
                "status": "FAIL",
                "detail": (
                    f"minting {_MINT_FT_COUNT} FTs from {_MINT_RBT} RBT held only "
                    f"as parts failed: {exc}"
                ),
            }]

        time.sleep(_SETTLE)

        balance_after = self._free_balance(self.parts_did)
        burnt_after, burnt_rows_after = self._burnt_for_ft(self.parts_did)
        burnt_value = burnt_after - burnt_before
        burnt_rows = burnt_rows_after - burnt_rows_before

        # 1. The mint succeeded, and it really did burn PARTS: more than one RBT
        #    row went to BurntForFT for a single whole RBT of value.
        if burnt_rows > 1:
            out.append({
                "check": "FTPARTS_MINT_FROM_PARTS",
                "status": "PASS",
                "detail": (
                    f"minted {_MINT_FT_COUNT} FTs ({record['ft_name']}) by burning "
                    f"{burnt_rows} part tokens totalling {burnt_value} RBT"
                ),
            })
        else:
            out.append({
                "check": "FTPARTS_MINT_FROM_PARTS",
                "status": "FAIL",
                "detail": (
                    f"mint burnt {burnt_rows} RBT row(s) worth {burnt_value}; a "
                    f"parts-only wallet must assemble {_MINT_RBT} RBT from several "
                    "part tokens, so a single burnt row means the wallet was not "
                    "parts-only and this suite proved nothing"
                ),
            })

        # 2. The mint costs exactly the RBT requested — no part silently
        #    destroyed, no change lost.
        spent = balance_before - balance_after
        if abs(spent - _MINT_RBT) < _EPSILON and abs(burnt_value - _MINT_RBT) < _EPSILON:
            out.append({
                "check": "FTPARTS_PARTS_BURNT",
                "status": "PASS",
                "detail": (
                    f"free balance fell by exactly {spent} and the same {burnt_value} "
                    f"is recorded as BurntForFT ({balance_before} -> {balance_after})"
                ),
            })
        else:
            out.append({
                "check": "FTPARTS_PARTS_BURNT",
                "status": "FAIL",
                "detail": (
                    f"free balance fell by {spent} and {burnt_value} was burnt, "
                    f"expected {_MINT_RBT} for both ({balance_before} -> "
                    f"{balance_after}); the parts backing the mint were not "
                    "accounted for one-to-one"
                ),
            })

        # 3. The FTs exist, per the API and independently per the tokens table.
        out.append(self._check_ft_balance(expected=_MINT_FT_COUNT))

        # 4+5. The counter that drives token selection must still describe
        #      reality after the burns — the defect this suite exists for.
        out.extend(self._check_denom_state("after the first parts mint"))

        return out

    def _check_ft_balance(self, expected: int) -> Dict[str, str]:
        """The minting DID must hold *expected* FTs, per API and per DB.

        The API is the contract; the tokens table is an independent cross-check
        so a reporting bug in either is distinguishable from a mint that did not
        create the FTs. Keys are name/creator/value/count (types/balance.go).
        """
        api_count = 0
        api_error: Optional[str] = None
        try:
            for entry in self.node_a.get_ft_balance(self.parts_did):
                if any(entry.get("name") == m["ft_name"] for m in self._minted):
                    api_count += int(entry.get("count") or 0)
        except Exception as exc:  # noqa: BLE001 - reported in the detail
            api_error = str(exc)

        db_count = self._ft_token_count(self.parts_did)

        if api_count == expected and db_count == expected:
            return {
                "check": "FTPARTS_FT_BALANCE",
                "status": "PASS",
                "detail": (
                    f"parts-funded DID holds {expected} FTs (API {api_count}, "
                    f"tokens table {db_count})"
                ),
            }
        return {
            "check": "FTPARTS_FT_BALANCE",
            "status": "FAIL",
            "detail": (
                f"expected {expected} FTs for the parts-funded DID, got API "
                f"{api_count}{f' (error: {api_error})' if api_error else ''}, "
                f"tokens table {db_count}"
            ),
        }

    def _check_denom_state(self, when: str) -> List[Dict[str, str]]:
        """token_denom must match the real Free rows, with no denom=0 row.

        Two separate checks because they are two separate defects in the same
        write: the increment-instead-of-decrement (drift) and the uncopied
        TokenValue (phantom denom=0). Reporting them together would hide which
        half regressed.
        """
        out: List[Dict[str, str]] = []
        drift = self._denom_drift(self.parts_did)
        rows = self._denom_rows(self.parts_did)
        counter = ", ".join(f"{d}x{c}" for d, c in rows) or "<empty>"

        if not drift:
            out.append({
                "check": "FTPARTS_DENOM_CONSISTENT",
                "status": "PASS",
                "detail": (
                    f"token_denom matches the actual Free RBT rows at every "
                    f"denomination {when}: {counter}"
                ),
            })
        else:
            detail = "; ".join(
                f"denom={d} counter={c} actual={a} drift={dr} "
                f"[{self._status_breakdown(self.parts_did, d)}]"
                for d, c, a, dr in drift
            )
            out.append({
                "check": "FTPARTS_DENOM_CONSISTENT",
                "status": "FAIL",
                "detail": (
                    f"token_denom drifted {when} for did={self.parts_did}: {detail}. "
                    "A positive drift at a denomination whose tokens are "
                    f"status={_STATUS_BURNT_FOR_FT} means the FT burn incremented "
                    "the counter instead of decrementing it; token selection will "
                    "pick tokens that are no longer Free"
                ),
            })

        zero_rows = [(d, c) for d, c in rows if abs(d) < _EPSILON]
        if not zero_rows:
            out.append({
                "check": "FTPARTS_NO_ZERO_DENOM",
                "status": "PASS",
                "detail": f"no phantom denom=0 row for the minting DID {when}",
            })
        else:
            out.append({
                "check": "FTPARTS_NO_ZERO_DENOM",
                "status": "FAIL",
                "detail": (
                    f"token_denom carries a denom=0 row {when} ({zero_rows}) for "
                    f"did={self.parts_did}: the FT burn wrote the counter keyed on "
                    "an uncopied TokenValue, counting tokens that can never be "
                    "selected and skipping the real denomination"
                ),
            })

        return out

    def _check_repeat_mint(self) -> List[Dict[str, str]]:
        """A second mint from the same parts wallet must also succeed.

        Consecutive mints are how a broken counter surfaces: the first mint
        leaves token_denom advertising the parts it burnt, and the next
        selection asks for them.
        """
        out: List[Dict[str, str]] = []

        balance_before = self._free_balance(self.parts_did)
        try:
            record = self._mint("B", start_index=_MINT_FT_COUNT)
        except Exception as exc:  # noqa: BLE001 - reported as a result
            return [{
                "check": "FTPARTS_REPEAT_MINT",
                "status": "FAIL",
                "detail": (
                    f"second mint from the same parts wallet failed: {exc}. The "
                    "first mint left token_denom describing tokens it had already "
                    "burnt, so selection could not find the rows it planned for"
                ),
            }]

        time.sleep(_SETTLE)
        balance_after = self._free_balance(self.parts_did)
        spent = balance_before - balance_after

        if abs(spent - _MINT_RBT) < _EPSILON:
            out.append({
                "check": "FTPARTS_REPEAT_MINT",
                "status": "PASS",
                "detail": (
                    f"second mint ({record['ft_name']}) succeeded and cost exactly "
                    f"{spent} RBT ({balance_before} -> {balance_after})"
                ),
            })
        else:
            out.append({
                "check": "FTPARTS_REPEAT_MINT",
                "status": "FAIL",
                "detail": (
                    f"second mint cost {spent} RBT, expected {_MINT_RBT} "
                    f"({balance_before} -> {balance_after})"
                ),
            })

        out.append(self._check_ft_balance(expected=_MINT_FT_COUNT * 2))
        out.extend(self._check_denom_state("after the second parts mint"))

        return out

    def _check_spend_after_mint(self) -> List[Dict[str, str]]:
        """The parts left over after the burns must still be spendable.

        This is the user-visible failure a stale token_denom produces: selection
        reads the counter, asks for denominations whose tokens are now
        BurntForFT, and the transfer dies on 'lockSelectedTokens: no tokens
        provided' — long after the mint that broke the counter.
        """
        balance_before = self._free_balance(self.parts_did)
        if balance_before + _EPSILON < _SPEND_AFTER_MINT:
            return [{
                "check": "FTPARTS_SPEND_AFTER_MINT",
                "status": "FAIL",
                "detail": (
                    f"parts wallet holds {balance_before} RBT after the mints, "
                    f"less than the {_SPEND_AFTER_MINT} this check spends — the "
                    f"mints consumed more than the {2 * _MINT_RBT} RBT they burnt"
                ),
            }]

        try:
            self.node_a.transfer_rbt(
                sender_did=self.parts_did,
                receiver_did=self.did_a,
                amount=_SPEND_AFTER_MINT,
                password=self.password,
            )
        except Exception as exc:  # noqa: BLE001 - reported as a result
            return [{
                "check": "FTPARTS_SPEND_AFTER_MINT",
                "status": "FAIL",
                "detail": (
                    f"spending {_SPEND_AFTER_MINT} RBT of the parts left after "
                    f"minting failed: {exc}. token_denom still advertises the "
                    "parts the FT mint burnt, so token selection picks rows that "
                    "are no longer Free"
                ),
            }]

        time.sleep(_SETTLE)
        balance_after = self._free_balance(self.parts_did)
        spent = balance_before - balance_after

        if abs(spent - _SPEND_AFTER_MINT) < _EPSILON:
            status, detail = "PASS", (
                f"spent {spent} RBT from the post-mint parts balance "
                f"({balance_before} -> {balance_after})"
            )
        else:
            status, detail = "FAIL", (
                f"spending {_SPEND_AFTER_MINT} RBT moved {spent} "
                f"({balance_before} -> {balance_after})"
            )

        return [{"check": "FTPARTS_SPEND_AFTER_MINT", "status": status, "detail": detail}]
