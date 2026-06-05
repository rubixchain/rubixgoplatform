#!/bin/bash
set -e

echo "Starting Rubix node..."

cd /app   # CRITICAL — Rubix uses ./ipfs (relative path), CWD must be /app

DB_NAME=${DB_NAME:-rubix}

DATA_DIR=/app/data
CONFIG_FILE=$DATA_DIR/config.toml

mkdir -p $DATA_DIR

# -------------------------
# Network validation
# -------------------------
if [ -f "$CONFIG_FILE" ]; then
  EXISTING=$(grep network_mode $CONFIG_FILE | awk -F '"' '{print $2}')
  if [ "$EXISTING" != "$NETWORK_MODE" ]; then
    echo "ERROR: Network mismatch. Existing=$EXISTING New=$NETWORK_MODE"
    exit 1
  fi
fi

# -------------------------
# Generate config
# -------------------------
echo "Generating config..."
envsubst < /app/config.template.toml > $CONFIG_FILE

# -------------------------
# Wait for Postgres
# -------------------------
echo "Waiting for Postgres..."
until pg_isready -h $DB_HOST -p $DB_PORT -U rubix; do
  sleep 2
done

echo "Postgres ready"

# -------------------------
# Detect and clear incomplete IPFS state
#
# initIPFS() in core/ipfs.go guards on directory existence only:
#   if os.IsNotExist(os.Stat(ipfsdir)) { run ipfs init }
#
# If .ipfs/ exists but config is absent (partial/stale state from a
# previous failed run), initIPFS skips ipfs init and ensureLibp2pStreamMounting
# immediately fails with:
#   "failed to read ipfs config: open .../config: no such file or directory"
#
# Fix: remove the incomplete directory so Rubix can re-initialize cleanly.
# -------------------------
IPFS_DIR="$DATA_DIR/.ipfs"
if [ -d "$IPFS_DIR" ] && [ ! -f "$IPFS_DIR/config" ]; then
  echo "WARNING: Incomplete IPFS state detected (.ipfs/ exists but config is missing)."
  echo "Removing stale .ipfs/ so Rubix can re-initialize IPFS from scratch."
  rm -rf "$IPFS_DIR"
fi

# -------------------------
# Place swarm key in CWD (/app) where Rubix reads it
#
# initIPFS copies the key from CWD via util.Filecopy(LocalnetSwarmKeyFilename, dest)
# where LocalnetSwarmKeyFilename = "localnetswarm.key" (relative, not absolute).
# -------------------------
echo "Placing swarm key for $NETWORK_MODE"

case "$NETWORK_MODE" in
  localnet)
    cp /swarm/localnetswarm.key ./localnetswarm.key
    ;;
  testnet)
    cp /swarm/testnetswarm.key ./testnetswarm.key
    ;;
  mainnet)
    cp /swarm/swarm.key ./swarm.key
    ;;
  *)
    echo "Invalid NETWORK_MODE: $NETWORK_MODE"
    exit 1
    ;;
esac

# -------------------------
# Initialize Rubix (config.toml only — IPFS init happens during run)
# -------------------------
if [ ! -f "$DATA_DIR/initialized" ]; then
  echo "Initializing Rubix..."
  ./rubixgoplatform init -p $DATA_DIR
  touch $DATA_DIR/initialized
fi

# -------------------------
# Start Rubix (triggers ipfs init + daemon internally)
# -------------------------
echo "Starting Rubix..."
exec ./rubixgoplatform run -p $DATA_DIR
