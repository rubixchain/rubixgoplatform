# Phase 08: Persistence Wiring - Context

**Gathered:** 2026-03-20
**Status:** Ready for planning

<domain>
## Phase Boundary

Wire `c.w.PersistPostConsensus(...)` into the initiator transaction flow — **one insertion point, one file, one call site**.

This phase satisfies **PERSIST-04 only**: `PersistPostConsensus` has at least one verified call site in the consensus finalization path (currently zero).

**PERSIST-01, PERSIST-02, PERSIST-03, PERSIST-05 are explicitly OUT OF SCOPE** for this phase — deferred to a future phase.

**Files this phase may touch:** `core/transaction.go` only.

**Files this phase MUST NOT touch:** `core/quorum_recv.go`, `core/unpledge.go`, `core/ft.go`, `core/parts/split.go`.

</domain>

<decisions>
## Implementation Decisions

### Scope
- Phase 08 = PERSIST-04 only — wire `PersistPostConsensus` in initiator flow, single call site
- No additional persistence wiring (pledge, unpledge, FT, split atomicity) — those are a future phase
- DO NOT call persistence during token creation or collection — only after consensus succeeds and signatures are assembled

### Failure handling
- **Soft fail** — if `PersistPostConsensus` returns an error, log the error and continue
- Do NOT return an error to the API caller; do NOT block the transaction response
- The transaction proceeds normally even if persistence fails; the DB miss is a known limitation at this stage
- Log at `Error` level with enough context (transactionID, DID, error message) to diagnose later

### Insertion timing
- Call `PersistPostConsensus` **BEFORE** `util.PublishTransaction`
- Order: persist → broadcast → notify receiver
- Rationale: if persistence fails, nothing has been broadcast yet — cleaner semantics; no network-visible transaction without a DB record attempt

### Call site specification
- **File:** `core/transaction.go`
- **Function:** `initiateTransaction`
- **Position:** After `signatureTobePublished` is assembled (line ~166), before `util.PublishTransaction` (line ~168)
- **Execution role:** `wallet.ExecutionRoleInitiator` ("initiator")
- **Required fields:** `TransactionInfo`, `Signature`, `DID`, `ExecutionRole`
- **Optional fields:** `AffectedTokens`, `TokenChainRows`, `TokenStates` — leave nil; `BuildPersistencePayload` auto-derives them from the DB

### Claude's Discretion
- Exact log message wording
- Whether to capture the error in a named variable or use `_` (use named variable for the log)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Persistence coordinator (the function being wired)
- `core/wallet/post_consensus_persistence.go` — `PersistPostConsensus` signature, `PostConsensusPersistenceRequest` struct, `ExecutionRoleInitiator` constant, `BuildPersistencePayload` auto-derivation behaviour (lines 1–110)

### Insertion target (the file being changed)
- `core/transaction.go` — `initiateTransaction` function (lines 23–196); insertion point is between `signatureTobePublished` assembly (~line 166) and `util.PublishTransaction` call (~line 168)

### Data available at insertion point (no assumptions needed)
- `ctx` — `context.TODO()` defined at line 28
- `transactionInfo` — `*models.TransactionInfo`, complete after `BuildTransactionInfoFromRequest` (line 46)
- `signatureTobePublished` — `*models.Signature`, assembled at lines 163–166
- `initiatorDID` — `string`, defined at line 29

No external specs — requirements fully captured in decisions above.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `wallet.ExecutionRoleInitiator` — string constant `"initiator"` in `core/wallet/post_consensus_persistence.go:19`
- `wallet.PersistPostConsensus(ctx context.Context, req *PostConsensusPersistenceRequest) error` — the function being wired; takes context + request struct
- `wallet.PostConsensusPersistenceRequest` — struct with fields: `Transaction`, `TransactionInfo`, `Signature`, `DID`, `ExecutionRole`, `AffectedTokens`, `TokenChainRows`, `TokenStates`

### Established Patterns
- Error logging pattern in `initiateTransaction`: `c.log.Error("InitiateTransaction: <msg>", "err", err)` — match this style
- Non-fatal errors in the function (e.g., `receiverPeer` error at line 190) use log + continue, not return — this is the model for our soft-fail
- `c.w.` is how all wallet methods are called from `Core`

### Integration Points
- Insertion window: `core/transaction.go` lines 166–167 (between signature assembly and publish)
- Import needed: `core/wallet` package is already imported via `c.w` — no new import required for the call itself

</code_context>

<specifics>
## Specific Ideas

- The call should look like:
  ```go
  if err := c.w.PersistPostConsensus(ctx, &wallet.PostConsensusPersistenceRequest{
      TransactionInfo: transactionInfo,
      Signature:       signatureTobePublished,
      DID:             initiatorDID,
      ExecutionRole:   wallet.ExecutionRoleInitiator,
  }); err != nil {
      c.log.Error("InitiateTransaction: failed to persist post-consensus state", "err", err)
  }
  ```
- This is the minimum viable call — `AffectedTokens/TokenChainRows/TokenStates` are nil and auto-derived

</specifics>

<deferred>
## Deferred Ideas

- **PERSIST-01**: Genesis minting (RBT/SC) wired to `PersistPostConsensus` — future phase
- **PERSIST-02**: Pledge/unpledge flows (`quorum_recv.go:1858`, `unpledge.go`) wired to `PersistPostConsensus` — future phase
- **PERSIST-03**: FT genesis and burn block creation wired to `PersistPostConsensus` — future phase
- **PERSIST-05**: `performTokenSplit` RBT split atomicity in `core/parts/split.go` — future phase
- Hard-fail behaviour (return error to API caller if persistence fails) — revisit after initial wiring is proven stable

</deferred>

---

*Phase: 08-persistence-wiring*
*Context gathered: 2026-03-20*
