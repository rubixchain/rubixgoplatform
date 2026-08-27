"""
sc_collateral.py — smart-contract deploy collateral accounting.

Deploying a smart contract with a value locks RBT as collateral: the tokens
backing that value are marked Committed (terminal) and the rest must come back
to the deployer as change.

The bug this guards against: LockTokensForSplit selects whole denominations, so
backing a 0.001 commitment picks a whole 1.000 token. If that token is committed
as-is, the other 0.999 is silently destroyed — a 0.001 contract costs a full RBT.

SCCOL_DENOM_CONSISTENT covers a second, independent defect on the same flow:
post-consensus skipped its token_denom update for an SC deploy (the collateral
rides in CommittedTokens with Tokens.RBT empty), and when it did run, the
isLocalTransfer credit landed back on the initiator — a deploy pins Owner to
Initiator — cancelling the decrement. Either way the counter kept advertising
committed tokens as spendable until a later selection failed with
"lockSelectedTokens: no tokens provided".

This is the first token_denom invariant anywhere in the repo: the table is
written by six code paths and, before this suite, was never once checked against
reality. That is why it has been repaired repeatedly and drifted again. Running
it immediately surfaced two further FT-side defects, reported separately by
SCCOL_FT_DENOM_DRIFT with their attributed cause. That check FAILS: the drift is
a real accounting defect (the counter advertises tokens that cannot be selected)
and stays red until the FT paths decrement token_denom the way the RBT and SC
paths do. It is kept separate from SCCOL_DENOM_CONSISTENT so an SC regression is
never mistaken for the known FT one.

Why the existing smart_contract_engine suite cannot catch this: it deploys at
sc_value=1.0, the single value where committing a whole 1.000 token is exactly
correct, and it asserts only that deployment succeeded — never on balance. The
defect is only visible when sc_value < denomination, so every case here uses a
FRACTIONAL value.

Cases:
  - balance delta:  deployer's free balance drops by exactly sc_value
  - committed sum:  Committed RBT totals sc_value, not a whole denomination
  - denom drift:    token_denom agrees with the real Free rows at the SC
                    collateral denomination
  - FT denom drift: drift at other denominations, reported separately with its
                    cause attributed (known-failing: FT paths do not decrement)
  - repeat deploys: three fractional deploys in a row all succeed (the failure
                    mode that first exposed this — contracts 2 and 3 died once
                    the stale counter over-reported availability)

Results use the same {"check", "status", "detail"} shape as the other engines so
they fold into verification.json and the runner's exit-on-fail gate.
"""

from __future__ import annotations

import logging
import time
from typing import Any, Dict, List

from test.integration.clients.api_client import NodeClient
from test.integration.engines.file_selector import select_smart_contract_files

log = logging.getLogger(__name__)

# Deliberately smaller than the 1.000 denomination the wallet holds, so the
# deploy must split a whole token and return change. At 1.0 the split is a
# no-op and the bug is invisible.
_SC_VALUE = 0.001

# Sum of token_value may carry float representation noise; compare with a
# tolerance well below the smallest value under test.
_EPSILON = 1e-9

# Settle time after a consensus round before reading balances back.
_SETTLE = 6

# The suite makes 4 deploys, each splitting a whole token to take _SC_VALUE from
# it. One whole RBT per deploy must be selectable at the moment of the split, so
# require a small whole-token float rather than 4 * _SC_VALUE.
_MIN_REQUIRED_BALANCE = 4.0

# token_status values (constants/constants.go): Free=0, Committed=5,
# BurntForFT=9.
_STATUS_FREE = 0
_STATUS_COMMITTED = 5
_STATUS_BURNT_FOR_FT = 9

# token_type ids are lowercase names in the token_type table.
_TYPE_RBT = "rbt"


class SCCollateralEngine:
    """Asserts SC deploy collateral is split, not consumed whole."""

    def __init__(
        self,
        node_a: NodeClient,
        did_a: str,
        db_a: Any,
        password: str = "mypassword",
    ) -> None:
        self.node_a = node_a
        self.did_a = did_a
        self.db_a = db_a
        self.password = password

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

    def _free_balance(self) -> float:
        """Sum of Free RBT for the deployer, straight from the DB."""
        rows = self._query(
            """
            SELECT COALESCE(SUM(token_value), 0)
              FROM tokens
             WHERE did = %s
               AND token_status = %s
               AND token_type = (SELECT id FROM token_type WHERE name = %s)
            """,
            (self.did_a, _STATUS_FREE, _TYPE_RBT),
        )
        return float(rows[0][0])

    def _committed_sum(self) -> float:
        """Sum of Committed RBT for the deployer — the SC collateral."""
        rows = self._query(
            """
            SELECT COALESCE(SUM(token_value), 0)
              FROM tokens
             WHERE did = %s
               AND token_status = %s
               AND token_type = (SELECT id FROM token_type WHERE name = %s)
            """,
            (self.did_a, _STATUS_COMMITTED, _TYPE_RBT),
        )
        return float(rows[0][0])

    def _denom_drift(self) -> List[tuple]:
        """Rows where token_denom disagrees with the real count of Free rows.

        Returns (denom, counter, actual, drift) for each mismatch; empty when
        the counter is consistent. Scoped to self.did_a: node A also hosts the
        intra-node secondary DID (did_a2), and mixing the two makes the numbers
        impossible to read.
        """
        return self._query(
            """
            SELECT d.denom,
                   d.count,
                   COALESCE(f.free_rows, 0),
                   d.count - COALESCE(f.free_rows, 0) AS drift
              FROM token_denom d
              LEFT JOIN (
                    SELECT did, token_value AS denom, COUNT(*) AS free_rows
                      FROM tokens
                     WHERE token_type = (SELECT id FROM token_type WHERE name = %s)
                       AND token_status = %s
                     GROUP BY did, token_value
              ) f ON f.did = d.did AND f.denom = d.denom
             WHERE d.did = %s
               AND d.count - COALESCE(f.free_rows, 0) <> 0
            """,
            (_TYPE_RBT, _STATUS_FREE, self.did_a),
        )

    def _status_breakdown(self, denom: float) -> str:
        """Where this denomination's non-Free tokens went, as 'status=N count'.

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
            (self.did_a, denom, _TYPE_RBT, _STATUS_FREE),
        )
        if not rows:
            return "no non-Free rows"
        return ", ".join(f"status={s} n={n}" for s, n in rows)

    def _deploy_fractional(self, label: str) -> Dict[str, Any]:
        """Generate and deploy one contract at _SC_VALUE. Raises on failure."""
        wasm_path, source_path = select_smart_contract_files()
        result = self.node_a.create_smart_contract(self.did_a, wasm_path, source_path)
        sc_id = result.get("smartContractId")
        if not sc_id:
            raise RuntimeError(f"{label}: generate returned no smartContractId")

        req_id = self.node_a.initiate_smart_contract_transaction(
            initiator_did=self.did_a,
            sc_id=sc_id,
            sc_value=_SC_VALUE,
            data="collateral-test",
        )
        response = self.node_a.complete_transaction(req_id, self.password)
        return {"sc_id": sc_id, "response": response}

    # ------------------------------------------------------------------- run

    def run(self) -> List[Dict[str, str]]:
        """Run every collateral check. Never raises — failures become results."""
        results: List[Dict[str, str]] = []

        # Inside --run-all-tests this suite runs late, after the shuttle and the
        # asset phases have spent from the same wallet. With no funds left every
        # deploy fails on "no tokens provided" — the very error this suite exists
        # to detect — for a reason that has nothing to do with the code under
        # test. Distinguish the two up front: an unfunded wallet is a harness
        # budgeting problem (SKIP), not an SC accounting defect (FAIL).
        balance = self._free_balance()
        if balance < _MIN_REQUIRED_BALANCE:
            log.warning(
                "SC collateral: skipping — node A holds %s RBT, need >= %s",
                balance, _MIN_REQUIRED_BALANCE,
            )
            return [{
                "check": "SCCOL_PRECONDITION",
                "status": "SKIP",
                "detail": (
                    f"node A holds {balance} free RBT for did={self.did_a}, "
                    f"below the {_MIN_REQUIRED_BALANCE} this suite needs. Earlier "
                    "phases drained the wallet; raise min_balance_buffer in the "
                    "config rather than reading this as an SC failure."
                ),
            }]

        for phase in (
            self._check_single_deploy_accounting,
            self._check_repeated_fractional_deploys,
        ):
            try:
                results.extend(phase())
            except Exception as exc:  # noqa: BLE001 - report, never abort the suite
                log.error("SC collateral phase %s failed: %s", phase.__name__, exc)
                results.append({
                    "check": f"SCCOL_{phase.__name__.strip('_').upper()}",
                    "status": "FAIL",
                    "detail": f"phase raised: {exc}",
                })

        return results

    # ---------------------------------------------------------------- phases

    def _check_single_deploy_accounting(self) -> List[Dict[str, str]]:
        """One fractional deploy: balance delta, committed sum, denom drift."""
        out: List[Dict[str, str]] = []

        balance_before = self._free_balance()
        committed_before = self._committed_sum()
        log.info(
            "=== SC COLLATERAL: deploying at value=%s (free=%s committed=%s) ===",
            _SC_VALUE, balance_before, committed_before,
        )

        self._deploy_fractional("single")
        time.sleep(_SETTLE)

        balance_after = self._free_balance()
        committed_after = self._committed_sum()

        # 1. The deployer loses exactly the contract's value — not a whole token.
        spent = balance_before - balance_after
        if abs(spent - _SC_VALUE) < _EPSILON:
            out.append({
                "check": "SCCOL_BALANCE_DELTA",
                "status": "PASS",
                "detail": f"free balance fell by exactly {spent} (value {_SC_VALUE})",
            })
        else:
            out.append({
                "check": "SCCOL_BALANCE_DELTA",
                "status": "FAIL",
                "detail": (
                    f"free balance fell by {spent}, expected {_SC_VALUE} "
                    f"({balance_before} -> {balance_after}); "
                    "collateral was committed without splitting, so the change was lost"
                ),
            })

        # 2. Only the contract's value is held as collateral.
        newly_committed = committed_after - committed_before
        if abs(newly_committed - _SC_VALUE) < _EPSILON:
            out.append({
                "check": "SCCOL_COMMITTED_SUM",
                "status": "PASS",
                "detail": f"committed RBT rose by exactly {newly_committed}",
            })
        else:
            out.append({
                "check": "SCCOL_COMMITTED_SUM",
                "status": "FAIL",
                "detail": (
                    f"committed RBT rose by {newly_committed}, expected {_SC_VALUE}; "
                    "a whole denomination was committed to back a fractional value"
                ),
            })

        # 3. token_denom must still agree with reality for the denomination this
        #    suite spends. Drift here is what later causes a spurious
        #    "no tokens provided" on the next deploy.
        #
        #    Drift at OTHER denominations is reported separately rather than
        #    failing this check: it comes from known FT-side defects that this
        #    suite does not exercise and must not be blamed for. Failing on it
        #    would leave the check permanently red and teach people to ignore it.
        drift = self._denom_drift()
        sc_drift = [row for row in drift if abs(float(row[0]) - _SC_VALUE) < _EPSILON]
        other_drift = [row for row in drift if row not in sc_drift]

        if not sc_drift:
            out.append({
                "check": "SCCOL_DENOM_CONSISTENT",
                "status": "PASS",
                "detail": (
                    f"token_denom matches the actual count of Free rows at "
                    f"denom={_SC_VALUE} (the denomination this suite spends)"
                ),
            })
        else:
            detail = "; ".join(
                f"denom={d} counter={c} actual={a} drift={dr} [{self._status_breakdown(d)}]"
                for d, c, a, dr in sc_drift
            )
            out.append({
                "check": "SCCOL_DENOM_CONSISTENT",
                "status": "FAIL",
                "detail": (
                    f"token_denom drifted at the SC collateral denomination for "
                    f"did={self.did_a}: {detail}"
                ),
            })

        out.append(self._report_ft_denom_drift(other_drift))

        return out

    def _report_ft_denom_drift(self, other_drift: List[tuple]) -> Dict[str, str]:
        """Report denom drift this suite did not cause, attributing each cause.

        token_denom has never had an invariant check before this suite, and the
        run surfaces two long-standing FT-side defects:

          - FT minting flips whole RBT parents to BurntForFT
            (core/wallet/token_chain.go, FTGenesisTxn -> UpdateTokenInfo) without
            decrementing token_denom, so the counter keeps advertising burnt
            tokens as spendable.
          - PersistGenesisTokenRecord upserts token_denom keyed on
            token.TokenValue, and an FT token carries value 0, creating a phantom
            denom=0 row counting tokens that can never be selected.

        This FAILS, deliberately. The drift is a real accounting defect: the
        counter advertises tokens that cannot be selected, which is what produced
        "lockSelectedTokens: no tokens provided" on the SC side. Recording it as
        advisory would let it sit indefinitely. It stays red until the FT paths
        decrement token_denom the way the RBT and SC paths now do.
        """
        if not other_drift:
            return {
                "check": "SCCOL_FT_DENOM_DRIFT",
                "status": "PASS",
                "detail": "no denom drift outside the SC collateral denomination",
            }

        parts: List[str] = []
        for denom, counter, actual, drift in other_drift:
            breakdown = self._status_breakdown(denom)
            if abs(float(denom)) < _EPSILON:
                cause = "phantom denom=0 row from FT genesis (FT tokens carry value 0)"
            elif f"status={_STATUS_BURNT_FOR_FT}" in breakdown:
                cause = "RBT burnt for FT minting, never decremented"
            else:
                cause = "unattributed — investigate"
            parts.append(
                f"denom={denom} counter={counter} actual={actual} drift={drift} "
                f"[{breakdown}] -> {cause}"
            )

        return {
            "check": "SCCOL_FT_DENOM_DRIFT",
            "status": "FAIL",
            "detail": (
                f"denom drift not caused by SC deploys, did={self.did_a}: "
                + "; ".join(parts)
            ),
        }

    def _check_repeated_fractional_deploys(self) -> List[Dict[str, str]]:
        """Three fractional deploys back to back must all succeed.

        This is the shape of the original failure: the first deploy consumed a
        whole token, the denom counter kept advertising it, and deploys 2 and 3
        died with "lockSelectedTokens: no tokens provided".
        """
        out: List[Dict[str, str]] = []
        rounds = 3

        balance_before = self._free_balance()
        failures: List[str] = []

        for i in range(1, rounds + 1):
            try:
                self._deploy_fractional(f"repeat-{i}")
                log.info("SC collateral: repeat deploy %d/%d succeeded", i, rounds)
            except Exception as exc:  # noqa: BLE001 - collected, reported below
                failures.append(f"deploy {i}: {exc}")
                log.error("SC collateral: repeat deploy %d/%d failed: %s", i, rounds, exc)
            time.sleep(_SETTLE)

        if failures:
            out.append({
                "check": "SCCOL_REPEATED_DEPLOYS",
                "status": "FAIL",
                "detail": f"{len(failures)}/{rounds} fractional deploys failed: " + "; ".join(failures),
            })
        else:
            out.append({
                "check": "SCCOL_REPEATED_DEPLOYS",
                "status": "PASS",
                "detail": f"all {rounds} consecutive fractional deploys succeeded",
            })

        # Whatever landed, the cost must be value-proportional, never one whole
        # token per contract.
        balance_after = self._free_balance()
        spent = balance_before - balance_after
        expected = _SC_VALUE * (rounds - len(failures))

        if abs(spent - expected) < _EPSILON:
            out.append({
                "check": "SCCOL_REPEATED_COST",
                "status": "PASS",
                "detail": f"{rounds - len(failures)} deploys cost exactly {spent}",
            })
        else:
            out.append({
                "check": "SCCOL_REPEATED_COST",
                "status": "FAIL",
                "detail": (
                    f"{rounds - len(failures)} deploys cost {spent}, expected {expected} "
                    f"({balance_before} -> {balance_after})"
                ),
            })

        return out
