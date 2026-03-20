# Analysis: Missing config.StorageConfig / config.ServiceConfig Build Failure

**Task:** 260320-h3o
**Date:** 2026-03-20
**Confidence:** HIGH (backed by git history, grep-confirmed zero callers, go build verification)

---

## 1. Build Errors

Two compile-time errors block the build:

| Error | File | Line | Symbol |
|-------|------|------|--------|
| `config.StorageConfig undefined` | `client/config.go` | 9 | `*config.StorageConfig` parameter type |
| `config.ServiceConfig undefined` | `client/services.go` | 9 | `*config.ServiceConfig` parameter type |

Both errors are in the same package (`client/`) and are the **only two build errors** currently preventing `go build ./...` from succeeding.

---

## 2. Root Cause

Commit `955228f` ("refactored config") rewrote `core/config/config.go` from scratch, replacing the old runtime DB configuration system with a TOML-based startup-time configuration.

**What was removed in 955228f:**

| Removed item | Location |
|---|---|
| `StorageConfig` struct | `core/config/config.go` |
| `ServiceConfig` struct | `core/config/config.go` |
| `ConfigData`, `Config`, `Ports`, `DIDConfigType`, `IPFSRecoveryConfig` structs | `core/config/config.go` |
| `core/config.go` (core-side HTTP handler for `/api/setup-db`) | `core/config.go` |
| `command/services.go` (command-side caller of `SetupService`) | `command/services.go` |
| `server/config.go::APISetupDB` function body | `server/config.go` |

**What the refactor missed:**

Two consumer files in `client/` were **not deleted** despite the entire server-side infrastructure being removed:

- `client/config.go` — HTTP client wrapper calling a now-deleted endpoint
- `client/services.go` — HTTP client wrapper calling a now-deleted endpoint

**Why these types have no replacement:**

`StorageConfig` and `ServiceConfig` were part of an old runtime DB configuration API — they allowed a running node to receive HTTP POST requests to configure its database connection dynamically. In the new PostgreSQL architecture:

- Database configuration happens at **startup time** via `config.toml`
- PostgreSQL connection details live in `types.UserConfig.Db` and `types.RubixConfig.DBConfig`
- There is **no runtime API to change DB configuration**

These types are not accidentally missing. They are **intentionally obsolete** with no equivalent in the new architecture.

---

## 3. Call Graph

### client/config.go — Client.SetupDB

```
client/config.go:9   Client.SetupDB(sc *config.StorageConfig)
  POST to setup.APISetupDB == "/api/setup-db"
    server/server.go:115  s.AddRoute(setup.APISetupDB, "POST", s.APISetupDB)
      s.APISetupDB — METHOD DOES NOT EXIST (deleted in 955228f)
        core/config.go — DELETED in 955228f (was the handler)
```

**Callers of Client.SetupDB:** ZERO — confirmed by `grep -rn '\.SetupDB\(' command/ grpcserver/`

### client/services.go — Client.SetupService

```
client/services.go:9  Client.SetupService(scfg *config.ServiceConfig)
  POST to setup.APISetupService == "/api/setup-service"
    server handler — NOT FOUND (never existed or also deleted)
```

**Callers of Client.SetupService:** ZERO — confirmed by `grep -rn '\.SetupService\(' command/ grpcserver/`

---

## 4. Dead Code Inventory

These items exist in the codebase but are unreachable from any active code path. They are a direct consequence of the same refactor that introduced the build errors.

| File | Location | Item | Severity |
|------|----------|------|----------|
| `server/server.go` | Line 115 | `s.AddRoute(setup.APISetupDB, "POST", s.APISetupDB)` — references method `s.APISetupDB` which does not exist | **Latent runtime panic** — currently masked by build failure in client/ |
| `setup/setup.go` | Line 46 | `APISetupDB = "/api/setup-db"` string constant | Harmless dead constant |
| `setup/setup.go` | Line 32 | `APISetupService = "/api/setup-service"` string constant | Harmless dead constant |
| `command/command.go` | Line 64 | `SetupDBCmd = "setupdb"` string constant | Harmless dead constant |
| `command/command.go` | Lines 141, 145 | `SetupDBCmd` entries in command name/help slice | Dead help-text entries |

**Note on server/server.go:115:** The route registration passes `s.APISetupDB` as a method value. Go will compile this without error if the method is referenced as a value (not called at compile time). However, at runtime, when the HTTP router attempts to invoke this handler, it will panic on a nil function value or fail to resolve the method. This is a **latent runtime bug** that is currently invisible because the build stops before linking.

---

## 5. Recommended Fix

**Strategy: Delete the two orphaned client files**

Do NOT attempt to restore `StorageConfig` / `ServiceConfig` — these concepts do not exist in the PostgreSQL architecture and restoring them would create dead types that nothing uses.

### Primary action (fixes build errors)

| Action | File | Lines |
|--------|------|-------|
| **DELETE** | `client/config.go` | 17 lines |
| **DELETE** | `client/services.go` | 16 lines |

These two deletions are sufficient to fix both build errors. The `client/` package has 18 other active files that are unaffected.

### Optional cleanup (separate follow-up tasks)

| Action | File | Rationale |
|--------|------|-----------|
| Remove route registration | `server/server.go:115` | Eliminates latent runtime panic from nonexistent method reference |
| Remove dead constants | `setup/setup.go:32,46` | Code hygiene |
| Remove dead command entries | `command/command.go:64,141,145` | Code hygiene |

---

## 6. Risk Assessment

| Risk | Assessment |
|------|------------|
| Breaking active callers | **ZERO risk** — confirmed zero callers of both functions across entire codebase |
| Breaking client/ package compilation | **ZERO risk** — 18 other files in client/ are unaffected |
| Functional regression | **ZERO risk** — server-side handlers were already deleted; the API has not worked since 955228f |
| Hidden import graph effects | **ZERO risk** — client/config.go and client/services.go import only `config` and `setup` packages, which remain in the codebase |

---

## 7. Follow-up Concerns

### config.Config type alias mismatch (separate investigation required)

The refactored `core/config/config.go` defines:

```go
type Config = types.RubixConfig
```

This type alias is consumed by multiple active files:

| File | Usage |
|------|-------|
| `grpcclient/command.go:55` | `cfg config.Config` |
| `grpcserver/grpc.go:43` | `cfg *config.Config` |
| `tools/config_converter.go` | config.Config fields |
| `core/ipfs_health.go` | config.Config fields |
| `core/ipfs_recovery.go` | config.Config fields |
| `wrapper/ensweb/` | config.Config fields |

The **old** `config.Config` struct had fields: `NodeAddress`, `NodePort`, `NodeConfigDir`, `CfgData ConfigData`.
The **new** `types.RubixConfig` has a completely different field layout.

Once the client/ build errors are fixed and compilation proceeds further, these callers may produce additional build errors or runtime field-access panics depending on how they use the type. This needs a separate investigation pass after the primary fix is applied.

**Recommended next task:** After deleting the two orphaned files, run `go build ./...` again and capture any new errors. If `config.Config` field mismatches surface, create a new quick task to trace and fix them.
