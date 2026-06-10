"""
api_client.py — HTTP wrapper for all Rubix node API endpoints.

All methods raise RuntimeError on non-200 responses or when the server
returns {"status": false, ...}.
"""

import logging
import os
import time
from typing import Any, Dict, Optional

import requests

log = logging.getLogger(__name__)


class NodeClient:
    """HTTP client for a single Rubix node.

    Args:
        base_url: e.g. "http://localhost:20000"
        name:     Label used in log messages.
    """

    def __init__(self, base_url: str, name: str = "node", password: str = "mypassword") -> None:
        self.base_url = base_url.rstrip("/")
        self.name = name
        self.password = password

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    # Most calls (incl. setup-quorum) return quickly. But heavy consensus
    # operations can occasionally run long when the host is under load, and a
    # short timeout fails them prematurely while the node is still working.
    # A generous timeout costs nothing on the fast path (fast calls return
    # immediately regardless) and keeps CI from flaking on a loaded runner.
    _POST_TIMEOUT = 120
    _GET_TIMEOUT = 60

    def _post_raw(self, path: str, json_data: Any = None) -> Dict[str, Any]:
        """POST to *path*, return parsed JSON.  Does NOT handle password challenges."""
        url = self.base_url + path
        log.debug("[%s] POST %s  body=%s", self.name, url, json_data)
        resp = requests.post(url, json=json_data, timeout=self._POST_TIMEOUT)
        if resp.status_code != 200:
            raise RuntimeError(
                f"{self.name}: POST {path} returned HTTP {resp.status_code}: {resp.text}"
            )
        resp_json: Dict[str, Any] = resp.json()
        if not resp_json.get("status"):
            raise RuntimeError(f"{self.name}: {path} failed: {resp_json}")
        return resp_json

    def _post(self, path: str, json_data: Any = None) -> Dict[str, Any]:
        """POST to *path*, automatically completing password challenges via self.password.

        If the server returns ``{"status": true, "message": "Password needed", ...}``
        this method extracts the request ID and immediately POSTs the password to
        ``/rubix/v1/signature``, then returns the final response.
        """
        resp = self._post_raw(path, json_data)
        if resp.get("message") == "Password needed":
            req_id: str = resp["result"]["id"]
            log.debug("[%s] Password challenge on %s, req_id=%s", self.name, path, req_id)
            resp = self._post_raw(
                "/rubix/v1/signature",
                {"id": req_id, "password": self.password, "signature": ""},
            )
        return resp

    def _get(self, path: str) -> Dict[str, Any]:
        """GET *path* (relative), return parsed JSON.  Raises on failure."""
        url = self.base_url + path
        log.debug("[%s] GET %s", self.name, url)
        resp = requests.get(url, timeout=self._GET_TIMEOUT)
        if resp.status_code != 200:
            raise RuntimeError(
                f"{self.name}: GET {path} returned HTTP {resp.status_code}: {resp.text}"
            )
        resp_json: Dict[str, Any] = resp.json()
        if not resp_json.get("status"):
            raise RuntimeError(f"{self.name}: {path} failed: {resp_json}")
        return resp_json

    def _post_multipart(
        self, path: str, files: Dict[str, Any], data: Dict[str, str]
    ) -> Dict[str, Any]:
        """POST multipart/form-data with files. Does NOT handle password challenges.

        Args:
            path: API endpoint (e.g., "/rubix/v1/nfts/generate")
            files: {"field_name": (filename, file_object)} for file uploads
            data: {"field_name": "value"} for regular form fields

        Returns:
            Parsed JSON response
        """
        url = self.base_url + path
        log.debug("[%s] POST %s (multipart)  data=%s", self.name, url, data)
        resp = requests.post(url, files=files, data=data, timeout=60)
        if resp.status_code != 200:
            raise RuntimeError(
                f"{self.name}: POST {path} returned HTTP {resp.status_code}: {resp.text}"
            )
        resp_json: Dict[str, Any] = resp.json()
        if not resp_json.get("status"):
            raise RuntimeError(f"{self.name}: {path} failed: {resp_json}")
        return resp_json

    # ------------------------------------------------------------------
    # DID management
    # ------------------------------------------------------------------

    def create_did(self, password: str = "mypassword") -> Dict[str, Any]:
        """Create a new DID on this node using local key generation.

        Returns:
            The "result" dict containing "did" and "peer_id".
        """
        log.info("[%s] Creating DID…", self.name)
        resp = self._post(
            "/rubix/v1/dids/create",
            {
                "password": password,
                "public_key": "",
                "private_key": "",
                "mnemonic": "",
                "childPath": 0,
            },
        )
        result = resp.get("result", {})
        log.info("[%s] DID created: %s", self.name, result.get("did"))
        return result

    def register_did(self, did: str) -> Dict[str, Any]:
        """Register *did* with the node's network layer."""
        log.info("[%s] Registering DID %s…", self.name, did)
        return self._post(f"/rubix/v1/dids/{did}/register", {})

    # ------------------------------------------------------------------
    # Peer management
    # ------------------------------------------------------------------

    def get_peer_id(self) -> str:
        """Return the node's IPFS peer ID string."""
        resp = self._get("/api/get-peer-id")
        peer_id: str = resp["message"]
        log.info("[%s] Peer ID: %s", self.name, peer_id)
        return peer_id

    def add_peer_details(self, peer_id: str, did: str) -> Dict[str, Any]:
        """Register a remote peer's DID ↔ PeerID mapping on this node.

        NOTE: Field names are Capitalised — the Go struct has no json tags.
        """
        log.info("[%s] Adding peer: did=%s peer_id=%s", self.name, did, peer_id)
        return self._post("/api/add-peer-details", {"DID": did, "PeerID": peer_id})

    # ------------------------------------------------------------------
    # Quorum configuration
    # ------------------------------------------------------------------

    def add_quorum(self, quorum_did: str) -> Dict[str, Any]:
        """Tell this node to use *quorum_did* as its quorum member.

        NOTE: Payload is a single "did" string, NOT an array.
        """
        log.info("[%s] Adding quorum DID: %s", self.name, quorum_did)
        return self._post("/api/addquorum", {"did": quorum_did})

    def setup_quorum(self, did: str, password: str = "mypassword") -> Dict[str, Any]:
        """Set up this node AS a quorum member using *did*."""
        log.info("[%s] Setting up quorum for DID: %s", self.name, did)
        return self._post(
            "/api/setup-quorum",
            {"did": did, "password": password, "priv_password": password},
        )

    # ------------------------------------------------------------------
    # Tokens
    # ------------------------------------------------------------------

    def generate_local_rbt(
        self,
        did: str,
        number_of_tokens: int,
        start_index: int,
        expected_total: Optional[float] = None,
    ) -> None:
        """Premint *number_of_tokens* localnet RBT tokens starting at *start_index*.

        The integration harness is localnet-only: this is the ONLY token
        generation path. Server-side flow is async — the password challenge is
        handled by _post, then this polls get_balance() until the balance
        reaches *expected_total*.

        IMPORTANT: this is called once PER BATCH. The skip/poll must compare the
        balance against the CUMULATIVE expected balance after this batch
        (`expected_total`), NOT the per-batch `number_of_tokens` — otherwise the
        first batch satisfies `balance >= number_of_tokens` and every later
        batch is wrongly skipped (only one batch ever mints). When the caller
        doesn't pass a cumulative target, fall back to the per-batch count.
        """
        target = float(expected_total) if expected_total is not None else float(number_of_tokens)

        # Skip only if we already hold the full cumulative target for this batch.
        try:
            balance = self.get_balance(did)
            if balance >= target:
                log.info(
                    "[%s] Balance already at target (%.4f >= %.4f), skipping this batch",
                    self.name, balance, target,
                )
                return
        except Exception as exc:  # noqa: BLE001
            log.warning("[%s] Initial balance check failed: %s, proceeding with mint", self.name, exc)

        log.info(
            "[%s] Generating %d localnet RBT tokens for DID %s (start_index=%d, target=%.0f)…",
            self.name, number_of_tokens, did, start_index, target,
        )
        self._post(
            "/api/generate-local-rbt",
            {"number_of_tokens": number_of_tokens, "did": did, "start_index": start_index},
        )

        deadline = time.time() + 120
        while time.time() < deadline:
            try:
                balance = self.get_balance(did)
                log.info("[%s] Balance poll: %.4f / %.0f", self.name, balance, target)
                if balance >= target:
                    log.info("[%s] Token generation complete (balance=%.4f).", self.name, balance)
                    return
            except Exception as exc:  # noqa: BLE001
                log.warning("[%s] Balance poll error: %s", self.name, exc)
            time.sleep(10)

        raise RuntimeError(
            f"{self.name}: Tokens did not appear within 120 s after generate-local-rbt"
        )

    def get_balance(self, did: str) -> float:
        """Return the RBT balance for *did* as a float."""
        resp = self._get(f"/rubix/v1/dids/{did}/balances/rbt")
        result = resp.get("result", {})
        log.debug("[%s] Raw balance result: %s", self.name, result)
        if isinstance(result, dict):
            return float(result.get("balance", 0))
        return float(result)

    # ------------------------------------------------------------------
    # Transactions (2-step flow)
    # ------------------------------------------------------------------

    def initiate_transaction(
        self, sender_did: str, receiver_did: str, rbt_amount: float
    ) -> str:
        """Step 1: Initiate a transfer.  Returns the request ID for step 2."""
        log.info(
            "[%s] Initiating transfer: %s -> %s  amount=%.4f",
            self.name,
            sender_did,
            receiver_did,
            rbt_amount,
        )
        # Use _post_raw: the "Password needed" response IS the challenge carrying req_id.
        # complete_transaction then supplies the password explicitly.
        resp = self._post_raw(
            "/rubix/v1/tx",
            {
                "initiator": sender_did,
                "owner": receiver_did,
                "tokens": {"rbt": rbt_amount},
                "memo": "",
            },
        )
        req_id: str = resp["result"]["id"]
        log.info("[%s] Transaction initiated, req_id=%s", self.name, req_id)
        return req_id

    def complete_transaction(
        self, req_id: str, password: str = "mypassword"
    ) -> Dict[str, Any]:
        """Step 2: Provide the signature to complete a pending transaction."""
        log.info("[%s] Completing transaction req_id=%s", self.name, req_id)
        resp = self._post(
            "/rubix/v1/signature",
            {"id": req_id, "password": password, "signature": ""},
        )
        log.info("[%s] Transaction completed: %s", self.name, resp)
        return resp

    @staticmethod
    def extract_txn_id(result: Optional[Dict[str, Any]]) -> Optional[str]:
        """Pull the on-chain transactionID out of a {"req_id", "response"} result.

        The completed-transaction response looks like
        ``{"status": True, "message": ..., "result": {"transactionID": "<hash>"}}``.
        Returns None if the result/response is missing or malformed (e.g. a failed tx).
        """
        if not result:
            return None
        resp = result.get("response") or {}
        inner = resp.get("result") or {}
        return inner.get("transactionID") or inner.get("transaction_id")

    def transfer_rbt(
        self,
        sender_did: str,
        receiver_did: str,
        amount: float,
        password: str = "mypassword",
    ) -> Dict[str, Any]:
        """Convenience: initiate + complete a transfer in one call.

        Returns:
            {"req_id": <str>, "response": <dict>}
        """
        req_id = self.initiate_transaction(sender_did, receiver_did, amount)
        response = self.complete_transaction(req_id, password)
        return {"req_id": req_id, "response": response}

    # ------------------------------------------------------------------
    # NFT operations
    # ------------------------------------------------------------------

    def initiate_nft_creation(
        self, did: str, metadata_path: str, artifact_path: str
    ) -> str:
        """Step 1: Upload NFT files and create NFT. Returns NFT ID directly.

        NOTE: Unlike other APIs, NFT creation does NOT use the password challenge pattern.
        The NFT is created immediately and the NFT ID is returned in the response.

        Args:
            did: DID that will own the NFT
            metadata_path: Path to JSON metadata file
            artifact_path: Path to artifact file (image, video, etc.)

        Returns:
            NFT token hash (e.g., "QmVudz77J43...")
        """
        log.info(
            "[%s] Initiating NFT creation for DID %s: metadata=%s artifact=%s",
            self.name,
            did,
            os.path.basename(metadata_path),
            os.path.basename(artifact_path),
        )

        # Open files and prepare multipart upload
        with open(metadata_path, "rb") as metadata_file, open(
            artifact_path, "rb"
        ) as artifact_file:
            files = {
                "metadata": (os.path.basename(metadata_path), metadata_file),
                "artifact": (os.path.basename(artifact_path), artifact_file),
            }
            data = {"did": did}

            # NFT creation returns the NFT ID directly in result (no password challenge)
            resp = self._post_multipart("/rubix/v1/nfts/generate", files, data)

        nft_id: str = resp["result"]
        log.info("[%s] NFT created successfully: %s", self.name, nft_id)
        return nft_id

    def create_nft(
        self,
        did: str,
        metadata_path: str,
        artifact_path: str,
        password: str = "mypassword",
    ) -> Dict[str, Any]:
        """Convenience: Upload NFT files and create NFT.

        NOTE: Unlike RBT transfers, NFT creation does NOT require a password challenge.
        The NFT is created immediately when files are uploaded.

        Args:
            did: DID that will own the NFT
            metadata_path: Path to JSON metadata file
            artifact_path: Path to artifact file (image, video, etc.)
            password: Unused (kept for API compatibility)

        Returns:
            {
                "nft_id": <str>,  # NFT token hash (e.g., "Qm...")
                "metadata": <str>,  # Metadata file path
                "artifact": <str>   # Artifact file path
            }
        """
        nft_id = self.initiate_nft_creation(did, metadata_path, artifact_path)
        return {
            "nft_id": nft_id,
            "metadata": metadata_path,
            "artifact": artifact_path,
        }

    def initiate_nft_transaction(
        self,
        sender_did: str,
        receiver_did: str,
        nft_id: str,
        nft_value: float = 1.0,
        data: str = "NFT deployment - initial transaction",
        transfer_ownership: bool = False,
    ) -> str:
        """Step 1: Initiate NFT transfer/deployment. Returns request ID for step 2.

        Args:
            sender_did: DID initiating the NFT transfer
            receiver_did: DID receiving the NFT (can be same as sender for deployment)
            nft_id: NFT token hash (e.g., "QmVudz77J43atukvn51TzNAfWqS66r3YwLd8S7t745vHv5")
            nft_value: NFT value (default: 1.0)
            data: Transaction description
            transfer_ownership: If True, transfer NFT ownership to receiver;
                              If False, execute NFT without ownership change

        Returns:
            Request ID for signature step
        """
        log.info(
            "[%s] Initiating NFT transaction: %s -> %s  nft=%s  transfer_ownership=%s",
            self.name,
            sender_did[:20] + "...",
            receiver_did[:20] + "...",
            nft_id,
            transfer_ownership,
        )

        # Build NFT transaction payload
        payload = {
            "initiator": sender_did,
            "owner": receiver_did,
            "tokens": {
                "rbt": 0,
                "ft": [],
                "nft": [
                    {
                        "nftId": nft_id,
                        "value": nft_value,
                        "data": data,
                    }
                ],
                "smartContract": [],
                "transferNftOwnership": transfer_ownership,
            },
            "memo": f"NFT {nft_id[:8]}... transfer",
        }

        # Use _post_raw: the "Password needed" response carries req_id
        resp = self._post_raw("/rubix/v1/tx", payload)
        req_id: str = resp["result"]["id"]
        log.info("[%s] NFT transaction initiated, req_id=%s", self.name, req_id)
        return req_id

    def transfer_nft(
        self,
        sender_did: str,
        receiver_did: str,
        nft_id: str,
        nft_value: float = 1.0,
        data: str = "NFT deployment - initial transaction",
        password: str = "mypassword",
    ) -> Dict[str, Any]:
        """Convenience: Initiate + complete NFT transfer in one call.

        Args:
            sender_did: DID initiating the NFT transfer
            receiver_did: DID receiving the NFT (can be same as sender for deployment)
            nft_id: NFT token hash
            nft_value: NFT value (default: 1.0)
            data: Transaction description
            password: Password for signature

        Returns:
            {"req_id": <str>, "response": <dict>}
        """
        req_id = self.initiate_nft_transaction(
            sender_did, receiver_did, nft_id, nft_value, data, transfer_ownership=False
        )
        response = self.complete_transaction(req_id, password)
        return {"req_id": req_id, "response": response}

    def mint_nft_children(
        self,
        initiator_did: str,
        parent_nft_id: str,
        number_of_children: int,
        data: str = "mint child NFTs",
        password: str = "mypassword",
    ) -> Dict[str, Any]:
        """Mint *number_of_children* child NFTs under *parent_nft_id*.

        Uses the standard transaction endpoint (POST /rubix/v1/tx) with the
        child-minting signal in the nft token spec: ``parentNFTId`` +
        ``numberOfChildren``. The completed response carries
        ``result.mintedNFTChildren`` (list of {parentNFTId, childNFTId}) and
        ``result.transactionID``.

        Returns:
            {"req_id": <str>, "response": <dict>} — same shape as transfer_nft;
            extract_txn_id() works, and response["result"]["mintedNFTChildren"]
            lists the created children.
        """
        log.info(
            "[%s] Minting %d child NFT(s) under parent %s for %s",
            self.name, number_of_children, parent_nft_id[:12] + "...",
            initiator_did[:20] + "...",
        )
        # One NFT entry per child: ParentNFTId signals a child-mint and the server
        # derives the child id via IPFS (NFTId is IGNORED when ParentNFTId is set,
        # so it must be omitted — supplying the parent id there makes the server
        # reject it as "already exists"). See types/models NFTInfo + core/nft_child_mint.go.
        nft_entries = [
            {
                "value": 0,
                "data": data,
                "parentNFTId": parent_nft_id,
            }
            for _ in range(number_of_children)
        ]
        payload = {
            "initiator": initiator_did,
            "tokens": {
                "rbt": 0,
                "ft": [],
                "nft": nft_entries,
                "smartContract": [],
                "transferNftOwnership": False,
            },
            "memo": f"mint {number_of_children} children of {parent_nft_id[:8]}...",
        }
        resp = self._post_raw("/rubix/v1/tx", payload)
        req_id: str = resp["result"]["id"]
        log.info("[%s] Child-NFT mint initiated, req_id=%s", self.name, req_id)
        response = self.complete_transaction(req_id, password)
        return {"req_id": req_id, "response": response}

    @staticmethod
    def extract_minted_children(result: Optional[Dict[str, Any]]) -> list:
        """Pull result.mintedNFTChildren (list of {parentNFTId, childNFTId}) from
        a {"req_id", "response"} mint result. Returns [] if absent."""
        if not result:
            return []
        resp = result.get("response") or {}
        inner = resp.get("result") or {}
        return inner.get("mintedNFTChildren") or []

    def execute_nft(
        self,
        executor_did: str,
        nft_id: str,
        nft_value: float = 1.0,
        data: str = "NFT self-execution",
        password: str = "mypassword",
    ) -> Dict[str, Any]:
        """Convenience: Execute NFT without transferring ownership (self-execution).

        This is used when the owner wants to execute/use their NFT
        without transferring it to another party.

        Args:
            executor_did: DID of the NFT owner executing the transaction
            nft_id: NFT token hash
            nft_value: NFT value (default: 1.0)
            data: Transaction description
            password: Password for signature

        Returns:
            {"req_id": <str>, "response": <dict>}
        """
        req_id = self.initiate_nft_transaction(
            sender_did=executor_did,
            receiver_did=executor_did,  # Same DID for self-execution
            nft_id=nft_id,
            nft_value=nft_value,
            data=data,
            transfer_ownership=False,
        )
        response = self.complete_transaction(req_id, password)
        return {"req_id": req_id, "response": response}

    def transfer_nft_ownership(
        self,
        sender_did: str,
        receiver_did: str,
        nft_id: str,
        nft_value: float = 1.0,
        data: str = "NFT ownership transfer",
        password: str = "mypassword",
    ) -> Dict[str, Any]:
        """Convenience: Execute NFT and transfer ownership to new owner.

        Args:
            sender_did: DID of current NFT owner
            receiver_did: DID of new NFT owner
            nft_id: NFT token hash
            nft_value: NFT value (default: 1.0)
            data: Transaction description
            password: Password for signature

        Returns:
            {"req_id": <str>, "response": <dict>}
        """
        req_id = self.initiate_nft_transaction(
            sender_did=sender_did,
            receiver_did=receiver_did,
            nft_id=nft_id,
            nft_value=nft_value,
            data=data,
            transfer_ownership=True,
        )
        response = self.complete_transaction(req_id, password)
        return {"req_id": req_id, "response": response}

    def subscribe_nft(self, nft_id: str) -> Dict[str, Any]:
        """Subscribe to an NFT to enable cross-node execution.

        Args:
            nft_id: NFT token hash

        Returns:
            API response
        """
        log.info("[%s] Subscribing to NFT: %s", self.name, nft_id)
        resp = self._get(f"/rubix/v1/nfts/subscribe?nft={nft_id}")
        log.info("[%s] Successfully subscribed to NFT", self.name)
        return resp

    def list_nfts(self) -> list:
        """List all NFTs stored in this node's database.

        Returns:
            List of NFT token records.
        """
        log.info("[%s] Listing all NFTs", self.name)
        resp = self._get("/rubix/v1/nfts")
        result = resp.get("result", [])
        log.info("[%s] Found %d NFTs", self.name, len(result) if result else 0)
        return result or []

    def get_nft_chain(self, nft_id: str) -> list:
        """Get the ordered transaction chain for a specific NFT.

        Args:
            nft_id: NFT token ID

        Returns:
            List of chain entries (transactionId, initiator, epoch, data).
        """
        log.info("[%s] Getting NFT chain for: %s", self.name, nft_id)
        resp = self._get(f"/rubix/v1/nfts/{nft_id}/chain")
        result = resp.get("result", [])
        log.info("[%s] NFT chain has %d entries", self.name, len(result) if result else 0)
        return result or []

    def get_nft_children(self, nft_id: str) -> list:
        """Get child NFTs of *nft_id* (GET /rubix/v1/nfts/{nft_id}/children).

        Returns the children list (empty when the NFT has no children).
        """
        log.info("[%s] Getting NFT children for: %s", self.name, nft_id)
        resp = self._get(f"/rubix/v1/nfts/{nft_id}/children")
        result = resp.get("result", [])
        log.info("[%s] NFT children: %d", self.name, len(result) if result else 0)
        return result or []

    def get_nft_parent(self, nft_id: str) -> Dict[str, Any]:
        """Get the parent NFT of *nft_id* (GET /rubix/v1/nfts/{nft_id}/parent).

        Returns the full {status, message, result} response; result is null when
        the NFT has no parent.
        """
        log.info("[%s] Getting NFT parent for: %s", self.name, nft_id)
        resp = self._get(f"/rubix/v1/nfts/{nft_id}/parent")
        return resp

    def fetch_nft(self, nft_id: str) -> Dict[str, Any]:
        """Download an NFT from IPFS if not already present locally.

        Args:
            nft_id: NFT token ID (46 chars, starts with Qm)

        Returns:
            API response
        """
        log.info("[%s] Fetching NFT from IPFS: %s", self.name, nft_id)
        resp = self._get(f"/rubix/v1/fetch-nft?nft={nft_id}")
        log.info("[%s] NFT fetched successfully", self.name)
        return resp

    def get_nft_balance(self, did: str) -> list:
        """Get NFTs owned by a specific DID (Free status only).

        Args:
            did: DID to query

        Returns:
            List of NFT records owned by the DID.
        """
        log.info("[%s] Getting NFT balance for DID: %s", self.name, did[:20] + "...")
        resp = self._get(f"/rubix/v1/dids/{did}/balances/nft")
        result = resp.get("result", [])
        log.info("[%s] DID owns %d NFTs", self.name, len(result) if result else 0)
        return result or []

    # ------------------------------------------------------------------
    # Smart Contract methods
    # ------------------------------------------------------------------

    def initiate_smart_contract_generation(
        self, did: str, wasm_path: str, source_path: str
    ) -> Dict[str, Any]:
        """Generate a smart contract by uploading WASM binary and source code.

        Args:
            did: DID that will own the smart contract
            wasm_path: Path to .wasm binary file
            source_path: Path to .rs source code file

        Returns:
            {"smartContractId": <str>}
        """
        log.info(
            "[%s] Initiating smart contract generation for DID %s: wasm=%s source=%s",
            self.name,
            did[:20] + "...",
            wasm_path.split("/")[-1],
            source_path.split("/")[-1],
        )

        data = {
            "did": did,
        }

        files = {
            "binaryCodePath": (wasm_path.split("/")[-1], open(wasm_path, "rb")),
            "rawCodePath": (source_path.split("/")[-1], open(source_path, "rb")),
        }

        resp = self._post_multipart("/rubix/v1/smart_contracts/generate", files, data)
        sc_id = resp["result"]

        log.info("[%s] Smart contract generated successfully: %s", self.name, sc_id)
        return {"smartContractId": sc_id}

    def create_smart_contract(
        self, did: str, wasm_path: str, source_path: str
    ) -> Dict[str, Any]:
        """Convenience: Generate smart contract in one call.

        Returns:
            {"smartContractId": <str>}
        """
        return self.initiate_smart_contract_generation(did, wasm_path, source_path)

    def initiate_smart_contract_transaction(
        self,
        initiator_did: str,
        sc_id: str,
        sc_value: float = 1.0,
        data: str = "test_execution",
    ) -> str:
        """Step 1: Initiate smart contract transaction. Returns request ID for step 2.

        Args:
            initiator_did: DID initiating the smart contract transaction
            sc_id: Smart contract token hash
            sc_value: Smart contract value (default: 1.0)
            data: Execution data/function name

        Returns:
            Request ID for signature step
        """
        log.info(
            "[%s] Initiating smart contract transaction: initiator=%s  sc=%s  data=%s",
            self.name,
            initiator_did[:20] + "...",
            sc_id,
            data,
        )

        # Build smart contract transaction payload
        payload = {
            "initiator": initiator_did,
            "owner": "",  # Always empty for smart contracts
            "tokens": {
                "rbt": 0,
                "ft": [],
                "nft": [],
                "smartContract": [
                    {
                        "smartContractId": sc_id,
                        "value": sc_value,
                        "data": data,
                    }
                ],
                "transferNftOwnership": False,
            },
            "memo": f"Smart contract {sc_id[:8]}... execution",
        }

        # Use _post_raw: the "Password needed" response carries req_id
        resp = self._post_raw("/rubix/v1/tx", payload)
        req_id: str = resp["result"]["id"]
        log.info("[%s] Smart contract transaction initiated, req_id=%s", self.name, req_id)
        return req_id

    def deploy_smart_contract(
        self,
        initiator_did: str,
        sc_id: str,
        data: str = "deployment",
        password: str = "mypassword",
    ) -> Dict[str, Any]:
        """Convenience: Deploy smart contract (first execution).

        Args:
            initiator_did: DID deploying the smart contract
            sc_id: Smart contract token hash
            data: Deployment data
            password: Password for signature

        Returns:
            {"req_id": <str>, "response": <dict>}
        """
        req_id = self.initiate_smart_contract_transaction(
            initiator_did=initiator_did,
            sc_id=sc_id,
            sc_value=1.0,
            data=data,
        )
        response = self.complete_transaction(req_id, password)
        return {"req_id": req_id, "response": response}

    def execute_smart_contract(
        self,
        executor_did: str,
        sc_id: str,
        data: str = "execution",
        password: str = "mypassword",
    ) -> Dict[str, Any]:
        """Convenience: Execute smart contract.

        Args:
            executor_did: DID executing the smart contract
            sc_id: Smart contract token hash
            data: Execution data/function name
            password: Password for signature

        Returns:
            {"req_id": <str>, "response": <dict>}
        """
        req_id = self.initiate_smart_contract_transaction(
            initiator_did=executor_did,
            sc_id=sc_id,
            sc_value=1.0,
            data=data,
        )
        response = self.complete_transaction(req_id, password)
        return {"req_id": req_id, "response": response}

    def subscribe_smart_contract(self, sc_id: str) -> Dict[str, Any]:
        """Subscribe to a smart contract to receive updates and enable execution.

        Args:
            sc_id: Smart contract token hash

        Returns:
            API response
        """
        log.info("[%s] Subscribing to smart contract: %s", self.name, sc_id)
        resp = self._get(f"/rubix/v1/smart_contracts/subscribe?smartContractToken={sc_id}")
        log.info("[%s] Successfully subscribed to smart contract", self.name)
        return resp

    def list_smart_contracts(self) -> list:
        """List all smart contract tokens stored in this node's database.

        Returns:
            List of smart contract token records.
        """
        log.info("[%s] Listing all smart contracts", self.name)
        resp = self._get("/rubix/v1/smart_contracts")
        result = resp.get("result", [])
        log.info("[%s] Found %d smart contracts", self.name, len(result) if result else 0)
        return result or []

    def get_smart_contract_chain(self, sc_id: str) -> list:
        """Get the ordered transaction chain for a specific smart contract.

        Args:
            sc_id: Smart contract token ID

        Returns:
            List of chain entries (transactionId, initiator, epoch, data).
        """
        log.info("[%s] Getting SC chain for: %s", self.name, sc_id)
        resp = self._get(f"/rubix/v1/smart_contracts/{sc_id}/chain")
        result = resp.get("result", [])
        log.info("[%s] SC chain has %d entries", self.name, len(result) if result else 0)
        return result or []

    def register_smart_contract_callback(
        self, sc_id: str, callback_url: str
    ) -> Dict[str, Any]:
        """Register a webhook URL for smart contract pubsub events.

        Args:
            sc_id: Smart contract token ID
            callback_url: HTTP URL to receive callbacks

        Returns:
            API response
        """
        log.info(
            "[%s] Registering callback for SC %s -> %s",
            self.name, sc_id, callback_url,
        )
        resp = self._post(
            "/rubix/v1/smart_contracts/register_callback",
            {"smartContractToken": sc_id, "callBackURL": callback_url},
        )
        log.info("[%s] Callback registered successfully", self.name)
        return resp

    # ------------------------------------------------------------------
    # Fungible Token (FT) operations
    # ------------------------------------------------------------------

    def mint_ft(
        self,
        did: str,
        ft_name: str,
        ft_count: int,
        token_count: int,
        ft_num_start_index: int,
    ) -> Dict[str, Any]:
        """Create *ft_count* fungible tokens under *ft_name* by burning
        *token_count* RBT, starting at the given FT number index.

        Endpoint: POST /rubix/v1/fts/mint (async, auto-completes password
        challenge via self.password).

        Args:
            did: Creator DID (must have >= token_count free RBT).
            ft_name: Name for the FT series (uniqueness per creator DID).
            ft_count: Number of FTs to create.
            token_count: Amount of RBT to convert into FTs.
            ft_num_start_index: Starting index for the FT series (used to
                avoid collisions across successive mints).

        Returns:
            {"req_id": <str>, "response": <dict>} — mirroring transfer_nft().
        """
        log.info(
            "[%s] Minting FT %r (count=%d, rbt=%d, start=%d) for %s",
            self.name, ft_name, ft_count, token_count, ft_num_start_index,
            did[:20] + "...",
        )
        payload = {
            "did": did,
            "ft_name": ft_name,
            "ft_count": ft_count,
            "token_count": token_count,
            "ft_num_start_index": ft_num_start_index,
        }
        # Mint is async with a signature challenge (APICreateFT runs the work in a
        # goroutine and returns a SignResponse carrying the request id). Drive the
        # two-step initiate→complete flow explicitly — like transfer_rbt — so the
        # final response carries the on-chain transactionID. Using _post here would
        # leave the challenge uncompleted (its message != "Password needed"), so the
        # transactionID would never be returned.
        init = self._post_raw("/rubix/v1/fts/mint", payload)
        req_id = (init.get("result") or {}).get("id") if isinstance(init.get("result"), dict) else None
        if not req_id:
            # No challenge id — nothing to complete; return the raw response as-is.
            log.info("[%s] FT mint response (no challenge): status=%s",
                     self.name, init.get("status"))
            return {"req_id": None, "response": init}
        response = self.complete_transaction(req_id, self.password)
        log.info("[%s] FT mint completed: status=%s req_id=%s",
                 self.name, response.get("status"), req_id)
        return {"req_id": req_id, "response": response}

    def list_fts(self) -> list:
        """List all FTs known to this node.

        Endpoint: GET /rubix/v1/fts

        Returns:
            List of FT records (id, ft_name, creator_did, ft_count, ...).
        """
        log.info("[%s] Listing all FTs", self.name)
        resp = self._get("/rubix/v1/fts")
        result = resp.get("result", [])
        log.info("[%s] Found %d FTs", self.name, len(result) if result else 0)
        return result or []

    def get_ft_balance(self, did: str) -> list:
        """Return FT balances held by *did*.

        Endpoint: GET /rubix/v1/dids/{did}/balances/ft

        Returns:
            List of {ft_name, ft_count, creator_did} dicts.
        """
        log.info("[%s] Getting FT balance for %s", self.name, did[:20] + "...")
        resp = self._get(f"/rubix/v1/dids/{did}/balances/ft")
        result = resp.get("result", [])
        log.info("[%s] FT balance: %d entries", self.name, len(result) if result else 0)
        return result or []

    def transfer_ft(
        self,
        sender_did: str,
        receiver_did: str,
        ft_name: str,
        ft_count: int,
        creator_did: str,
        memo: str = "FT transfer",
        password: str = "mypassword",
    ) -> Dict[str, Any]:
        """Transfer *ft_count* fungible tokens of series (creator_did, ft_name)
        from sender_did to receiver_did.

        Uses the bundled /rubix/v1/tx endpoint with only the ``ft`` array
        populated.  Returns {"req_id": ..., "response": ...}.
        """
        log.info(
            "[%s] Transferring FT %r count=%d: %s -> %s",
            self.name, ft_name, ft_count,
            sender_did[:20] + "...", receiver_did[:20] + "...",
        )
        payload = {
            "initiator": sender_did,
            "owner": receiver_did,
            "tokens": {
                "rbt": 0,
                "ft": [
                    {
                        "ftName": ft_name,
                        "numberOfFts": ft_count,
                        "creatorDID": creator_did,
                    }
                ],
                "nft": [],
                "smartContract": [],
                "transferNftOwnership": False,
            },
            "memo": memo,
        }
        resp = self._post_raw("/rubix/v1/tx", payload)
        req_id: str = resp["result"]["id"]
        log.info("[%s] FT transfer initiated, req_id=%s", self.name, req_id)
        final = self.complete_transaction(req_id, password)
        return {"req_id": req_id, "response": final}

    # ------------------------------------------------------------------
    # Bundled / combined transactions
    # ------------------------------------------------------------------

    def initiate_bundled_transaction(
        self,
        sender_did: str,
        receiver_did: str,
        rbt_amount: float = 0.0,
        nft_id: str = "",
        nft_value: float = 1.0,
        nft_data: str = "bundled NFT execution",
        sc_id: str = "",
        sc_value: float = 1.0,
        sc_data: str = "bundled SC execution",
        transfer_nft_ownership: bool = False,
        memo: str = "bundled transaction",
        ft_list: list = None,
    ) -> str:
        """Step 1: Initiate a bundled transaction combining RBT + NFT + SC.

        Sends all token types in a single ``/rubix/v1/tx`` payload so the
        platform locks and settles them atomically.

        Args:
            sender_did: DID initiating the transaction.
            receiver_did: DID receiving the RBT (and optionally NFT ownership).
                          For SC-only, can be empty string.
            rbt_amount: Amount of RBT to transfer (0 to skip).
            nft_id: NFT token hash to execute (empty to skip).
            nft_value: NFT value (default 1.0).
            nft_data: Data string for NFT execution.
            sc_id: Smart contract token hash to execute (empty to skip).
            sc_value: SC value (default 1.0).
            sc_data: Data string for SC execution.
            transfer_nft_ownership: If True, transfer NFT ownership to receiver.
            memo: Transaction memo.

        Returns:
            Request ID for the signature step.
        """
        log.info(
            "[%s] Initiating bundled transaction: %s -> %s  rbt=%.4f  nft=%s  sc=%s",
            self.name,
            sender_did[:20] + "...",
            receiver_did[:20] + "..." if receiver_did else "(none)",
            rbt_amount,
            nft_id[:12] + "..." if nft_id else "(none)",
            sc_id[:12] + "..." if sc_id else "(none)",
        )

        # Build token details
        nft_list = []
        if nft_id:
            nft_list.append({
                "nftId": nft_id,
                "value": nft_value,
                "data": nft_data,
            })

        sc_list = []
        if sc_id:
            sc_list.append({
                "smartContractId": sc_id,
                "value": sc_value,
                "data": sc_data,
            })

        payload = {
            "initiator": sender_did,
            "owner": receiver_did,
            "tokens": {
                "rbt": rbt_amount,
                "ft": list(ft_list) if ft_list else [],
                "nft": nft_list,
                "smartContract": sc_list,
                "transferNftOwnership": transfer_nft_ownership,
            },
            "memo": memo,
        }

        resp = self._post_raw("/rubix/v1/tx", payload)
        req_id: str = resp["result"]["id"]
        log.info("[%s] Bundled transaction initiated, req_id=%s", self.name, req_id)
        return req_id

    def execute_bundled_transaction(
        self,
        sender_did: str,
        receiver_did: str,
        rbt_amount: float = 0.0,
        nft_id: str = "",
        nft_value: float = 1.0,
        nft_data: str = "bundled NFT execution",
        sc_id: str = "",
        sc_value: float = 1.0,
        sc_data: str = "bundled SC execution",
        transfer_nft_ownership: bool = False,
        memo: str = "bundled transaction",
        password: str = "mypassword",
        ft_list: list = None,
    ) -> Dict[str, Any]:
        """Convenience: Initiate + complete a bundled transaction in one call.

        Returns:
            {"req_id": <str>, "response": <dict>}
        """
        req_id = self.initiate_bundled_transaction(
            sender_did=sender_did,
            receiver_did=receiver_did,
            rbt_amount=rbt_amount,
            nft_id=nft_id,
            nft_value=nft_value,
            nft_data=nft_data,
            sc_id=sc_id,
            sc_value=sc_value,
            sc_data=sc_data,
            transfer_nft_ownership=transfer_nft_ownership,
            memo=memo,
            ft_list=ft_list,
        )
        response = self.complete_transaction(req_id, password)
        return {"req_id": req_id, "response": response}

    # ------------------------------------------------------------------
    # All-in-one transaction — RBT + FT[] + NFT[] + SC[] in a single call
    # ------------------------------------------------------------------

    def initiate_all_transaction(
        self,
        sender_did: str,
        receiver_did: str,
        rbt_amount: float = 0.0,
        ft_list: Optional[list] = None,
        nft_list: Optional[list] = None,
        sc_list: Optional[list] = None,
        transfer_nft_ownership: bool = False,
        memo: str = "all-in-one transaction",
    ) -> str:
        """Step 1: Initiate an all-in-one transaction carrying *every* token
        type in a single ``/rubix/v1/tx`` payload.

        Unlike ``initiate_bundled_transaction`` (which accepts only a single
        NFT id and a single SC id), this variant takes full lists so a single
        request can combine:

          * an RBT amount,
          * ``ft_list``   = [{"ftName": str, "numberOfFts": float, "creatorDID": str}, ...]
          * ``nft_list``  = [{"nftId": str, "value": float, "data": str}, ...]
          * ``sc_list``   = [{"smartContractId": str, "value": float, "data": str}, ...]
          * an optional ``transferNftOwnership`` flag.

        The server processes all non-RBT assets atomically under a single DB
        transaction (see ``core/transaction_builder.go:BuildTransactionInfoFromRequest``).

        Args:
            sender_did: DID initiating the transaction.
            receiver_did: DID receiving the RBT / NFT ownership.
            rbt_amount: Amount of RBT to transfer (0 to skip).
            ft_list: Pre-built FTInfo dicts (pass ``None`` or ``[]`` to skip).
            nft_list: Pre-built NFTInfo dicts (pass ``None`` or ``[]`` to skip).
            sc_list: Pre-built SmartContractInfo dicts (pass ``None`` or ``[]`` to skip).
            transfer_nft_ownership: Whether to transfer NFT ownership to receiver.
            memo: Transaction memo.

        Returns:
            Request ID (string) for the signature step.
        """
        ft_list = list(ft_list) if ft_list else []
        nft_list = list(nft_list) if nft_list else []
        sc_list = list(sc_list) if sc_list else []

        log.info(
            "[%s] Initiating ALL-IN-ONE tx: %s -> %s  rbt=%.4f  fts=%d  nfts=%d  scs=%d  xferNftOwn=%s",
            self.name,
            sender_did[:20] + "...",
            (receiver_did[:20] + "...") if receiver_did else "(none)",
            rbt_amount,
            len(ft_list),
            len(nft_list),
            len(sc_list),
            transfer_nft_ownership,
        )

        payload = {
            "initiator": sender_did,
            "owner": receiver_did,
            "tokens": {
                "rbt": rbt_amount,
                "ft": ft_list,
                "nft": nft_list,
                "smartContract": sc_list,
                "transferNftOwnership": transfer_nft_ownership,
            },
            "memo": memo,
        }

        resp = self._post_raw("/rubix/v1/tx", payload)
        req_id: str = resp["result"]["id"]
        log.info("[%s] All-in-one transaction initiated, req_id=%s", self.name, req_id)
        return req_id

    def execute_all_transaction(
        self,
        sender_did: str,
        receiver_did: str,
        rbt_amount: float = 0.0,
        ft_list: Optional[list] = None,
        nft_list: Optional[list] = None,
        sc_list: Optional[list] = None,
        transfer_nft_ownership: bool = False,
        memo: str = "all-in-one transaction",
        password: str = "mypassword",
    ) -> Dict[str, Any]:
        """Convenience: Initiate + complete an all-in-one transaction.

        Wraps :meth:`initiate_all_transaction` and the password-challenge
        completion step.

        Returns:
            ``{"req_id": <str>, "response": <dict>}``
        """
        req_id = self.initiate_all_transaction(
            sender_did=sender_did,
            receiver_did=receiver_did,
            rbt_amount=rbt_amount,
            ft_list=ft_list,
            nft_list=nft_list,
            sc_list=sc_list,
            transfer_nft_ownership=transfer_nft_ownership,
            memo=memo,
        )
        response = self.complete_transaction(req_id, password)
        return {"req_id": req_id, "response": response}

    # ------------------------------------------------------------------
    # Transaction query methods
    # ------------------------------------------------------------------

    def get_transaction(self, tx_id: str) -> Dict[str, Any]:
        """Get a transaction by its ID.

        Args:
            tx_id: Transaction ID

        Returns:
            Transaction record.
        """
        log.info("[%s] Getting transaction: %s", self.name, tx_id)
        resp = self._get(f"/rubix/v1/tx/{tx_id}")
        return resp.get("result", {})

    def list_transactions(self) -> list:
        """List all transactions on this node.

        Returns:
            List of transaction records.
        """
        log.info("[%s] Listing all transactions", self.name)
        resp = self._get("/rubix/v1/tx")
        result = resp.get("result", [])
        log.info("[%s] Found %d transactions", self.name, len(result) if result else 0)
        return result or []

    def get_transactions_by_did(self, did: str, token_type: str) -> list:
        """Transaction history for *did* filtered by *token_type*.

        Endpoint: GET /rubix/v1/tx/{did}/{token_type}
        token_type is one of: rbt, nft, ft, smartContract.

        Returns the transaction list (empty if none).
        """
        log.info("[%s] Getting %s transactions for %s", self.name, token_type, did[:20] + "...")
        resp = self._get(f"/rubix/v1/tx/{did}/{token_type}")
        result = resp.get("result", [])
        log.info("[%s] %s tx history: %d", self.name, token_type, len(result) if result else 0)
        return result or []
