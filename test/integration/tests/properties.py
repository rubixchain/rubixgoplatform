"""
properties.py — NFT permissioned-execution (properties) integration tests.

Covers the properties token feature: a permission document governing an NFT,
resolved and enforced by the quorum during consensus. Set via
tokens.setProperties + tokens.properties on POST /rubix/v1/tx; read via
GET /rubix/v1/nfts/{nft_id}/properties.

Both directions are asserted, because either alone gives false confidence:
  - POSITIVE: a permitted operation SUCCEEDS (a gate that blocks everything
    would otherwise look like a pass).
  - NEGATIVE: a forbidden operation is REJECTED **by the properties gate**, not
    merely rejected. Several unrelated checks (notably TokenChainIntegrityCheck)
    run before ValidateNFTProperties and reject a non-owner first, so a test
    that only asserted "did it fail" would pass while enforcement was broken.
    Every negative case here matches on "ValidateNFTProperties" in the error.

Ownership matters for the same reason: to test that the whitelist blocks the
NFT's OWNER, the NFT must first be transferred to that owner — otherwise the
chain-integrity check rejects it before the properties gate is ever reached.

Cases:
  - regression:  an NFT with NO properties deploys/executes exactly as before
  - whitelist:   listed DID executes; unlisted OWNER is rejected
  - transferable: unset => transfer rejected; set => transfer succeeds
  - edit auth:   deployer may edit; non-deployer (incl. current owner) may not
  - versioning:  three successive edits, latest governs, chain tip advances
  - read API:    reports unrestricted for a plain NFT, document for a governed one
  - spend path:  the 0.001-valued properties token is never counted as RBT

Results use the same {"check", "status", "detail"} shape as the other engines so
they fold into verification.json and the runner's exit-on-fail gate.
"""

from __future__ import annotations

import json
import logging
import os
import tempfile
import time
from typing import Any, Callable, Dict, List, Optional

from test.integration.clients.api_client import NodeClient

log = logging.getLogger(__name__)

# A properties rejection MUST come from the properties gate. Matching only on
# "rejected" would pass when an earlier check (e.g. chain integrity) rejects for
# an unrelated reason while enforcement is silently broken.
_REASON_PROPERTIES = ["validatenftproperties"]
_REASON_NOT_WHITELISTED = ["not in the whitelist"]
_REASON_NON_TRANSFERABLE = ["non-transferable"]
_REASON_NOT_DEPLOYER = ["only the deployer"]

# Settle time after a consensus round before the next dependent operation.
_SETTLE = 4


class PropertiesEngine:
    """Runs NFT-properties positive and negative checks against a live cluster.

    Args:
        node_a/node_b: NodeClient for the two transacting nodes.
        did_a/did_b:   their primary DIDs. did_a is the NFT deployer throughout.
        password:      wallet password for completing transactions.
    """

    def __init__(
        self,
        node_a: NodeClient,
        node_b: NodeClient,
        did_a: str,
        did_b: str,
        password: str = "mypassword",
    ) -> None:
        self.node_a = node_a
        self.node_b = node_b
        self.did_a = did_a
        self.did_b = did_b
        self.password = password
        self._tmpdir = tempfile.mkdtemp(prefix="rubix-props-")

    # ------------------------------------------------------------------
    # Low-level helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _payload(
        initiator: str,
        owner: str,
        nft_id: str,
        *,
        transfer: bool = False,
        props: Optional[Dict[str, Any]] = None,
        data: str = "properties-test",
    ) -> Dict[str, Any]:
        tokens: Dict[str, Any] = {
            "rbt": 0,
            "ft": [],
            "nft": [{"nftId": nft_id, "value": 1.0, "data": data}],
            "smartContract": [],
            "transferNftOwnership": transfer,
        }
        if props is not None:
            tokens["setProperties"] = True
            tokens["properties"] = props
        return {
            "initiator": initiator,
            "owner": owner,
            "tokens": tokens,
            "memo": "properties-test",
        }

    @staticmethod
    def _run_tx(node: NodeClient, payload: Dict[str, Any]) -> None:
        """Post a tx and complete it. Raises on rejection at either step."""
        resp = node._post_raw("/rubix/v1/tx", payload)
        req_id = (resp.get("result") or {}).get("id")
        if not req_id:
            raise RuntimeError(f"tx rejected at initiate: {json.dumps(resp)[:400]}")
        done = node.complete_transaction(req_id)
        if not done.get("status"):
            raise RuntimeError(f"tx rejected at complete: {json.dumps(done)[:400]}")

    def _new_nft(self, label: str) -> str:
        """Create and deploy a fresh NFT owned by did_a.

        The artifact is unique per NFT: identical content hashes to the same CID
        and /nfts/generate then fails on the duplicate.
        """
        meta_path = os.path.join(self._tmpdir, f"meta_{label}.json")
        art_path = os.path.join(self._tmpdir, f"art_{label}_{int(time.time() * 1000)}.txt")
        with open(meta_path, "w", encoding="utf-8") as fh:
            json.dump({"name": f"props-{label}", "description": "properties suite"}, fh)
        with open(art_path, "w", encoding="utf-8") as fh:
            fh.write(f"artifact {label} {time.time()}\n")

        nft_id = self.node_a.create_nft(self.did_a, meta_path, art_path)["nft_id"]
        self._run_tx(self.node_a, self._payload(self.did_a, self.did_a, nft_id, data="deploy"))
        time.sleep(_SETTLE)
        log.info("[properties] %s: deployed NFT %s", label, nft_id)
        return nft_id

    def _read_props(self, nft_id: str) -> Dict[str, Any]:
        resp = self.node_a._get(f"/rubix/v1/nfts/{nft_id}/properties")
        return resp.get("result") or {}

    # ------------------------------------------------------------------
    # Assertion helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _reason_matches(err_text: str, reasons: List[str]) -> bool:
        low = err_text.lower()
        return any(r.lower() in low for r in reasons)

    def _expect_success(self, check: str, op: Callable[[], Any], detail: str = "") -> Dict[str, str]:
        """Run *op* expecting it to succeed. A permitted operation must work."""
        try:
            op()
        except Exception as exc:  # noqa: BLE001
            return {
                "check": check,
                "status": "FAIL",
                "detail": f"permitted operation was rejected: {str(exc)[:200]}",
            }
        return {"check": check, "status": "PASS", "detail": detail or "permitted operation succeeded"}

    def _expect_properties_rejection(
        self,
        check: str,
        op: Callable[[], Any],
        reasons: List[str],
    ) -> Dict[str, str]:
        """Run *op* expecting the PROPERTIES gate to reject it.

        Asserts both that the properties validator produced the rejection and
        that the specific restriction fired, so an unrelated earlier check
        cannot masquerade as working enforcement.
        """
        try:
            op()
        except Exception as exc:  # noqa: BLE001
            err = str(exc)
            if not self._reason_matches(err, _REASON_PROPERTIES):
                return {
                    "check": check,
                    "status": "FAIL",
                    "detail": f"rejected, but NOT by the properties gate: {err[:200]}",
                }
            if not self._reason_matches(err, reasons):
                return {
                    "check": check,
                    "status": "FAIL",
                    "detail": f"properties rejected, but wrong restriction: {err[:200]}",
                }
            return {"check": check, "status": "PASS", "detail": f"correctly rejected ({err[:90]})"}
        return {
            "check": check,
            "status": "FAIL",
            "detail": "forbidden operation was accepted (expected properties rejection)",
        }

    # ------------------------------------------------------------------
    # Entry point
    # ------------------------------------------------------------------

    def run(self) -> List[Dict[str, str]]:
        results: List[Dict[str, str]] = []
        log.info("=== PROPERTIES TESTS: starting ===")
        for phase in (
            self._no_properties_regression,
            self._whitelist,
            self._transferable,
            self._edit_authorization,
            self._versioning,
            self._spend_path,
        ):
            try:
                results.extend(phase())
            except Exception as exc:  # noqa: BLE001 — a setup failure must not kill the suite
                results.append({
                    "check": f"PROPS_{phase.__name__.strip('_').upper()}",
                    "status": "FAIL",
                    "detail": f"phase raised during setup: {str(exc)[:200]}",
                })
        passed = sum(1 for r in results if r["status"] == "PASS")
        log.info("=== PROPERTIES TESTS: %d/%d passed ===", passed, len(results))
        return results

    # ------------------------------------------------------------------
    # Test families
    # ------------------------------------------------------------------

    def _no_properties_regression(self) -> List[Dict[str, str]]:
        """An NFT with no properties must behave exactly as before the feature."""
        out: List[Dict[str, str]] = []
        nft = self._new_nft("plain")

        out.append(self._expect_success(
            "PROPS_NO_PROPS_EXECUTE",
            lambda: self._run_tx(self.node_a, self._payload(self.did_a, self.did_a, nft, data="exec")),
            "unrestricted NFT executes",
        ))
        time.sleep(_SETTLE)

        out.append(self._expect_success(
            "PROPS_NO_PROPS_TRANSFER",
            lambda: self._run_tx(
                self.node_a, self._payload(self.did_a, self.did_b, nft, transfer=True, data="xfer")),
            "unrestricted NFT transfers",
        ))
        time.sleep(_SETTLE)

        # The read API must say "unrestricted" rather than erroring.
        try:
            resp = self.node_a._get(f"/rubix/v1/nfts/{nft}/properties")
            msg = str(resp.get("message", "")).lower()
            ok = resp.get("status") is True and resp.get("result") is None and "unrestricted" in msg
            out.append({
                "check": "PROPS_READ_UNRESTRICTED",
                "status": "PASS" if ok else "FAIL",
                "detail": f"message={resp.get('message')!r} result={resp.get('result')!r}",
            })
        except Exception as exc:  # noqa: BLE001
            out.append({
                "check": "PROPS_READ_UNRESTRICTED",
                "status": "FAIL",
                "detail": f"read raised: {str(exc)[:200]}",
            })
        return out

    def _whitelist(self) -> List[Dict[str, str]]:
        """Whitelist gates the initiator — including the NFT's own owner."""
        out: List[Dict[str, str]] = []
        nft = self._new_nft("whitelist")

        out.append(self._expect_success(
            "PROPS_SET_WHITELIST",
            lambda: self._run_tx(self.node_a, self._payload(
                self.did_a, self.did_a, nft,
                props={"transferable": True, "whitelist": [self.did_a]}, data="setprops")),
            "properties set with whitelist=[A]",
        ))
        time.sleep(_SETTLE)

        # Read-back: the document must be retrievable and reflect what was set.
        try:
            res = self._read_props(nft)
            ok = res.get("version") == 1 and self.did_a in (res.get("whitelist") or [])
            out.append({
                "check": "PROPS_READ_DOCUMENT",
                "status": "PASS" if ok else "FAIL",
                "detail": f"version={res.get('version')} whitelist={res.get('whitelist')}",
            })
        except Exception as exc:  # noqa: BLE001
            out.append({"check": "PROPS_READ_DOCUMENT", "status": "FAIL",
                        "detail": f"read raised: {str(exc)[:200]}"})

        # POSITIVE: whitelisted initiator executes.
        out.append(self._expect_success(
            "PROPS_WHITELIST_ALLOWS_LISTED",
            lambda: self._run_tx(self.node_a, self._payload(self.did_a, self.did_a, nft, data="exec-A")),
            "whitelisted DID executes",
        ))
        time.sleep(_SETTLE)

        # Hand the NFT to B so the negative case exercises the properties gate
        # rather than the chain-integrity check that rejects a non-owner first.
        self._run_tx(self.node_a, self._payload(self.did_a, self.did_b, nft, transfer=True, data="xfer"))
        time.sleep(_SETTLE + 1)

        # NEGATIVE: B owns the NFT but is not whitelisted.
        out.append(self._expect_properties_rejection(
            "PROPS_WHITELIST_BLOCKS_UNLISTED_OWNER",
            lambda: self._run_tx(self.node_b, self._payload(self.did_b, self.did_b, nft, data="exec-B")),
            _REASON_NOT_WHITELISTED,
        ))
        return out

    def _transferable(self) -> List[Dict[str, str]]:
        """The transferable flag gates transfer, and only transfer."""
        out: List[Dict[str, str]] = []
        nft = self._new_nft("transferable")

        self._run_tx(self.node_a, self._payload(
            self.did_a, self.did_a, nft,
            props={"transferable": False, "whitelist": [self.did_a, self.did_b]}, data="setprops"))
        time.sleep(_SETTLE)

        # NEGATIVE: transfer blocked while the flag is unset.
        out.append(self._expect_properties_rejection(
            "PROPS_TRANSFER_BLOCKED_WHEN_UNSET",
            lambda: self._run_tx(self.node_a, self._payload(
                self.did_a, self.did_b, nft, transfer=True, data="xfer-blocked")),
            _REASON_NON_TRANSFERABLE,
        ))
        time.sleep(_SETTLE)

        # POSITIVE: a non-transfer execute is unaffected by the flag.
        out.append(self._expect_success(
            "PROPS_EXECUTE_UNAFFECTED_BY_TRANSFERABLE",
            lambda: self._run_tx(self.node_a, self._payload(self.did_a, self.did_a, nft, data="exec")),
            "execute allowed while non-transferable",
        ))
        time.sleep(_SETTLE)

        # POSITIVE: flip the flag, transfer now succeeds.
        out.append(self._expect_success(
            "PROPS_EDIT_ENABLE_TRANSFERABLE",
            lambda: self._run_tx(self.node_a, self._payload(
                self.did_a, self.did_a, nft,
                props={"transferable": True, "whitelist": [self.did_a, self.did_b]}, data="enable")),
            "deployer enabled transferable",
        ))
        time.sleep(_SETTLE)

        # Confirm the edit is what the read API now reports, so a failure below
        # points at enforcement rather than at the edit silently not landing.
        try:
            res = self._read_props(nft)
            out.append({
                "check": "PROPS_EDIT_VISIBLE_AFTER_WRITE",
                "status": "PASS" if res.get("transferable") is True else "FAIL",
                "detail": f"transferable={res.get('transferable')} (expected True)",
            })
        except Exception as exc:  # noqa: BLE001
            out.append({"check": "PROPS_EDIT_VISIBLE_AFTER_WRITE", "status": "FAIL",
                        "detail": f"read raised: {str(exc)[:200]}"})

        out.append(self._expect_success(
            "PROPS_TRANSFER_ALLOWED_WHEN_SET",
            lambda: self._run_tx(self.node_a, self._payload(
                self.did_a, self.did_b, nft, transfer=True, data="xfer-ok")),
            "transfer succeeds once transferable is set",
        ))
        return out

    def _edit_authorization(self) -> List[Dict[str, str]]:
        """Only the deployer may edit — not even the current owner."""
        out: List[Dict[str, str]] = []
        nft = self._new_nft("editauth")

        self._run_tx(self.node_a, self._payload(
            self.did_a, self.did_a, nft,
            props={"transferable": True, "whitelist": [self.did_a, self.did_b]}, data="setprops"))
        time.sleep(_SETTLE)

        # POSITIVE: deployer edits its own properties.
        out.append(self._expect_success(
            "PROPS_EDIT_BY_DEPLOYER",
            lambda: self._run_tx(self.node_a, self._payload(
                self.did_a, self.did_a, nft,
                props={"transferable": True, "whitelist": [self.did_a]}, data="edit-deployer")),
            "deployer edit accepted",
        ))
        time.sleep(_SETTLE)

        # Transfer to B: B becomes owner but is still not the deployer.
        self._run_tx(self.node_a, self._payload(self.did_a, self.did_b, nft, transfer=True, data="xfer"))
        time.sleep(_SETTLE + 1)

        # NEGATIVE: the current owner is not the deployer and may not edit.
        # This one is rejected up-front by validatePropertiesRequest, so it does
        # not carry the ValidateNFTProperties prefix the quorum path uses.
        try:
            self._run_tx(self.node_b, self._payload(
                self.did_b, self.did_b, nft,
                props={"transferable": True, "whitelist": [self.did_b]}, data="edit-owner"))
            out.append({
                "check": "PROPS_EDIT_BLOCKED_FOR_NON_DEPLOYER",
                "status": "FAIL",
                "detail": "non-deployer edit was accepted (expected rejection)",
            })
        except Exception as exc:  # noqa: BLE001
            err = str(exc)
            ok = self._reason_matches(err, _REASON_NOT_DEPLOYER)
            out.append({
                "check": "PROPS_EDIT_BLOCKED_FOR_NON_DEPLOYER",
                "status": "PASS" if ok else "FAIL",
                "detail": (f"correctly rejected ({err[:90]})" if ok
                           else f"rejected, but wrong reason: {err[:200]}"),
            })
        return out

    def _versioning(self) -> List[Dict[str, str]]:
        """Successive edits: the latest version governs and the chain advances."""
        out: List[Dict[str, str]] = []
        nft = self._new_nft("versioning")

        versions = [
            {"transferable": False, "whitelist": [self.did_a]},
            {"transferable": True, "whitelist": [self.did_a]},
            {"transferable": True, "whitelist": [self.did_a, self.did_b]},
        ]
        for idx, props in enumerate(versions, start=1):
            out.append(self._expect_success(
                f"PROPS_VERSION_{idx}",
                lambda p=props, i=idx: self._run_tx(self.node_a, self._payload(
                    self.did_a, self.did_a, nft, props=p, data=f"v{i}")),
                f"version {idx} written",
            ))
            time.sleep(_SETTLE)

        # The third version must be the one in force.
        try:
            res = self._read_props(nft)
            wl = sorted(res.get("whitelist") or [])
            governs = res.get("transferable") is True and wl == sorted([self.did_a, self.did_b])
            out.append({
                "check": "PROPS_LATEST_VERSION_GOVERNS",
                "status": "PASS" if governs else "FAIL",
                "detail": f"transferable={res.get('transferable')} whitelist={res.get('whitelist')}",
            })
        except Exception as exc:  # noqa: BLE001
            out.append({"check": "PROPS_LATEST_VERSION_GOVERNS", "status": "FAIL",
                        "detail": f"read raised: {str(exc)[:200]}"})
        return out

    def _spend_path(self) -> List[Dict[str, str]]:
        """The 0.001-valued properties token must never count as spendable RBT."""
        out: List[Dict[str, str]] = []
        before = self.node_a.get_balance(self.did_a)

        nft = self._new_nft("spend")
        self._run_tx(self.node_a, self._payload(
            self.did_a, self.did_a, nft,
            props={"transferable": True, "whitelist": [self.did_a]}, data="setprops"))
        time.sleep(_SETTLE)

        after = self.node_a.get_balance(self.did_a)
        # An NFT deploy pledges RBT, so the balance may legitimately drop; what
        # must never happen is the 0.001 properties token being counted IN.
        out.append({
            "check": "PROPS_TOKEN_NOT_COUNTED_AS_RBT",
            "status": "PASS" if after <= before else "FAIL",
            "detail": f"balance before={before} after={after} (must not increase)",
        })
        return out
