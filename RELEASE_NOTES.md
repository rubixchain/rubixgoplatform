v1.0.5 is a hardening release. Most of the work sits in consensus and chain sync — closing a double-spend path, making quorum collateral return reliably, and fixing a DID record that could quietly go wrong. There is one new API and additional logging.

---

## Double-Spend Hardening

Two independent bugs together allowed a replayed RBT split to mint its child tokens twice. Both are fixed.

**A consumed parent could come back as spendable.** Applying a synced token chain derived `token_status` from the chain's role but only handled Deploy and Execute, so a consumed parent was written back as `Free` — and since split selection only picks Free RBT, it became re-selectable and could be split a second time into a duplicate genesis. Commit and Burn are now mapped correctly in both status-derivation paths.

**Consensus now verifies split ancestry against a full node.** A child's ancestors are derived from the token-ID hierarchy — deterministic and not initiator-controlled — and intersected with the genesis transaction's declared committed tokens. Each ancestor must have been burnt by this genesis or not at all; one burnt by an earlier transaction means the split is re-consuming an already-spent token, and it is rejected. The check reads local burn records first, then syncs unconfirmed ancestors from an authoritative full node. Where no full node is reachable it accepts with a warning, so transactions keep flowing.

---

## Quorum Liquidity Fix

Rejected transactions no longer strand a quorum's pledge tokens in `Locked`.

Pledge tokens are locked early but only promoted to `Pledged` — and recorded for later release — once `PledgeV2` succeeds. Any abort before that point returned without releasing them, so they were never picked up by the unpledge callback. A release guard now runs on every abort path, scoped by lock reference ID, and is disarmed as soon as `PledgeV2` commits. Successful transactions pledge exactly as before.

---

## DID Locality Fix

A quorum node's own DID could silently stop being marked local.

The transaction callback built the publisher's DID record without setting the local flag, and the upsert overwrites that column unconditionally — so a node's own DID was flipped to non-local the first time it initiated a transaction after quorum setup. The flag is now set from the publisher identity, so self-initiated transactions re-affirm the node's own DID as local while remote initiators stay non-local. Existing incorrect records repair themselves on the node's next self-initiated transaction.

---

## Consensus Check Corrections

- `IsParentTokenBurnt` no longer syncs the genesis transaction when the transaction under validation *is* the genesis.
- A genesis-only peer fetch API backs `IsParentTokenBurnt` and the minter allowlist check. It persists nothing.
- The minter allowlist now syncs the genesis transaction from the token owner rather than the initiator.
- Pledge tokens arriving with an empty previous transaction ID are backfilled in the transaction info.

---

## New API: Public Key by DID

```
GET /rubix/v1/dids/{did}/public_key
```

Returns the hex-encoded public key a DID was derived from, resolved from the local DID directory or fetched from IPFS. This is the reverse of creating a DID from a public key, and takes the same encoding. Documented in Swagger.

---

## Logging and Tests

Debug logging across chain sync, database reads and writes, and pubsub, aimed at the full-node and sender paths where sync issues have been hardest to trace.

New coverage for the split-ancestor verification, DID public-key resolution, and the public-key API both locally and across nodes.
