---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: completed
stopped_at: Completed quick task 260320-ig1
last_updated: "2026-03-20T08:00:00.000Z"
last_activity: 2026-03-20 — Deleted grpcserver/, grpcclient/, protos/; removed gRPC dependencies from go.mod (260320-ig1)
progress:
  total_phases: 6
  completed_phases: 3
  total_plans: 9
  completed_plans: 6
  percent: 67
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-19)

**Core value:** Every dev can build `release-v1` without errors — PostgreSQL is the only persistence layer
**Current focus:** Phase 08 -- Persistence Wiring (next phase to plan)

## Current Position

Phase: 08 in progress; Plan 08-01 complete
Status: Phase 08-01 complete — PersistPostConsensus wired in initiateTransaction (PERSIST-04 satisfied)
Last activity: 2026-03-20 - Completed quick task 260320-hvl: Remove dead route registrations (APIInitiateRBTTransfer, APIInitiatePinRBT) from server/server.go

Progress: [███████░░░] 67% (phases 01, 02, 04, 05, 06, 07 complete; 08 in progress; 09-10 pending)

## Performance Metrics

**Velocity:**
- Total plans completed: (v1 + v2 plans — tracked per milestone)
- Average duration: not tracked across milestones
- Total execution time: not tracked across milestones

**By Phase:**

| Phase | Plans | Status |
|-------|-------|--------|
| 01 | Complete | v1 |
| 02 | Complete | v1 |
| 04 | Complete | v2 |
| Phase 05 P01 | 2min | 2 tasks | 3 files | Complete |
| Phase 05 P02 | 64s | 2 tasks | 3 files | Complete |
| Phase 05 P03 | 113s | 2 tasks | 4 files | Complete |
| Phase 06 | ~25min | 4 plans | 14 files | Complete |
| Phase 07 | ~2hrs | 17 quick tasks | multiple files | Complete |
| Phase 08 pre-work | ~30min | out-of-band | 4 files | SerializeTransactionInfo + PersistGenesisTokenRecord + genesis token paths |
| Phase 08-persistence-wiring P01 | 3min | 1 tasks | 1 files |

## Accumulated Context

### Decisions (v3)

- Credit system: REMOVE entirely — no PostgreSQL migration, no stub
- Unpledge sequence: STUB — dev team owns implementation; no-op with TODO
- Token chain validation: STUB — dev team owns implementation; no-op with TODO
- Canonical tokenchain role strings: `constants.TokenRole_*` already defined in `constants/constants.go:42-52`
- contract.TokenInfo replacement: ContractTokenInfo is a core-local type in core/contract_types.go; NOT merged with models.TokenInfo (different field sets) — used in consensus/quorum paths only (quick-260320-5ix)
- IsDIDExist: single bool return — log error internally, do not surface to caller
- util.VerifySignature / util.SignTransaction in `util/transaction.go` replace contract.VerifySignature
- All token types use `models.TokenInfo` uniformly — no separate FT/NFT/SC structs
- core.InitiateTransaction (models.TransactionRequest) is the single entry point — type-specific initiators removed
- InitNewTokenDenomArrayForDID stubbed with return nil — dev team owns the INSERT implementation (05-01)
- APISyncTransactionChain = "/api/sync-transaction-chain" placed in core.go const block after APISyncTokenChain (05-02)
- SyncTransactionChainRequest struct has Did string only -- matches transaction.go usage; dev-team to expand fields (05-02)
- TokenStatus_Generated=13, Transferred=14, Fetched=15 added; all existing values unchanged; lifecycle grouping applied (05-02)
- wallet.TokenIs* replaced directly with constants.TokenStatus_* — no aliases or stubs; wallet import preserved in all 4 files (05-03)
- AddTransactionHistory stub uses *model.TransactionDetails matching all existing call sites (06)
- SyncStatus added as transient int field db:- to models.Token satisfying quorum_validation.go access patterns (06)
- LegacyStorageStub replaces all c.s.* SQLite calls in dead-code ft.go with no-op implementations (06)
- models.SerializeTransactionInfo is the single source of truth for all txInfo JSON encoding — no direct json.Marshal(txInfo) at call sites (Phase 08 pre-work)
- ComputeTransactionID is exported from core/wallet/post_consensus_persistence.go; uses SerializeTransactionInfo + hex.EncodeToString (hex string, not raw bytes — printable and DB-safe) (Phase 08 pre-work)
- PersistGenesisTokenRecord (core/wallet/token_chain.go) is the genesis-specific atomic 3-insert (transactions→tokens→tokenchain in single pgx.Tx); ON CONFLICT idempotency on all three tables — for genesis paths only; full consensus paths continue to use PersistPostConsensus (Phase 08 pre-work)
- generateTestTokens and generateTestTokensFaucet no longer use block package — replaced with SerializeTransactionInfo + PvtSign + ComputeTransactionID + PersistGenesisTokenRecord; ipfsnode import removed from core/token.go (Phase 08 pre-work)
- Remaining CreateTokenBlock call sites NOT yet fixed: core/quorum_recv.go:1858, core/unpledge.go:187 — these are in scope for Phase 08
- PersistPostConsensus call site wired in initiateTransaction (08-01): positioned after signatureTobePublished assembly, before util.PublishTransaction; soft-fail (log only, no early return); PERSIST-04 satisfied (08-01)

### Blockers/Concerns

- Phase 06 prerequisite for Phase 07 — wallet methods must exist before contract.TokenInfo callers are updated (DONE)
- contract/ package eliminated (quick-260320-5ix) — Phase 07 contract removal scope complete ahead of schedule
- Phase 08 prerequisite for Phase 09 — persistence must be wired before architecture consolidation removes type-specific paths
- RBT split atomicity (PERSIST-05) is the highest-risk item in Phase 08 — test carefully
- core/network.go still imports block package (line 5) — one of the CLEANUP-01 files for Phase 10
- core/fullnode.go block import removed (quick task 260320-4ok); processSingleTransaction stubbed with TODO; StoreFailedTransaction/GetAllFailedToSyncTokens/AddTransactionsToFullNodeTransactionHistoryTable/ReadFullNodeTransactionHistoryTable/UpdateFullNodeTransactionHistoryTable added as no-op stubs in ft_stubs.go; EventTransaction.BlockHash/AssetType fields added to types/models/events.go
- core/parts/genesis_transaction.go:46 still has json.Marshal(txInfo) — out of scope for Phase 08 pre-work but needs fixing before Phase 08 complete (grep check: grep -rn "json.Marshal(txInfo)" core/ should return 0)

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260320-4ok | Fix core/fullnode.go: eliminate block package dependency without introducing new logic | 2026-03-20 | 02ba77d | [260320-4ok-fix-core-fullnode-go-eliminate-block-pac](./quick/260320-4ok-fix-core-fullnode-go-eliminate-block-pac/) |
| 260320-4zf | Fix missing contract package: create contract/ stub satisfying all 11 importers | 2026-03-20 | f3c3fb2 | [260320-4zf-fix-missing-contract-package-in-core-ft-](./quick/260320-4zf-fix-missing-contract-package-in-core-ft-/) |
| 260320-59w | Remove core/ft_transfer_optimized.go and dead pool types (TokenInfoPool, TokenSlicePool) | 2026-03-20 | 4e991da | [260320-59w-evaluate-and-remove-core-ft-transfer-opt](./quick/260320-59w-evaluate-and-remove-core-ft-transfer-opt/) |
| 260320-5ix | Eliminate contract package: internalize types to core, delete contract/ directory | 2026-03-20 | d3bd38b | [260320-5ix-eliminate-contract-package-dependency-cl](./quick/260320-5ix-eliminate-contract-package-dependency-cl/) |
| 260320-6jw | Refactor syncTokenChain: replace GetAllTokenBlocks with GetTokenChainByTokenID (PostgreSQL) | 2026-03-20 | 871dd58 | [260320-6jw-refactor-first-getalltokenblocks-usage-i](./quick/260320-6jw-refactor-first-getalltokenblocks-usage-i/) |
| 260320-76v | Replace soft tokenchain linkage warning with hard error return | 2026-03-20 | 13d57c1 | [260320-76v-replace-soft-tokenchain-linkage-warning-](./quick/260320-76v-replace-soft-tokenchain-linkage-warning-/) |
| 260320-78x | Fix GetTokenChainByTokenID: add missing previous_transaction_id and id to SELECT | 2026-03-20 | 8ca90ab | [260320-78x-fix-gettokenchainbytokenid-add-missing-p](./quick/260320-78x-fix-gettokenchainbytokenid-add-missing-p/) |
| 260320-7aa | Add position contiguity validation to syncTokenChain tokenchain invariant checks | 2026-03-20 | 4e4ba6d | [260320-7aa-add-position-gap-validation-to-synctoken](./quick/260320-7aa-add-position-gap-validation-to-synctoken/) |
| 260320-7fy | Refactor GetMissingBlockSequence: remove block traversal, use PostgreSQL tokenchain validation | 2026-03-20 | 0cd3b56 | [260320-7fy-refactor-getmissingblocksequence-in-core](./quick/260320-7fy-refactor-getmissingblocksequence-in-core/) |
| 260320-7vr | Stub all 14 functions in token_chain_validation.go; remove block package dependency | 2026-03-20 | 6e77069 | [260320-7vr-refactor-token-chain-validation-go-remov](./quick/260320-7vr-refactor-token-chain-validation-go-remov/) |
| 260320-85w | Replace interface{} with TokenChainInput typed placeholder; add util.StrToHex | 2026-03-20 | a3b8dc2 | — |
| 260320-8kl | Refactor core/ft.go to remove block package: PersistGenesisTokenRecord + ReadToken | 2026-03-20 | 60db81d | [260320-8kl-refactor-core-ft-go-to-remove-block-base](./quick/260320-8kl-refactor-core-ft-go-to-remove-block-base/) |
| 260320-a78 | Stub core/quorum_validation.go; remove block package dependency (13 functions) | 2026-03-20 | 8887b35 | [260320-a78-stub-core-quorum-validation-go-remove-bl](./quick/260320-a78-stub-core-quorum-validation-go-remove-bl/) |
| 260320-a78+ | Restore missing core types (ConensusRequest, QuorumDIDPeerMap, Token, AllToken); add util.HexToStr | 2026-03-20 | 2faf82a | — |
| 260320-art | Stub core/recover.go: remove block package dependency (initiateRecoverRBT, recoverPinnedToken) | 2026-03-20 | 87460ab | [260320-art-stub-core-recover-go-remove-block-depend](./quick/260320-art-stub-core-recover-go-remove-block-depend/) |
| 260320-b1c | Stub smartcontract_tokenchain_validation.go: remove block import; stub ValidateSmartContractTokenChain/Block/ValidateTxnInitiator | 2026-03-20 | b474fdc | — |
| 260320-b1c+ | Add wallet stubs GetSmartContractTokenByDeployer + GetSmartContractToken | 2026-03-20 | e18401b | — |
| 260320-bkj | Remove block package dependency from core/token.go; add BlockStub wallet stubs | 2026-03-20 | 3a1c339 | [260320-bkj-remove-block-dependency-from-core-token-](./quick/260320-bkj-remove-block-dependency-from-core-token-/) |
| 260320-ffj | Reconcile GSD planning files with actual code state after 17 quick tasks | 2026-03-20 | b50e046 | [260320-ffj-reconcile-gsd-planning-files-with-actual](./quick/260320-ffj-reconcile-gsd-planning-files-with-actual/) |
| 260320-h3o | Analyze missing config types causing build failure (NO CODE CHANGES) | 2026-03-20 | 34f1344 | [260320-h3o-analyze-missing-config-types-causing-bui](./quick/260320-h3o-analyze-missing-config-types-causing-bui/) |
| 260320-hf8 | Delete dead client/config.go + client/services.go; remove APISetupDB route from server.go | 2026-03-20 | d758f82 | [260320-hf8-remove-dead-client-config-service-files-](./quick/260320-hf8-remove-dead-client-config-service-files-/) |
| 260320-hvl | Remove dead route registrations (APIInitiateRBTTransfer, APIInitiatePinRBT) from server/server.go | 2026-03-20 | 3af2a4b | [260320-hvl-remove-dead-server-route-registrations-p](./quick/260320-hvl-remove-dead-server-route-registrations-p/) |
| 260320-ig1 | Remove gRPC layer completely from build: delete grpcserver/, grpcclient/, protos/; clean go.mod | 2026-03-20 | effc8cd | [260320-ig1-remove-grpc-layer-completely-from-build](./quick/260320-ig1-remove-grpc-layer-completely-from-build/) |

## Session Continuity

Last session: 2026-03-20T08:00:00Z
Stopped at: Completed quick task 260320-ig1
Resume file: None
