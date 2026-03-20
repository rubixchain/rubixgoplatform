---
phase: 08-persistence-wiring
plan: 01
subsystem: database
tags: [postgres, pgx, persistence, transaction, consensus]

# Dependency graph
requires:
  - phase: 08-pre-work
    provides: PersistPostConsensus implementation in core/wallet/post_consensus_persistence.go
provides:
  - PersistPostConsensus call site in initiateTransaction (PERSIST-04 satisfied)
  - Live PostgreSQL persistence for all initiated transactions at the consensus finalization point
affects: [09-architecture-consolidation, InitiateTransaction callers, Phase 08 remaining plans]

# Tech tracking
tech-stack:
  added: []
  patterns: [soft-fail persistence pattern — log error, never block transaction broadcast]

key-files:
  created: []
  modified:
    - core/transaction.go

key-decisions:
  - "PersistPostConsensus placed between signatureTobePublished assembly and util.PublishTransaction — persist before broadcast is the canonical ordering"
  - "Soft-fail: persistence error logged at Error level, execution falls through to util.PublishTransaction — transaction response is never blocked by DB failure"
  - "AffectedTokens, TokenChainRows, TokenStates left nil — auto-derived by BuildPersistencePayload inside PersistPostConsensus"
  - "Transaction field left nil — auto-built from TransactionInfo + Signature inside coordinator"

patterns-established:
  - "Soft-fail persistence: if err := c.w.PersistPostConsensus(...); err != nil { c.log.Error(...) } — no return, no panic"
  - "Initiator-role call site uses wallet.ExecutionRoleInitiator constant, not a raw string"

requirements-completed: [PERSIST-04]

# Metrics
duration: 3min
completed: 2026-03-20
---

# Phase 08 Plan 01: Persistence Wiring — Initiator Call Site Summary

**PersistPostConsensus wired into initiateTransaction with soft-fail semantics: persist-before-broadcast ordering, single call site, PERSIST-04 satisfied**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-03-20T06:10:00Z
- **Completed:** 2026-03-20T06:13:00Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishments

- Added `core/wallet` import to `core/transaction.go`
- Inserted `c.w.PersistPostConsensus` call between `signatureTobePublished` assembly and `util.PublishTransaction`
- PostgreSQL persistence is now live for all initiated transactions — the persistence coordinator had zero callers before this plan
- Soft-fail: DB failure is logged at Error level and execution continues to broadcast unchanged

## Task Commits

Each task was committed atomically:

1. **Task 1: Wire PersistPostConsensus call in initiateTransaction** - `955b84d` (feat)

**Plan metadata:** (final docs commit — see below)

## Files Created/Modified

- `core/transaction.go` — Added `core/wallet` import and `PersistPostConsensus` call site (10 lines inserted)

## Decisions Made

- Soft-fail pattern: persistence error logs at Error level but does not block transaction broadcast. This ensures a DB outage cannot prevent RBT transfers from completing.
- Only four required fields passed to `PostConsensusPersistenceRequest` (`TransactionInfo`, `Signature`, `DID`, `ExecutionRole`) — optional slice fields auto-derived by the coordinator.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

The `client/` package had two pre-existing build errors (`undefined: config.StorageConfig`, `undefined: config.ServiceConfig`) unrelated to this change. `go build ./core/...` passes cleanly. These are CLEANUP-01 scope items.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- PERSIST-04 is satisfied: initiator persistence is live
- Phase 08 remaining plans (quorum receiver, unpledge, split) can proceed
- Phase 09 architecture consolidation prerequisite (persistence must be wired) is now partially met

## Self-Check: PASSED

- core/transaction.go: FOUND
- commit 955b84d: FOUND (feat(08-01): wire PersistPostConsensus call in initiateTransaction)
- go build ./core/...: PASS

---
*Phase: 08-persistence-wiring*
*Completed: 2026-03-20*
