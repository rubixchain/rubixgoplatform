# test/integration — Rubix integration test harness

Committed, CI-facing integration tests for the Rubix node. Brings up a 3-node
cluster (nodeA, nodeB, quorum) backed by PostgreSQL via Docker Compose, then
drives the nodes through the HTTP API (`/rubix/v1/...`) and asserts correctness
across every asset subsystem: RBT, FT, NFT, smart contracts, plus bundled /
all-in-one / intra-node flows.

This harness is what `.github/workflows/docker-integration.yml` runs. It is the
committed, stable counterpart to the local-only `e2e_stress/` scratchpad.

## Layout

```
test/integration/
├── runner.py            # entry point: arg parsing, env lifecycle, exit code
├── config.py / config.json
├── clients/             # HTTP + DB clients (node-facing, Docker-agnostic)
│   ├── api_client.py    # NodeClient — HTTP wrapper for /rubix/v1 endpoints
│   └── db_validator.py  # DBValidator — per-node PostgreSQL integrity checks
├── engines/             # asset operation logic (Docker-agnostic)
│   ├── minter.py shuttle.py reporter.py file_selector.py callback_receiver.py
│   ├── nft_engine.py smart_contract_engine.py ft_engine.py
│   └── bundled_engine.py intra_node_engine.py
├── env/                 # cluster lifecycle — the Docker-vs-external seam
│   ├── base.py          # Environment interface (setup/teardown)
│   └── docker_env.py    # DockerEnvironment — compose up/down
├── tests/
│   └── happy_path.py    # StressRunner — the full happy-path suite
└── test_contracts/      # .wasm + .rs fixtures for smart contract tests
```

Tests are unaware of *how* the cluster exists. `env/` owns that, so the same
suites can later run against already-running nodes (`--no-docker`, a future
external environment) without changing test logic.

## Running locally (from repo root)

```bash
pip install -r test/integration/requirements.txt

# Fast smoke — every subsystem at minimal token counts:
python3 -m test.integration.runner --micro-test --run-all-tests

# Full run-all (heavier):
python3 -m test.integration.runner --run-all-tests

# Against already-running nodes (skip Docker up/down):
python3 -m test.integration.runner --no-docker --run-all-tests
```

Requires Docker (with compose) for the default path. The runner builds the node
image from `test/docker/rubix/Dockerfile` and uses `test/docker/docker-compose.stress.yml`.

## Native mode (no Docker — macOS / Windows)

`--native` runs the same suites against a cluster of **native host processes**
instead of containers: three local PostgreSQL instances plus three
`rubixgoplatform` processes. This is for platforms where GitHub-hosted runners
can't run the Linux node containers (macOS has no Docker; Windows can't run Linux
containers). Test logic is unchanged — only the cluster lifecycle (`env/`) differs.

```bash
# Build the node binary for your OS first (repo root):
make compile-mac        # macOS  (or compile-linux / compile-windows)

# Run natively (no Docker):
python3 -m test.integration.runner --native \
    --config test/integration/config.native.json --small-test --run-all-tests
```

Prerequisites:
- **PostgreSQL** installed and on PATH (e.g. `brew install postgresql@16` on
  macOS). The harness finds `initdb`/`pg_ctl` on PATH or in common install
  locations and manages three throwaway instances itself.
- The **`rubixgoplatform` binary** built via `make compile-<os>` (auto-detected:
  `mac/`, `linux/`, or `windows/`).
- Network access on first run to fetch the pinned **kubo** (IPFS) build for your
  OS/arch; it's cached under `test/native/.kubo/` thereafter.

Native mode uses the binary's own `node_index`-derived ports (no Docker
remapping): API **20000/20001/20002**, Postgres **5433/5434/5435** — which is why
it needs `--config test/integration/config.native.json` (the default
`config.json` targets the Docker-mapped ports). All runtime state lives under
`test/native/` (gitignored). `--native` is mutually exclusive with `--no-docker`.

## Exit code

The runner exits non-zero when **any** verification check fails or the run
raises — so CI catches assertion failures. (The suite logs failed checks but
does not itself raise; `runner.py` reads `StressRunner.verification_failed`.)

## Output artifacts (under `test/docker/data/stress/logs/`, gitignored)

- `transactions.jsonl` — every transaction as a JSON line
- `summary.json` — latency stats, success/fail counts, final balances
- `verification.json` — per-check PASS/FAIL results
- `db_snapshot_<ts>.txt` — per-DB diagnostic snapshot

## Key flags

| Flag | Purpose |
|------|---------|
| `--micro-test` / `--small-test` | Smaller token counts / fewer txns (fast) |
| `--run-all-tests` | RBT + NFT + SC + FT + bundled + all-in-one + intra-node |
| `--nft-count N` / `--nft-only` / `--nft-self-execute` / `--nft-transfer` / `--nft-cross-execute` | NFT scope |
| `--sc-count N` / `--sc-execute` / `--sc-only` | Smart contract scope |
| `--ft-count N` / `--ft-only` / `--ft-transfer` | FT scope |
| `--ft-parts-tests` / `--ft-parts-only` | FT minted out of PART RBTs + token_denom accounting (auto-enabled by `--run-all-tests`; `--ft-parts-only` runs it alone against a fresh wallet) |
| `--bundled-test` / `--all-in-one-test` / `--intra-node-test` | Combined flows |
| `--skip-ft` | Disable all FT-dependent paths (composes with `--run-all-tests`) |
| `--no-docker` | Don't manage Docker; nodes already running |
| `--config PATH` | Custom JSON config |

Run `python3 -m test.integration.runner --help` for the complete list.
```
