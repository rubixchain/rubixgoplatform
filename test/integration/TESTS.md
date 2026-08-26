# Integration Test Catalogue

This document lists every test the integration harness runs — happy-path
subsystem coverage **and** negative / failure-path cases. It is the reference
for what `python3 -m test.integration.runner --run-all-tests` actually verifies.

- **Driver:** HTTP REST (`/rubix/v1/...`) against a 3-node Docker cluster
  (nodeA, nodeB, quorum) backed by PostgreSQL. **Localnet only.**
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
        → INTRA_NODE → NEGATIVE → FINALISE
```

Within the NFT subsystem the sub-phases are also ordered, and the burn is
strictly last because it is terminal:

```
create → deploy → mint children → self-execute → transfer → cross-execute
       → repeated executions → BURN → verification
```

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

## 2a. NFT burn (`--nft-burn`)

Permanently destroys an NFT. Enabled by `--nft-burn` (and by `--run-all-tests`,
but deliberately **not** by `--bundled-test` / `--all-in-one-test`, which reuse
the deployed NFTs in later rounds).

**Why burning exists.** Quorum pledge release is triggered by a *later*
transaction spending the pledged tokens. RBT keeps moving, so RBT pledges always
eventually release; a dead NFT is never transacted again, so the quorum's
collateral would stay pledged forever. The burn is that missing terminal
transaction.

**Burn is terminal — it runs LAST among the NFT phases.** A burnt NFT can no
longer be executed or transferred, so running it earlier would break every phase
after it. The parent-rejection test also depends on the child-mint phase having
already run.

**Shape.** `POST /rubix/v1/tx` with `tokens.burnNft: true`. Unlike an execute or
transfer, a burn does **not** go through quorum consensus — it is a self-signed
transaction, persisted locally and then published to two channels: the
`rubix_txn` stream (so pledging quorums release collateral) and the NFT's own
topic (so subscribers learn the NFT is dead).

**Three phases run in order:** burn a childless NFT; attempt to burn a parent
that still has live children (**must be refused**); re-burn the already-burnt
NFT (**must succeed idempotently**).

**Verification checks**

| Check | Asserts |
|-------|---------|
| `NFT_BURN_STATUS_<id>` | The burnt NFT's chain grew to ≥ 2 entries (deploy + burn). |
| `NFT_BURN_BALANCE_NODE_A` / `..._NODE_B` | The burnt NFT no longer appears in its owner's NFT balance (covers the `GetNFTsByDid` status filter). |
| `NFT_BURN_CHAIN_SYNC_<id>` | The burn synced to the opposite (subscriber) node. **WARN (non-blocking) on localnet**, where publish is a no-op — see the caveat below. Becomes a real PASS on a testnet. |
| `NFT_BURN_PARENT_BLOCKED` | Burning a parent NFT with live children was **refused**. A FAIL here means children were silently orphaned. WARN if no parent/child pair existed to test. |
| `NFT_BURN_IDEMPOTENT` | Re-burning an already-burnt NFT succeeded instead of erroring. WARN if nothing was burnt. |
| `NFT_BURN_RAN` | Emitted as WARN only when the burn phase found no burnable NFT (every deployed NFT was a parent or a child). Raise `--nft-count` to exercise the phase. |

**Negative cases.** Because a burn is terminal and skips consensus,
`validateNFTBurnRequest` is the only thing between a malformed request and
permanent asset destruction. Each of these asserts the node **refuses**:

| Check | Request that must be refused |
|-------|------------------------------|
| `NFT_BURN_REJECTS_WRONG_OWNER` | Burning an NFT the initiator does not own. |
| `NFT_BURN_REJECTS_UNKNOWN_NFT` | Burning a non-existent NFT id. |
| `NFT_BURN_REJECTS_BURN_PLUS_TRANSFER` | `burnNft` + `transferNftOwnership` (contradictory). |
| `NFT_BURN_REJECTS_BURN_PLUS_RBT` | `burnNft` + an RBT transfer — the RBT side would ride the non-consensus path and silently lose quorum validation. |
| `NFT_BURN_REJECTS_BURN_PLUS_SC` | `burnNft` + a smart contract entry. |
| `NFT_BURN_REJECTS_EMPTY_NFT_ID` | `burnNft` with an empty `nftId`. |
| `NFT_BURN_REJECTS_MIXED_BURNT_AND_LIVE` | A request mixing an already-burnt NFT with a live one (rejected rather than partially applied). |
| `NFT_BURN_CHILD_ALLOWED` | **Positive:** burning a CHILD NFT *succeeds* — only parents with live children are blocked. |

**Parent/child unblocking sequence.** The parent guard is only correct if it
blocks *and* unblocks at the right boundary. A guard that simply refused every
parent forever would pass a naive block-only test, so these four checks run as a
chain:

| Step | Check | Expected |
|------|-------|----------|
| 2 live children | `NFT_BURN_PARENT_BLOCKED` | parent REFUSED |
| burn child #1 | `NFT_BURN_CHILD_ALLOWED` | child burnt |
| 1 live child | `NFT_BURN_PARENT_STILL_BLOCKED` | parent STILL refused |
| burn child #2 | `NFT_BURN_PARENT_AFTER_CHILDREN` | parent NOW burnable |

The last step is the important one: `liveChildNFTs` filters out already-burnt
children, so a fully-burnt subtree must not pin its root un-burnable forever.

> NFTs are never `Pledged` (pledging selects RBT only, via `CollectRBTTokens`),
> so there is no pledged-NFT burn case to test. The status guard in
> `validateNFTBurnRequest` stays default-deny anyway, so any future status is
> refused until someone explicitly decides it is burnable.

**Sizing note.** The burn deliberately skips parents (un-burnable by design) and
children, so it needs at least one standalone NFT. With `--nft-count 2` the
child-mint phase claims NFT #1, leaving exactly one candidate — enough, but with
no margin. Use `--nft-count 3` or more when you want the burn phase reliably
exercised.

> ### ⚠ Localnet caveat — what this suite cannot prove
>
> `util.PublishTransaction` returns immediately on localnet
> (`util/transaction.go:63-66`), so **nothing is broadcast** and this harness is
> localnet-only. The suite verifies the API contract, both guards, the local
> status change, the chain entry, and balance filtering. It does **not** verify
> the two properties the feature exists for:
>
> 1. quorum collateral actually being released, and
> 2. whether receiving nodes accept a burn transaction, which by design carries
>    **no quorum signature** (`transactionInfo.Quorums` is empty).
>
> Both require a manual run against a real testnet with a live quorum holding a
> matching `unpledge_sequence_info` row. Treat a green burn suite as necessary
> but not sufficient.

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
