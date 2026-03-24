#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Rubix dev runner
# Builds cmd/dev, ensures Postgres is up on :5500, resets DB, then runs.
# ---------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
RUN_DIR="/tmp/rubix-run"
BIN="/tmp/rubix-dev"
CONTAINER="rubix-node0-postgres"
PG_PORT=5500
PG_USER=rubix
PG_PASS=rubixpass
PG_DB=rubix

# ---- 1. Build ---------------------------------------------------------------
echo "==> Building cmd/dev..."
cd "$REPO_ROOT"
go build -ldflags="-linkmode=external" -o "$BIN" ./cmd/dev/

# macOS 26+ requires a valid code signature on every binary
if [[ "$(uname)" == "Darwin" ]]; then
  echo "==> Signing binary (macOS)..."
  codesign -s - -f "$BIN"
fi

echo "    Binary: $BIN"

# ---- 2. Postgres ------------------------------------------------------------
echo "==> Checking Postgres on port $PG_PORT..."

pg_ready() {
  nc -z localhost "$PG_PORT" 2>/dev/null
}

if pg_ready; then
  echo "    Postgres already listening on :$PG_PORT"
else
  if ! docker info > /dev/null 2>&1; then
    echo "ERROR: Postgres is not running and Docker daemon is not reachable."
    echo "       Start Docker Desktop and try again."
    exit 1
  fi

  CONTAINER_STATE=$(docker inspect --format '{{.State.Status}}' "$CONTAINER" 2>/dev/null || echo "missing")

  case "$CONTAINER_STATE" in
    running)
      echo "ERROR: Container '$CONTAINER' is running but port $PG_PORT is not open — check port mapping."
      exit 1
      ;;
    exited|created|paused)
      echo "    Starting existing container '$CONTAINER'..."
      docker start "$CONTAINER"
      ;;
    missing)
      echo "    Creating container '$CONTAINER' on port $PG_PORT..."
      docker run -d \
        --name "$CONTAINER" \
        -e POSTGRES_USER="$PG_USER" \
        -e POSTGRES_PASSWORD="$PG_PASS" \
        -e POSTGRES_DB="$PG_DB" \
        -p "${PG_PORT}:5432" \
        postgres:16
      ;;
    *)
      echo "    Container '$CONTAINER' is in state '$CONTAINER_STATE' — attempting start..."
      docker start "$CONTAINER"
      ;;
  esac

  echo -n "    Waiting for Postgres"
  for i in $(seq 1 30); do
    if pg_ready; then
      echo " ready."
      break
    fi
    echo -n "."
    sleep 1
    if [[ $i -eq 30 ]]; then
      echo ""
      echo "ERROR: Postgres did not become ready in 30 seconds."
      exit 1
    fi
  done
fi

# ---- 3. Reset DB ------------------------------------------------------------
echo "==> Resetting DB (truncate all dev tables)..."
PGPASSWORD="$PG_PASS" psql -h localhost -p "$PG_PORT" -U "$PG_USER" -d "$PG_DB" -c \
  "TRUNCATE dids, tokens, tokenchain, tokenchain_index, transactions, transaction_units, local_test_token_info RESTART IDENTITY CASCADE;"
echo "    DB reset OK"

# ---- 4. Kill any lingering IPFS daemon from a previous run ------------------
pkill -f "$RUN_DIR/ipfs daemon" 2>/dev/null || true
sleep 1

# ---- 5. Run dir + config ----------------------------------------------------
mkdir -p "$RUN_DIR/config"
#rm -rf "$RUN_DIR/.ipfs"
rm -rf "$RUN_DIR/localnet/dids"
cp "$REPO_ROOT/config/config.toml" "$RUN_DIR/config/config.toml"

# IPFS binary (looked up as ./ipfs relative to CWD)
if [[ -f "$REPO_ROOT/ipfs" ]]; then
  cp "$REPO_ROOT/ipfs" "$RUN_DIR/ipfs"
  chmod +x "$RUN_DIR/ipfs"
  # macOS 26+ requires a valid code signature on every binary
  if [[ "$(uname)" == "Darwin" ]]; then
    echo "==> Signing ipfs binary (macOS)..."
    codesign -s - -f "$RUN_DIR/ipfs"
  fi
else
  echo "WARNING: $REPO_ROOT/ipfs not found — IPFS will not start"
fi

# Swarm key for localnet private network
if [[ -f "$REPO_ROOT/localnetswarm.key" ]]; then
  cp "$REPO_ROOT/localnetswarm.key" "$RUN_DIR/localnetswarm.key"
fi

# ---- 6. Run -----------------------------------------------------------------
echo "==> Starting rubix dev node (cwd: $RUN_DIR)..."
echo "    Press Ctrl-C to stop."
cd "$RUN_DIR"
exec "$BIN"
