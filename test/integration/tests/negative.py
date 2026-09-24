"""
negative.py — Negative / failure-path integration tests.

Where the happy-path suite (happy_path.py) asserts "the operation succeeds and
the end-state is correct", this suite asserts the inverse: an INVALID operation
is REJECTED for the RIGHT reason, and observable state is UNCHANGED afterward.

This is intentionally stricter than a plain expect_failure (any error = pass).
A run can have failed transactions for unrelated reasons (e.g. token-budget
starvation), so a negative test that only checked "did it fail" would give false
confidence. Each case here asserts BOTH:
  1. the operation raised the expected rejection (error text matches), AND
  2. the relevant balance / token state did not move.

Results are returned in the same shape the happy-path engines use
({"check", "status": "PASS"|"FAIL", "detail"}) so they flow into
verification.json and the runner's exit-on-fail gate.

Cases (4 families):
  - balance violations: transfer from a zero-balance DID; transfer more RBT
    than owned.
  - decimal/precision: transfer with more than MaxSupportedDecimalPlaces (3)
    decimal places.
  - FT over-transfer: transfer more FTs than the DID holds.
  - invalid inputs: unknown/malformed receiver DID; non-positive amount;
    malformed DID on the public-key lookup.
  - locked-NFT release: a child-mint on a SUBSCRIBED parent fails at pledge
    ("Quorum is not setup"); the parent must return to Deployed with no
    lock reference, and the retry against the real quorum must succeed.
"""

from __future__ import annotations

import logging
import time
from typing import Any, Dict, List, Optional

from test.integration.clients.api_client import NodeClient
from test.integration.clients.db_validator import DBValidator

log = logging.getLogger(__name__)

# tokens.token_status values (constants.TokenStatus_*) the lock-release case asserts on.
_STATUS_DEPLOYED = 10

# Node enforces constants.MaxSupportedDecimalPlaces = 3, so anything with >3
# decimal places (and any RBT amount below 0.001) must be rejected.
_TOO_MANY_DECIMALS = 0.00000009  # 8 dp — the old suite's canonical bad amount

# Substrings that identify a CORRECT rejection per family. Matched
# case-insensitively against the raised error text. Lists allow for the
# server wording varying slightly across paths.
_REASON_INSUFFICIENT = ["insufficient balance", "no tokens provided", "not enough"]
_REASON_DECIMAL = ["decimal places", "fractional", "precise"]
_REASON_FT_INSUFFICIENT = ["insufficient", "queryandlockfts", "ft lock failed", "have 0"]
_REASON_INVALID_DID = ["did", "invalid", "not found", "peer", "unknown"]
_REASON_BAD_AMOUNT = ["amount", "invalid", "must be", "greater than", "positive"]
_REASON_QUORUM_NOT_SETUP = ["quorum is not setup"]


class NegativeEngine:
    """Runs negative-path checks against an already-set-up cluster.

    Args:
        node_a/node_b: NodeClient for the two transacting nodes.
        did_a/did_b:   their primary DIDs.
        password:      wallet password for completing transactions.
        did_q:         the real (set-up) quorum DID, restored after the
                       locked-NFT case repoints nodeB at a bogus quorum.
        db_b:          DBValidator for nodeB, used to read the parent NFT's
                       tokens row. The locked-NFT case is skipped without it.
    """

    def __init__(
        self,
        node_a: NodeClient,
        node_b: NodeClient,
        did_a: str,
        did_b: str,
        password: str = "mypassword",
        did_q: Optional[str] = None,
        db_b: Optional[DBValidator] = None,
    ) -> None:
        self.node_a = node_a
        self.node_b = node_b
        self.did_a = did_a
        self.did_b = did_b
        self.password = password
        self.did_q = did_q
        self.db_b = db_b

    # ------------------------------------------------------------------
    # Assertion helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _reason_matches(err_text: str, reasons: List[str]) -> bool:
        low = err_text.lower()
        return any(r in low for r in reasons)

    def _expect_rejection(
        self,
        check: str,
        op,
        reasons: List[str],
        unchanged: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, str]:
        """Run *op* expecting it to RAISE with one of *reasons* in the message.

        op: zero-arg callable performing the invalid operation.
        unchanged: optional {"label": (getter_callable, before_value)} map; after
                   the (expected) rejection, each getter must still equal
                   before_value, proving no state moved.
        Returns a verification-result dict.
        """
        try:
            op()
        except Exception as exc:  # noqa: BLE001 — any raise is the rejection path
            err = str(exc)
            if not self._reason_matches(err, reasons):
                return {
                    "check": check,
                    "status": "FAIL",
                    "detail": f"rejected, but wrong reason: {err[:160]}",
                }
            # Rejected for the right reason — now confirm state is unchanged.
            if unchanged:
                for label, (getter, before) in unchanged.items():
                    try:
                        after = getter()
                    except Exception as gexc:  # noqa: BLE001
                        return {
                            "check": check,
                            "status": "FAIL",
                            "detail": f"rejected correctly but state read failed ({label}): {gexc}",
                        }
                    if after != before:
                        return {
                            "check": check,
                            "status": "FAIL",
                            "detail": f"rejected correctly but {label} changed: {before} -> {after}",
                        }
            return {
                "check": check,
                "status": "PASS",
                "detail": f"correctly rejected ({err[:80]})",
            }
        # op did NOT raise — the invalid operation was wrongly ACCEPTED.
        return {
            "check": check,
            "status": "FAIL",
            "detail": "invalid operation was accepted (expected rejection)",
        }

    # ------------------------------------------------------------------
    # Test families
    # ------------------------------------------------------------------

    def run(self) -> List[Dict[str, str]]:
        results: List[Dict[str, str]] = []
        log.info("=== NEGATIVE TESTS: starting ===")
        results.extend(self._balance_violations())
        results.extend(self._decimal_precision())
        results.extend(self._ft_over_transfer())
        results.extend(self._invalid_inputs())
        results.extend(self._locked_nft_release_on_quorum_failure())
        passed = sum(1 for r in results if r["status"] == "PASS")
        log.info("=== NEGATIVE TESTS: %d/%d passed ===", passed, len(results))
        return results

    def _locked_nft_release_on_quorum_failure(self) -> List[Dict[str, str]]:
        """A failed child-mint on a subscribed parent must not leave it Locked.

        nodeA deploys an NFT and nodeB subscribes, so nodeB's tokens row for it
        carries nodeA's DID. nodeB then repoints its quorum at did_a (which never
        ran setup-quorum) and child-mints: the pledge is rejected with "Quorum is
        not setup". BuildTransactionInfoFromRequest has already locked the parent
        by then, so the failure path must release it by lock reference — a
        release keyed on the initiator DID misses this row and strands it. A
        retry against the real quorum then proves the parent is usable again.
        nodeB's quorum is restored in all cases.
        """
        if self.db_b is None or not self.did_q:
            log.warning("NEG_NFT_LOCK_RELEASE: skipped (db_b / did_q not provided)")
            return []

        from test.integration.engines.file_selector import select_nft_files

        check = "NEG_NFT_LOCK_RELEASED_ON_QUORUM_FAILURE"
        out: List[Dict[str, str]] = []

        # Setup: deploy on nodeA, subscribe on nodeB, wait for the row to sync.
        try:
            metadata_path, artifact_path = select_nft_files()
            parent = self.node_a.create_nft(self.did_a, metadata_path, artifact_path)["nft_id"]
            self.node_a.transfer_nft(
                sender_did=self.did_a, receiver_did=self.did_a, nft_id=parent,
                data="negative: parent for lock-release case", password=self.password,
            )
            self.node_b.subscribe_nft(parent)
            before = None
            deadline = time.time() + 60
            while time.time() < deadline:
                before = self.db_b.get_token_lock_state(parent)
                if before is not None:
                    break
                time.sleep(2)
        except Exception as exc:  # noqa: BLE001
            return [{"check": check, "status": "FAIL", "detail": f"setup failed: {str(exc)[:160]}"}]

        if before is None:
            return [{"check": check, "status": "FAIL",
                     "detail": "parent NFT never appeared in nodeB's tokens table after subscribe"}]
        if before != (_STATUS_DEPLOYED, None):
            return [{"check": check, "status": "FAIL",
                     "detail": f"unexpected parent state on nodeB before the case: {before}"}]

        # Break nodeB's quorum: did_a is a valid peer but never ran setup-quorum.
        try:
            self.node_b.remove_all_quorums()
            self.node_b.add_quorum(self.did_a)

            out.append(self._expect_rejection(
                check,
                lambda: self.node_b.mint_nft_children(
                    initiator_did=self.did_b, parent_nft_id=parent,
                    number_of_children=1, password=self.password,
                ),
                _REASON_QUORUM_NOT_SETUP,
                unchanged={"nodeB_parent_row": (lambda: self.db_b.get_token_lock_state(parent), before)},
            ))
        finally:
            self.node_b.remove_all_quorums()
            self.node_b.add_quorum(self.did_q)

        # The symptom being guarded: later attempts must work once the quorum is right.
        retry_check = "NEG_NFT_CHILD_MINT_RETRY_AFTER_QUORUM_FIX"
        try:
            res = self.node_b.mint_nft_children(
                initiator_did=self.did_b, parent_nft_id=parent,
                number_of_children=1, password=self.password,
            )
            kids = self.node_b.extract_minted_children(res)
            if len(kids) == 1:
                out.append({"check": retry_check, "status": "PASS",
                            "detail": f"child minted on retry: {kids[0].get('childNFTId', '')[:12]}..."})
            else:
                out.append({"check": retry_check, "status": "FAIL",
                            "detail": f"retry succeeded but minted {len(kids)} children (expected 1)"})
        except Exception as exc:  # noqa: BLE001
            out.append({"check": retry_check, "status": "FAIL",
                        "detail": f"retry after quorum fix was rejected: {str(exc)[:160]}"})
        return out

    def _balance_violations(self) -> List[Dict[str, str]]:
        out: List[Dict[str, str]] = []

        # (a) Transfer from a DID with ZERO balance. nodeB starts unfunded in a
        # fresh cluster; if it happens to hold RBT, this still asserts you can't
        # send more than you have via the insufficient path below.
        bal_b = self.node_b.get_balance(self.did_b)
        out.append(self._expect_rejection(
            "NEG_RBT_ZERO_BALANCE",
            lambda: self.node_b.transfer_rbt(self.did_b, self.did_a, bal_b + 100, self.password),
            _REASON_INSUFFICIENT,
            unchanged={"nodeB_balance": (lambda: self.node_b.get_balance(self.did_b), bal_b)},
        ))

        # (b) Transfer MORE than owned from nodeA (insufficient balance).
        bal_a = self.node_a.get_balance(self.did_a)
        out.append(self._expect_rejection(
            "NEG_RBT_INSUFFICIENT",
            lambda: self.node_a.transfer_rbt(self.did_a, self.did_b, bal_a + 1000, self.password),
            _REASON_INSUFFICIENT,
            unchanged={"nodeA_balance": (lambda: self.node_a.get_balance(self.did_a), bal_a)},
        ))
        return out

    def _decimal_precision(self) -> List[Dict[str, str]]:
        bal_a = self.node_a.get_balance(self.did_a)
        return [self._expect_rejection(
            "NEG_RBT_DECIMAL_PLACES",
            lambda: self.node_a.transfer_rbt(self.did_a, self.did_b, _TOO_MANY_DECIMALS, self.password),
            _REASON_DECIMAL + _REASON_INSUFFICIENT,  # node may reject on precision or on amount
            unchanged={"nodeA_balance": (lambda: self.node_a.get_balance(self.did_a), bal_a)},
        )]

    def _ft_over_transfer(self) -> List[Dict[str, str]]:
        # Attempt to transfer FTs that nodeA does not hold (huge count under a
        # name that does not exist / is not owned). Asserts the FT lock rejects.
        out: List[Dict[str, str]] = []
        try:
            ft_before = self.node_a.get_ft_balance(self.did_a)
        except Exception:  # noqa: BLE001
            ft_before = []
        # transfer_ft(sender_did, receiver_did, ft_name, ft_count, creator_did, …)
        out.append(self._expect_rejection(
            "NEG_FT_OVER_TRANSFER",
            lambda: self.node_a.transfer_ft(
                self.did_a, self.did_b, "neg-nonexistent-ft", 1_000_000, self.did_a,
                password=self.password,
            ),
            _REASON_FT_INSUFFICIENT,
            unchanged={"nodeA_ft": (lambda: self.node_a.get_ft_balance(self.did_a), ft_before)},
        ))
        return out

    def _invalid_inputs(self) -> List[Dict[str, str]]:
        out: List[Dict[str, str]] = []

        # (a) Unknown / malformed receiver DID.
        out.append(self._expect_rejection(
            "NEG_INVALID_RECEIVER_DID",
            lambda: self.node_a.transfer_rbt(self.did_a, "bafyinvalidnonexistentdid000000000000", 1, self.password),
            _REASON_INVALID_DID + _REASON_INSUFFICIENT,
        ))

        # (b) Non-positive amount.
        bal_a = self.node_a.get_balance(self.did_a)
        out.append(self._expect_rejection(
            "NEG_NON_POSITIVE_AMOUNT",
            lambda: self.node_a.transfer_rbt(self.did_a, self.did_b, -5, self.password),
            _REASON_BAD_AMOUNT + _REASON_INSUFFICIENT,
            unchanged={"nodeA_balance": (lambda: self.node_a.get_balance(self.did_a), bal_a)},
        ))

        # (c) Public-key lookup for a malformed DID. Must be rejected by the
        # handler's DID validation, BEFORE any IPFS resolution is attempted —
        # otherwise a junk DID would stall the request on a DHT lookup.
        out.append(self._expect_rejection(
            "NEG_PUBLIC_KEY_MALFORMED_DID",
            lambda: self.node_a.get_public_key("bafyinvalidnonexistentdid000000000000"),
            _REASON_INVALID_DID,
        ))
        return out
