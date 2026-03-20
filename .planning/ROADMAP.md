# Roadmap: Rubix Ledger

## Milestones

- [x] **v1: Transaction Preparation** — Phases 01-02 (complete)
- [x] **v2: Compilation Recovery Audit** — Phase 04 (complete)
- [ ] **v3: Legacy Removal & PostgreSQL Migration** — Phases 05-10 (in progress)

---

<details>
<summary>v1: Transaction Preparation (Phases 01-02) — COMPLETE</summary>

### Phase 01: Wallet Foundation
**Goal**: PostgreSQL-backed wallet layer exists with token locking for all token types
**Status**: Complete

### Phase 02: BuildTransactionInfoFromRequest
**Goal**: Transaction preparation function packages tokens into TransactionInfo ready for consensus
**Status**: Complete

### Phase 03: Transaction Atomicity (DEFERRED)
**Goal**: RBT split wrapped in a single pgx.Tx; querier pattern introduced
**Status**: Deferred — plans preserved in phases/03-transaction-atomicity/

</details>

<details>
<summary>v2: Compilation Recovery Audit (Phase 04) — COMPLETE</summary>

### Phase 04: Compilation Audit
**Goal**: Every go build ./... error on release-v1 is identified, classified, and given an action item
**Status**: Complete

</details>

---

## v3: Legacy Removal & PostgreSQL Migration

**Milestone Goal:** Remove all deleted-package imports and legacy patterns from release-v1 so that `go build ./...` passes with zero errors.

## Phases

- [x] **Phase 05: Foundation Fixes** — Fix all isolated compile blockers and unify the constants type system (completed 2026-03-19)
- [x] **Phase 06: Wallet Layer** — Implement missing wallet methods, fix tokenchain column bug, remove LevelDB remnants, replace legacy token types (completed 2026-03-20)
- [x] **Phase 07: Contract Removal** (completed 2026-03-20) — Replace contract.TokenInfo and contract.Contract at all call sites with models equivalents
- [ ] **Phase 08: Persistence Wiring** — Wire PersistPostConsensus to all genesis/pledge/unpledge/FT paths; fix RBT split atomicity
- [ ] **Phase 09: Architecture Consolidation** — Remove type-specific initiators; make InitiateTransaction the sole entry point for all token types
- [ ] **Phase 10: Stub and Cleanup** — Stub tokenchain validation and unpledge; remove credit system; strip all remaining block/contract imports; verify clean build

## Phase Details

### Phase 05: Foundation Fixes
**Goal**: All isolated, zero-risk compile blockers are resolved and the constants package is the single source of truth for token status values
**Depends on**: Phase 04 (audit complete)
**Requirements**: COMPILE-01, COMPILE-02, COMPILE-03, COMPILE-04, COMPILE-05, CONST-01, CONST-02
**Success Criteria** (what must be TRUE):
  1. `core/wallet/token_chain.go` queries compile and use `position` as the column name instead of `height`
  2. `core/wallet/denom.go` compiles — `InitNewTokenDenomArrayForDID` has a body, the `unnest` typo is fixed, and the missing comma in the SET clause is present
  3. `core/wallet/token.go` `LockTokenByID` uses a parameterised `$1` placeholder instead of the hardcoded literal `2`
  4. `GetTokensFromDenomMap` SQL uses `LIMIT $4` (no equals sign) and compiles
  5. `APISyncTransactionChain` and `SyncTransactionChainRequest` are defined or their undefined references are removed; all `wallet.TokenIsXxx` call sites replaced with `constants.TokenStatus_Xxx`
**Plans:** 3/3 plans complete
Plans:
- [ ] 05-01-PLAN.md — Fix SQL compile blockers in core/wallet/ (token_chain, denom, token)
- [ ] 05-02-PLAN.md — Define APISyncTransactionChain, SyncTransactionChainRequest, and new TokenStatus constants
- [ ] 05-03-PLAN.md — Replace all wallet.TokenIs* references with constants.TokenStatus_*

### Phase 06: Wallet Layer
**Goal**: The `core/wallet` package exposes all methods that `core/` code calls via `c.w.*`, all backed by PostgreSQL, with no LevelDB remnants and no legacy struct types
**Depends on**: Phase 05
**Requirements**: WALLET-01, WALLET-02, WALLET-03, WALLET-04
**Success Criteria** (what must be TRUE):
  1. All 8 missing wallet methods exist and return correct types: `ReadToken`, `GetAllTokens`, `IsDIDExist` (single bool return), `CreateToken`, `CreateSmartContractToken`, `UpdateNFTStatus`, `GetFTTokensChunk`, `GetSmartContractTokensChunk`
  2. Tokenchain read methods use `position` column — `GetTokenChainByTokenID`, `GetLatestTransactionAndRoleByTokenID`, `GetTransactionAndRoleAtHeight` return correct rows without runtime errors; fetch-by-transactionID method exists
  3. `core/ft.go` and `core/ft_transfer_optimized.go` contain no `c.s.*` calls — all FT storage reads and writes go through `c.w.*` PostgreSQL methods
  4. No file in the codebase references `wallet.Token`, `wallet.FTToken`, or `wallet.SmartContract` struct types — all replaced with `models.Token`
**Plans**: TBD

### Phase 07: Contract Removal
**Goal**: The deleted `contract` package is no longer referenced in any function signature, type assertion, or struct literal — replaced entirely by `models.TokenInfo`, `models.TransactionInfo`, and `util.VerifySignature`
**Depends on**: Phase 06
**Requirements**: CONTRACT-01, CONTRACT-02
**Success Criteria** (what must be TRUE):
  1. All 9 `contract.TokenInfo` call sites use `models.TokenInfo` — extra fields (TokenValue, TokenType, OwnerDID) are fetched from the `tokens` table by TokenID at call sites that need them
  2. All 12 `contract` import sites are updated — `contract.Contract` consensus envelope is replaced with `models.TransactionInfo` plus `util.VerifySignature` (`util/transaction.go`); `contract.VerifySignature` is gone from the codebase
  3. `go build ./...` fails only on files that still import `block` (not `contract`) — the `contract` package import count is zero
**Plans**: 1/1 plans complete
Plans:
- [x] 07-01-PLAN.md -- Contract Package Elimination

### Phase 08: Persistence Wiring
**Goal**: Wire `c.w.PersistPostConsensus` into the initiator transaction flow — single call site, single file (`core/transaction.go`), PERSIST-04 only
**Depends on**: Phase 07
**Requirements**: PERSIST-04
**Scope**: Initiator flow only. `quorum_recv.go`, `unpledge.go`, `ft.go`, `split.go` are NOT touched in this phase.
**Success Criteria** (what must be TRUE):
  1. `PersistPostConsensus` is called exactly once in `initiateTransaction` in `core/transaction.go`, after `signatureTobePublished` is assembled and before `util.PublishTransaction`
  2. The call uses `ExecutionRoleInitiator`, passes `transactionInfo`, `signatureTobePublished`, and `initiatorDID` — `AffectedTokens/TokenChainRows/TokenStates` are nil (auto-derived)
  3. Failure is soft: error is logged at Error level, transaction response is NOT blocked
  4. No other files are modified — `quorum_recv.go`, `unpledge.go`, `ft.go`, `split.go` unchanged
**Plans:** 1 plan
Plans:
- [ ] 08-01-PLAN.md — Wire PersistPostConsensus call in initiateTransaction (single call site, soft-fail)

**Deferred to future phase (PERSIST-01/02/03/05):**
- PERSIST-01: Genesis minting paths (`core/token.go` — partially done in pre-work)
- PERSIST-02: Pledge/unpledge flows (`core/quorum_recv.go:1858`, `core/unpledge.go`)
- PERSIST-03: FT genesis and burn (`core/ft.go`)
- PERSIST-05: RBT split atomicity (`core/parts/split.go`)

**Pre-work done (2026-03-20, out-of-phase):**
- `models.SerializeTransactionInfo` added as the single source of truth for txInfo JSON encoding (`types/models/transaction_info.go`)
- `ComputeTransactionID` exported from `core/wallet/post_consensus_persistence.go` — uses `SerializeTransactionInfo` + `hex.EncodeToString` (printable hex, not raw bytes)
- `PersistGenesisTokenRecord` added to `core/wallet/token_chain.go` — atomic 3-insert (transactions→tokens→tokenchain) in single pgx.Tx with ON CONFLICT idempotency
- `generateTestTokens` and `generateTestTokensFaucet` in `core/token.go` replaced `block.GenesisBlock`/`CreateTokenBlock` with the new PostgreSQL flow — PERSIST-01 criterion for `core/token.go` is satisfied
- Remaining `CreateTokenBlock` calls: `core/quorum_recv.go:1858`, `core/unpledge.go:187` — still in scope for this phase

### Phase 09: Architecture Consolidation
**Goal**: `core.InitiateTransaction` is the single, verified entry point for all token types — type-specific initiators and old transfer request types are removed
**Depends on**: Phase 08
**Requirements**: ARCH-01, ARCH-02, ARCH-03
**Success Criteria** (what must be TRUE):
  1. `InitiateFTTransfer` and any other type-specific initiator functions are deleted from the codebase — no call sites reference them
  2. FT, NFT, and SC transaction initiation logic lives in `core/transaction.go` (or is removed), not in per-type files dispatching separately
  3. `model.TransferFTReq` and all other `core/model` transfer request types are removed — `models.TransactionRequest` is the only input type accepted by transaction initiation
**Plans**: TBD

### Phase 10: Stub and Cleanup
**Goal**: All remaining `block` and `contract` imports are gone, credit system is deleted, tokenchain validation and unpledge are stubbed with no-ops, and `go build ./...` passes on `release-v1` with zero errors
**Depends on**: Phase 09
**Requirements**: STUB-01, STUB-02, STUB-03, CLEANUP-01, CLEANUP-02, CLEANUP-03
**Success Criteria** (what must be TRUE):
  1. `core/token_chain_validation.go` and `core/smartcontract_tokenchain_validation.go` contain only no-op stub implementations with TODO comments — no `block` package references remain in them
  2. The unpledge sequence tracking functions (`AddUnpledgeSequenceInfo`, `GetUnpledgeSequenceDetails`, `RemoveUnpledgeSequenceInfo`) are stubbed as no-ops with TODO comments
  3. All credit system call sites (`GetCredit`, `StoreCredit`, `RemoveCredit`) and the `creditStatus` handler are deleted — no replacement stub is required
  4. Zero files in the repository import `github.com/rubixchain/rubixgoplatform/block` — verified by grep
  5. Zero files in the repository import `github.com/rubixchain/rubixgoplatform/contract` — verified by grep
  6. `go build ./...` on `release-v1` exits with code 0 and zero error lines
**Plans**: TBD

---

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 01: Wallet Foundation | v1 | - | Complete | 2026-03-19 |
| 02: BuildTransactionInfoFromRequest | v1 | - | Complete | 2026-03-19 |
| 04: Compilation Audit | v2 | - | Complete | 2026-03-19 |
| 05: Foundation Fixes | v3 | 3/3 | Complete | 2026-03-19 |
| 06: Wallet Layer | v3 | 4/4 | Complete | 2026-03-20 |
| 07: Contract Removal | v3 | 1/1 | Complete | 2026-03-20 |
| 08: Persistence Wiring | v3 | 0/1 | Planned | - |
| 09: Architecture Consolidation | v3 | 0/TBD | Not started | - |
| 10: Stub and Cleanup | v3 | 0/TBD | Not started | - |

---

*Roadmap last updated: 2026-03-20 — Phase 08 planned (1 plan, PERSIST-04 only)*
