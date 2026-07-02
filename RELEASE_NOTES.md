When we started working on this release, we kept running into the same wall: the old transaction model wasn't designed for what Rubix was becoming. Per-token chain blocks, CBOR serialization, LevelDB — it all made sense early on, but it wasn't going to hold up at scale, and it made cross-asset transactions unnecessarily complex. So we rebuilt the foundation.

v1.0.0 is that rebuild.

---

## The Transaction Model is Different Now

The biggest change is one you won't see in an API response, but you'll feel in every transaction.

We've moved from a model where each token maintained its own independent chain of blocks to a DAG-structured transaction graph where every transaction — regardless of asset type — is a single, unified object called `TransactionInfo`. RBT transfers, NFT executions, FT movements, smart contract calls: they all flow through the same structure, get hashed the same way, and get signed the same way.

```json
{
  "initiator": "<DID>",
  "owner": "<DID>",
  "epoch": 1748956231,
  "network": "mainnet",
  "tokens": {
    "rbt": [{ "tokenId": "...", "previousTransactionID": "..." }],
    "nft": [{ "tokenId": "...", "previousTransactionID": "..." }]
  },
  "committedTokens": ["tokenID.prevTransactionID"],
  "quorums": { "<quorum DID>": ["tokenID.prevTransactionID"] },
  "memo": "...",
  "data": "..."
}
```

The transaction ID is a deterministic hash of this struct. Signatures are over the ID, not over per-token data. Every party in a transaction — initiator, quorums, receiver — is working from the same canonical record.

Each token's history is now an ordered list of `(transactionID, role)` pairs in PostgreSQL. Traversing the chain is a table lookup, not a block walk.

The old CBOR block package and LevelDB storage are gone.

---

## Token Roles That Actually Mean Something

Tokens now carry an explicit role in each transaction entry:

Mint → Transfer → Execute → Deploy → Burn → Commit → Uncommit → Pledge → Unpledge

This sounds simple, but it eliminates an entire class of heuristics that the old code relied on to figure out what a token was doing in a given transaction. Chain traversal, validation logic, and explorer queries are all cleaner because of it.

---

## Verification That Holds Up Under Scrutiny

The consensus verification pipeline is now multi-layered and consistent across quorum and full-node:

**Step 1 — Transaction integrity.** Recompute the hash from `TransactionInfo`. If it doesn't match the provided transaction ID, reject immediately. Signature verification runs over the ID, not the raw payload.

**Step 2 — Ownership.** For each token, confirm the initiator was the owner in the referenced previous transaction. Cross-check the latest transaction ID in tokenchain against `previousTransactionID` in the payload. Confirm the token's current role makes it eligible to move.

**Step 3 — Token ID validity.** Tokens are checked against the hardcoded token map — level, number, and network. Mainnet tokens follow the token map levels strictly. Testnet token levels start from 50000. Part token index must land within the supported decimal-place range (1–1332 for 3 decimal places).

**Step 4 — Genesis provenance.** For Level-1 (premint) tokens, we walk back to the genesis transaction in tokenchain, pull the owner from transactions, and validate it against the hardcoded DID → token-range map. This is how we enforce that only the 11 designated DIDs — managing 4.3 million RBTs across defined ranges — could have minted genesis-level tokens. No exceptions.

**Step 5 — IPFS pinning.** The tokens attribute values are IPFS-added and pinned on arrival. Token state is anchored to content, not block IDs.

**Step 6 — Field validation.** DID strings validated as IPFS CIDv0 (`bafyb…`), epoch validated against current time, network validated, token structs validated for shape conformance.

---

## How Transactions Flow Now

The consensus dance has been restructured to build `TransactionInfo` incrementally, which matters for latency:

1. Stateless validation first — balance check, decimal place limits — before anything hits the network.
2. `TransactionInfo` is formed without the `quorums` field.
3. Quorums are asked only for the value they should pledge, not a token list. They return pledge token details.
4. Initiator fills the `quorums` field. `TransactionInfo` is now complete.
5. Hash it, sign it, send it to quorums for consensus.
6. Success or failure is published to `rubix_txn`. Post-consensus routing: empty owner → publish to token topic (SC/NFT self-exec); NFT → publish to token topic; everything else → direct transfer.

The round-trip payload is smaller. Quorums do less speculative work. Token selection is pre-filtered by `token_denom` rather than scanning and retrying. The async receiver sync path is now the only path — the dead synchronous branch that was sitting behind `asyncMode := false` is gone. Background token chain sync runs concurrently with transaction completion rather than blocking it.

---

## NFT Gets a Proper Transfer Flow

NFT ownership transfer previously relied on PubSub, which worked but introduced timing dependencies and made the flow hard to reason about. It now follows the same RBT/FT flow: quorum-authorized, consensus-verified, deterministic.

On top of that:

- Parent-child lineage is a first-class concept. `GET /rubix/v1/nfts/{id}/parent` and `/children` let you traverse the NFT family tree. Minting a child NFT from a parent is now explicit in the API response — `initiateTransaction` returns `MintedNFTChildren`.
- NFT content JSON now includes the RBT value of the asset, so value is recoverable from IPFS without needing the token ID as a side channel.
- Multi-file upload for NFT deploy via OpenAPI spec 3.
- Subscribers can execute NFTs without taking ownership. Value updates are restricted to the owner. Minimum pledge is `MinTransferAmount`.

---

## Minter Governance on Mainnet

The minter allowlist is now enforced at two levels — quorum and full-node — for mainnet transactions. Every token entering consensus is checked against the network-approved minter list before a single pledge is committed. The check extends to committed tokens, and the initiator gets a descriptive error rather than a silent rejection when it fails.

New swarm keys for mainnet and testnet ship with this release, along with refreshed bootstrap nodes.

---

## New Schema

Three tables now own the entire transaction layer:

| Table | What it holds |
|-------|---------------|
| `transactions` | Transaction ID, base64-encoded `TransactionInfo`, combined initiator + quorum signatures |
| `tokenchain` | Per-token ordered list of `(transactionID, role)` |
| `requests` | In-flight transaction lifecycle: request ID, transaction ID, status, role |

---

## Recovering and Syncing a Wallet From a Full Node

A node that has lost its database or drifted out of sync can now rebuild its wallet from a full node — via the `sync` command or the `POST /rubix/v1/sync` API (with a `did`). The node signs a one-time challenge with its DID's private key to prove ownership, then sends the tokens and chain positions it already has. The full node returns only the missing chain data, flags any tokens whose local state has diverged, and returns their full chain. Recovery is idempotent and resumable, and after rebuilding the database it re-pins each recovered token's state content on IPFS so the tokens are immediately spendable.
