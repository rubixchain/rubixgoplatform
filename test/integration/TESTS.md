# Integration Test Catalogue

This document lists every test the integration harness runs — happy-path
subsystem coverage **and** negative / failure-path cases. It is the reference
for what `python3 -m test.integration.runner --run-all-tests` actually verifies.

- **Driver:** HTTP REST (`/rubix/v1/...`) against a 3-node Docker cluster
  (nodeA, nodeB, quorum) backed by PostgreSQL — plus, under `--fullnode-test`,
  a 4th `-fullnode` observer node with its own PostgreSQL. **Localnet only.**
- **What "verification" means:** after each subsystem runs its transactions,
  the harness reads back node + DB state and asserts the *result* is correct
  (chain length, balance, cross-node sync, callback delivery) — not just that
  the API returned 200. A transaction can fail (e.g. budget) without failing a
  verification check, and vice-versa.
- **Pass/fail in CI:** the run exits non-zero if any verification check is
  `FAIL`. `WARN` is non-blocking (see the FT settlement note at the bottom).

Run with `--run-all-tests` (every subsystem below + negatives), or scope to a
single subsystem with the per-flag options (`--nft-only`, `--sc-only`,
`--ft-only`, `--negative-tests`, …). See the README for the full flag list.

---

## Phase order

Under `--run-all-tests` the subsystems run strictly sequentially, with a settle
window between each:

```
MINTING → SHUTTLE → NFT → SMART_CONTRACT → BUNDLED_TX → FT → ALL_IN_ONE
        → INTRA_NODE → NEGATIVE → [FULLNODE] → FINALISE
```

`FULLNODE` runs only with `--fullnode-test`. Note that the fullnode's *setup*
(DID + peer cross-registration + gossipsub mesh) happens right after node setup,
**before minting** — gossipsub has no replay, so a subscriber that joins late
silently misses everything published before it arrived. Only the *verification*
runs at the end, by which point the fullnode has observed the whole run.

---

## 1. RBT — token generation & transfer

| Phase | What it does |
|-------|--------------|
| Minting | Pre-mints localnet RBT on nodeA and quorum (`/rubix/v1/tokens/generate_local_rbt`). |
| Shuttle | Alternating A→B / B→A RBT transfers across sequential + parallel phases. |

**Verification checks**

| Check | Asserts |
|-------|---------|
| `TX_LIST_NODE_A` / `TX_LIST_NODE_B` | Each node records the expected transactions. |

(RBT balances are also asserted within the bundled / all-in-one / intra-node
checks below.)

---

## 2. NFT — create, deploy, mint children, execute, transfer

Creates N NFTs, deploys them to the chain, mints child NFTs under a parent,
self-executes, cross-node subscribes + executes, and (optionally) transfers
ownership.

**Child-NFT minting.** A parent NFT spawns children via `POST /rubix/v1/tx`
with one NFT token entry **per child**, each carrying `parentNFTId` (the server
derives each child id via IPFS — `nftId` is IGNORED when `parentNFTId` is set,
and there is **no** `numberOfChildren` field: N children = N entries). The
completed response returns `result.mintedNFTChildren` (`[{parentNFTId, childNFTId}]`)
and `result.transactionID`.

**Cross-node chain sync.** After nodeA deploys an NFT and nodeB subscribes +
executes it, the NFT chain must sync onto nodeB — asserted by comparing the
chain length on both nodes (mirrors `SC_CHAIN_SYNC`).

**Verification checks**

| Check | Asserts |
|-------|---------|
| `NFT_LIST_NODE_A` / `NFT_LIST_NODE_B` | Node lists the NFTs it should own. |
| `NFT_CHAIN_<id>_NODE_A` / `..._NODE_B` | NFT token chain has the expected length (grows per deploy/execute/transfer). On the executor node, present only after a cross-node execute (sync). |
| `NFT_CHAIN_SYNC_<id>` | Cross-node execute: NFT chain length on the executor node equals the owner's (subscribe→sync delivered the full chain). |
| `NFT_MINT_CHILDREN` | Child-mint tx succeeded and minted the requested number of children. |
| `NFT_CHILDREN_MINTED_<parent>` | The parent's `nfts/{id}/children` lists every minted child. |
| `NFT_PARENT_OF_<child>` | Each child's `nfts/{id}/parent` points back to the parent. |
| `NFT_CHILDREN_QUERY` / `NFT_PARENT_QUERY` | The children/parent query endpoints respond for a deployed NFT. |
| `NFT_BALANCE_NODE_A` / `NFT_BALANCE_NODE_B` | NFT ownership counts are correct. |

---

## 3. Smart Contract — deploy, execute, callback

Deploys N `.wasm` contracts, self-executes, cross-node subscribes + executes,
and verifies the contract's registered callback URL is actually invoked.

**Verification checks**

| Check | Asserts |
|-------|---------|
| `SC_LIST_NODE_A` / `SC_LIST_NODE_B` | Node lists the deployed contracts. |
| `SC_CHAIN_<id>_NODE_A` / `..._NODE_B` | SC token chain has the expected length. |
| `SC_CHAIN_SYNC_<id>` | The SC chain matches across nodes (consensus synced it). |
| `SC_TX_LIST_NODE_A` / `..._NODE_B` | SC transactions are recorded on each node. |
| `SC_REGISTER_CALLBACK_<id>` | Callback URL registration succeeded. |
| `SC_CALLBACK_REGISTER_<id>` | Callback registered in the node's `call_back_urls`. |
| `SC_CALLBACK_TRIGGER_EXECUTE_<id>` | Executing the contract triggers the callback. |
| `SC_CALLBACK_DELIVERED_<id>` | The node actually POSTed to the callback receiver. |
| `SC_CALLBACK_INITIATOR_<id>` | Callback payload carries the correct initiator. |
| `SC_CALLBACK_DELIVERY` | End-to-end callback delivery (skipped if no SC deployed). |

---

## 4. FT — fungible token mint & transfer

Mints N FT batches (burning RBT), then transfers a slice of each batch
nodeA ↔ nodeB.

**Verification checks**

| Check | Asserts |
|-------|---------|
| `FT_LIST_NODE_A` / `FT_LIST_NODE_B` | Node lists its FT series. |
| `FT_BALANCE_NODE_A` / `FT_BALANCE_NODE_B` | Per-DID FT counts are correct. |
| `FT_TX_LIST_NODE_A` / `FT_TX_LIST_NODE_B` | Expected FT transactions recorded. |

---

## 5. Bundled transaction (RBT + NFT + SC in one `/tx`)

A single `/rubix/v1/tx` call carrying an RBT transfer + NFT execution + SC
execution atomically, alternating A→B / B→A each round.

**Verification checks**

| Check | Asserts |
|-------|---------|
| `BUNDLED_RBT_BALANCE_NODE_A` / `..._NODE_B` | RBT moved correctly. |
| `BUNDLED_NFT_CHAIN_NODE_A` / `..._NODE_B` | NFT chain advanced on both nodes. |
| `BUNDLED_SC_CHAIN_NODE_A` / `..._NODE_B` | SC chain advanced on both nodes. |
| `BUNDLED_SC_CHAIN_SYNC` | SC chain consistent across nodes. |
| `BUNDLED_TX_LIST_NODE_A` / `..._NODE_B` | The bundled transaction is recorded. |

---

## 6. All-in-one transaction (RBT + every FT + every NFT + every SC in one `/tx`)

A single `/rubix/v1/tx` per round carrying RBT + every minted FT batch + every
deployed NFT + every deployed SC. Direction alternates A↔B per round.

**Verification checks**

| Check | Asserts |
|-------|---------|
| `ALLINONE_RBT_BALANCE_NODE_A` / `..._NODE_B` | RBT moved correctly. |
| `ALLINONE_FT_BALANCES_NODE_A` / `..._NODE_B` | Every FT batch updated correctly. |
| `ALLINONE_NFT_CHAIN_NODE_A_<id>` / `..._NODE_B_<id>` | Each NFT chain advanced on both nodes. |
| `ALLINONE_SC_CHAIN_NODE_A_<id>` / `..._NODE_B_<id>` | Each SC chain advanced on both nodes. |
| `ALLINONE_SC_CHAIN_SYNC_<id>` | Each SC chain consistent across nodes. |
| `ALLINONE_TX_LIST_NODE_A` / `..._NODE_B` | The all-in-one transaction is recorded. |

---

## 7. Intra-node (two DIDs on the same node)

Creates a **secondary DID** (`did_a2`) on nodeA and exercises the full asset
matrix against the primary `did_a`, all within nodeA's wallet boundary — RBT
ping-pong, FT back-and-forth, plus an NFT and an SC deployed + self-executed by
`did_a2`.

**Verification checks**

| Check | Asserts |
|-------|---------|
| `intra_node.secondary_did_created` | did_a2 was created on nodeA. |
| `intra_node.rbt_balances` | RBT balances readable for did_a and did_a2. |
| `intra_node.nft_chain` | did_a2's NFT chain advanced. |
| `intra_node.sc_chain` | did_a2's SC chain advanced. |
| `intra_node.ft_balance[<ft_name>]` | did_a2 holds the funded FTs. **(WARN-only — see note.)** |

---

## 8. Negative / failure-path tests

These invert the assertion: an **invalid** operation must be **rejected** for
the right reason **and** leave observable state **unchanged**. Stricter than a
plain "expect failure" — a rejection that happened for the wrong reason (or a
state change) is a FAIL.

| Check | Scenario | Asserts |
|-------|----------|---------|
| `NEG_RBT_ZERO_BALANCE` | Transfer RBT from a DID with no balance | Rejected; sender balance unchanged. |
| `NEG_RBT_INSUFFICIENT` | Transfer more RBT than owned | Rejected (insufficient balance); balance unchanged. |
| `NEG_RBT_DECIMAL_PLACES` | Transfer `0.00000009` RBT (> 3 decimal places) | Rejected by the precision rule; balance unchanged. |
| `NEG_FT_OVER_TRANSFER` | Transfer 1,000,000 FTs that aren't held | Rejected (FT lock fails); FT balance unchanged. |
| `NEG_INVALID_RECEIVER_DID` | Transfer to a malformed / unknown DID | Rejected. |
| `NEG_NON_POSITIVE_AMOUNT` | Transfer a negative RBT amount | Rejected; balance unchanged. |

---

## 9. Transaction persistence (final cross-node assurance)

A cross-cutting check that runs **last**, after every subsystem and the deferred
intra-node check, so all writes have committed across nodes. For every recorded
transaction with `status == SUCCESS` and an on-chain `transaction_id`, it looks
the id up in the `transactions` table and asserts the row exists on **every node
that took part** — both the sender's and the receiver's node for a cross-node
transfer, and the single node for a mint / deploy / self-execute.

Which nodes must hold a given txn is read from the row's own
`info->>'initiator'` and `info->>'owner'` DIDs (mapped to nodes), not guessed
from per-engine record fields — so cross-node transfers naturally require both
ends and single-node operations require one. A SUCCESS txn that is absent from
**any** node, or missing on a participant node, is a hard FAIL.

**Verification checks**

| Check | Asserts |
|-------|---------|
| `TX_PERSISTED_BOTH_NODES` | Every recorded-SUCCESS transaction is persisted in the `transactions` table on each participating node (sender AND receiver). Missing on any participant → FAIL. |

**Coverage.** A record may carry more than one on-chain id (`transaction_id` and
`deploy_transaction_id`) — the intra-node NFT/SC phases fire a deploy AND an
execute, and both are verified. Covered: RBT shuttle, intra-node RBT/FT, FT
transfers, NFT/SC deploy + self-execute + cross-node execute + ownership
transfer, bundled, and all-in-one.

**Legitimately skipped (no on-chain id to assert, never a FAIL):**

- `FT_MINT` — the FT mint API (`POST /rubix/v1/fts/mint` → `Core.CreateFTs`)
  returns `{"message": "FT created successfully", "result": null}`. Unlike NFT
  mint, it does **not** surface a `transactionID`, so there is nothing to look
  up. (The mint still creates the FT genesis transaction in the DB; the id just
  isn't returned to the client.)
- `INTRA_NODE_SETUP` — secondary-DID creation, not a token transaction.

---

## 10. Fullnode (`--fullnode-test`)

Starts a 4th node with `rubixgoplatform run -fullnode` (compose profile
`fullnode`, its own PostgreSQL `rubix_f`) and asserts the whole observer path:

```
localnet transaction → rubix_txn PubSub → fullnode → validation → persistence
```

**What the fullnode actually does** (`core/fullnode.go`): subscribes to the
`rubix_txn` gossipsub topic, drops events with `Status == false`, de-duplicates
by transaction ID, then re-runs the *full* consensus validation
(`consensus.ValidateTransaction` with `isFullnode=true` — transaction-ID
integrity, initiator **and** quorum signature verification, token-chain
integrity with peer chain-sync, replayed-split detection, token ownership by
previous txn, pledged-token checks, the fullnode-only
transaction-value-vs-pledge-value check, IPFS pin checks). Only on success does
it write to `fullnode_transactions` / `fullnode_rbt` / `fullnode_tokenchain` /
`fullnode_tokenchain_index`; a terminal validation failure is dead-lettered into
`fullnode_invalid_transactions`.

So a row in `fullnode_transactions` is *proof the entire path completed* — not
merely that a message arrived. That is what makes these checks meaningful.

**Driving transaction.** The suite drives its own tracked A→B RBT transfer
rather than reusing one from an earlier phase, so the assertion never depends on
another subsystem's timing. The transfer retries with **alternating direction**
(A→B, B→A, …): this phase runs last, and a bad all-in-one / intra-node round can
leave one node's token chain in a state the quorum rejects at consensus
("chain mismatch after sync"), which would otherwise fail the fullnode phase for
a reason that has nothing to do with the fullnode. Whatever transfer succeeds is
asserted against exactly as strictly — the real participants are carried into the
field checks, so a B→A transfer is verified as B→A.

**How it is observed.** The fullnode exposes **no HTTP API** over its stored
transactions (its only fullnode-specific routes are libp2p:
`/rubix/v1/fullnode/sync`, `/rubix/v1/fullnode/recover`). Reading its own
PostgreSQL tables is therefore the intended observation point — see
`clients/db_validator.py:FullnodeDBValidator`. All waits are bounded polls
against real conditions (container health, gossipsub peer membership, a row
landing); there are no fixed sleeps.

**Verification checks**

| Check | Asserts |
|-------|---------|
| `FULLNODE_STARTUP` | The `-fullnode` node came up and migrated its schema (`fullnode_transactions` exists). |
| `FULLNODE_PUBSUB_SUBSCRIBED` | The node subscribed to `rubix_txn` — via its own `ipfs pubsub ls` and/or the `Successfully subscribed to topic: rubix_txn` line in `/app/data/log.txt`. |
| `FULLNODE_TXN_RECEIVED` | A tracked A→B RBT transfer was received, validated and persisted into `fullnode_transactions` within the timeout. |
| `FULLNODE_TRACKED_TXN_NOT_REJECTED` | The tracked transfer is **not** in `fullnode_invalid_transactions`. It is driven in isolation by the engine, so its arrival order is deterministic and there is no legitimate reason to reject it. |
| `FULLNODE_TXN_FIELDS` | The stored row's `id`, and its `info` `initiator` / `owner` / `network` / `tokens.rbt`, match the transfer that was made. |
| `FULLNODE_QUORUM_SIGNATURE_PERSISTED` | The stored `signature` JSONB carries the initiator signature and ≥1 complete quorum signature (did + signature). These were verified by `SignatureVerificationCheck` before persistence. |
| `FULLNODE_RBT_TOKEN_STATE` | Every transferred RBT token has a `fullnode_rbt` row owned by the **receiver** DID at this transaction id. |
| `FULLNODE_PLEDGED_TOKENS` | The transaction records ≥1 quorum with pledged tokens, and each pledged token has a `pledge`-role `fullnode_tokenchain` row for this transaction. |
| `FULLNODE_TOKENCHAIN_WRITTEN` | Per-token chain entries were written for the transaction (not just the transaction row). |
| `FULLNODE_NO_DUPLICATE_CHAIN_ENTRIES` | No `(token_id, transaction_id)` pair appears twice. Unlike `tokenchain`, `fullnode_tokenchain` has **no** `UNIQUE(token_id, transaction_id)` constraint, so nothing at the DB layer would stop this. |
| `FULLNODE_CHAIN_CONTIGUOUS` | Every stored chain is a contiguous `0..N-1` position run. |
| `FULLNODE_CHAIN_LINKED` | Each row's `previous_transaction_id` equals the transaction at `position-1`; position 0 (and only position 0) has an empty previous id. |
| `FULLNODE_CHAIN_NO_DANGLING_REFS` | No chain row references a transaction absent from `fullnode_transactions` — the two `DEFERRABLE INITIALLY DEFERRED` FKs were never deferred into a lie. |
| `FULLNODE_PLEDGE_NEVER_GENESIS` | No `pledge`-role chain entry has an empty `previous_transaction_id` (a quorum must already have owned the token). |
| `FULLNODE_DEAD_LETTER_REPORT` | **WARN, non-gating.** Reports the total number of dead-lettered transactions and a grouped reason histogram. See the note below. |
| `FULLNODE_RESTART_RECOVERY` | After a container restart the fullnode re-subscribes, re-joins the gossipsub mesh and **processes** a new transaction published post-restart. PASS if it persists it; WARN if it receives it and rejects it at validation (the subscription and mesh still recovered — the rejection belongs to the dead-letter report); **FAIL only on silence** — neither persisted nor dead-lettered, i.e. the event never reached it. |
| `FULLNODE_RESTART_IDEMPOTENT` | Re-ingestion after restart produced no duplicate chain entries and lost no rows. The dedup map (`DynamicTxnProcessor.processedTxns`) is in-memory and empty after a restart, so anything gossipsub re-delivers takes the full processing path again. (A growing dead-letter count is *not* treated as a restart regression — retries of pre-restart transactions can exhaust after it.) |

The integrity checks (`NO_DUPLICATE` / `CONTIGUOUS` / `LINKED` /
`NO_DANGLING_REFS` / `PLEDGE_NEVER_GENESIS`) and the dead-letter report assert
over **everything the fullnode stored during the run** — every mint genesis
transaction plus every RBT/FT/NFT/SC/bundled transaction the other subsystems
produced — not just the one tracked transfer.

### Why the dead-letter total is a WARN, not a FAIL

Under the concurrent `--run-all-tests` matrix the fullnode rejects a tail of
transactions (order of 10–30 out of ~1500 ingested in measured runs). Two
distinct causes have been observed, **both pre-existing and neither introduced
by this suite**:

**1. FT genesis transactions are signed over the wrong bytes.**
`Wallet.FTGenesisTxn` (`core/wallet/token_chain.go:786`) signs the *serialized
transaction info*:

```go
txInfoBytes, _ := models.SerializeTransactionInfo(txnInfo)
signatureBytes, _ := dc.Sign(txInfoBytes)          // <- serialized info
```

Every other signing path, and the verifier, use the *transaction ID* (the hex
SHA3-256 of those bytes) — `util.SignTransaction` / `util.VerifySignature`
(`util/transaction.go:31`):

```go
transactionId, _ := GetTransactionID(txInfo)
signatureBytes, _ := dc.Sign([]byte(transactionId))  // <- transaction id
```

`core/wallet/token_chain.go:786` is the only `dc.Sign` call site in the
transaction path that does not sign the transaction ID. The two can never
agree, so **every** published FT genesis transaction fails the fullnode's
`SignatureVerificationCheck` and is dead-lettered with
`initiator signature verification failed: SignVerify: failed to verify private
key signature`. (Base64 encodings do match — both are `StdEncoding` — so the
mismatch is the signed payload, not the encoding.)

This suite reports the defect; it deliberately does **not** fix it. Changing
what a transaction signs is security-sensitive and has an on-chain
compatibility dimension (FT genesis transactions already signed the old way on
testnet/mainnet would not re-verify under a corrected scheme), so the fix and
its migration story belong to the maintainers, not to a CI-coverage change.

**2. Genesis events that arrive after an on-demand chain sync.**
`TokenChainIntegrityCheck`'s genesis rule (`core/consensus/checks.go:727-731`):
a token's genesis event is processed *after* a later transaction already caused
the fullnode to sync that token's chain from a peer, so
`latestTransactionID != currentTxID` and the now-redundant genesis event is
rejected. The remainder are `chain mismatch` on pledge tokens from the same
arrival-order race. Eliminating this needs the hold-and-release / bundling gate
described in `Fullnode-Txn-Bundling-Implementation-Plan.md`, which is **not
implemented on this branch**.

In both cases the stored data is intact — every structural check above
(`NO_DUPLICATE`, `CONTIGUOUS`, `LINKED`, `NO_DANGLING_REFS`,
`PLEDGE_NEVER_GENESIS`) passes. It is the surplus/unverifiable event that is
dropped, not the chain.

Gating CI on the total would fail every PR for known pre-existing defects
rather than for a regression, so the suite reports the count and a grouped
reason histogram into `verification.json` and the CI log instead — visible and
diffable run-to-run. **Flip `FULLNODE_DEAD_LETTER_REPORT` to a hard FAIL once
the FT genesis signing bug and the bundling gate both land.**

The hard guarantees are unaffected: the tracked transaction **must** be
validated and persisted, **must not** be rejected, and nothing the fullnode did
store may be duplicated, discontiguous, unlinked or dangling.

**Not covered, and why: failed transactions.** `TxnCallBack` returns early on
`!newEvent.Status` (`core/fullnode.go`), so a *failed* transaction is published
but deliberately never stored. There is no "failed transaction persisted with
expected state" behaviour to assert. `FULLNODE_TRACKED_TXN_NOT_REJECTED` and
`FULLNODE_DEAD_LETTER_REPORT` cover the adjacent case — a transaction the
fullnode receives but cannot validate.

Skip the restart pair with `--no-fullnode-restart` (saves ~1–2 min locally).

---

## Notes

- **`intra_node.ft_balance` PASS/FAIL — the old "settlement lag" WARN was a
  harness parsing bug, not lag.** `GET /rubix/v1/dids/{did}/balances/ft` returns
  `result[]` of `FTBalance`, whose JSON keys are **`name`/`creator`/`value`/`count`**
  (see `types/balance.go`). The check used to match on `ft_name`/`FTName` — keys
  that do **not** exist in the response — so it always counted 0 and WARNed, even
  though the API returned the FT correctly. Fixed to read the real keys (`name`/
  `count`). The API works and reflects the transfer immediately (no lag). The
  `tokens` table (`token_type='ft'`) is queried as an independent cross-check;
  PASS if either source shows the FT, FAIL only if both agree it's absent.
  Verified (2026-06-05): `API count=2, DB FT tokens=2`.

- **Transaction success ≠ verification success.** Failed transactions (e.g.
  token-budget starvation in an undersized run) can coexist with all
  verification checks passing, because verification asserts the *end-state* of
  what succeeded. CI uses `--small-test`, sized so the full matrix completes.

- **Outputs** (under `test/docker/data/stress/logs/`): `verification.json`
  (per-check PASS/WARN/FAIL), `summary.json` (tx counts + balances),
  `transactions.jsonl` (every transaction), `db_snapshot_<ts>.txt`.
