#!/usr/bin/env bash
# Start a local IPFS node (Docker ipfs/kubo) for off-chain report storage.
#
# The API gateway prefers IPFS (off_chain_uri = ipfs://<CID>) and falls back to
# SQLite when this node is down. Uses the kubo repo's default HTTP RPC port 5001.
#
# Usage:
#   ./scripts/start-ipfs.sh    # start (or no-op if already running)
#   ./scripts/start-ipfs.sh down   # stop the container

set -euo pipefail

CONTAINER="${IPFS_CONTAINER:-ipfs-node}"
IMAGE="${IPFS_IMAGE:-ipfs/kubo:latest}"

if [[ "${1:-up}" == "down" ]]; then
  echo ">> Stopping IPFS container ${CONTAINER}..."
  docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
  exit 0
fi

if docker ps --format '{{.Names}}' | grep -qx "${CONTAINER}"; then
  echo ">> IPFS node already running (${CONTAINER})."
  exit 0
fi

if docker ps -a --format '{{.Names}}' | grep -qx "${CONTAINER}"; then
  echo ">> Starting existing IPFS container ${CONTAINER}..."
  docker start "${CONTAINER}"
else
  echo ">> Starting new IPFS node ${CONTAINER} (${IMAGE})..."
  docker run -d --name "${CONTAINER}" \
    -p 5001:5001 \
    -p 8080:8080 \
    --restart unless-stopped \
    "${IMAGE}"
fi

echo ">> Waiting for the RPC API on :5001..."
for _ in $(seq 1 30); do
  if curl -sf http://localhost:5001/api/v0/version >/dev/null 2>&1; then
    echo ">> IPFS ready: http://localhost:5001"
    exit 0
  fi
  sleep 1
done

echo "ERROR: IPFS did not become ready on :5001" >&2
exit 1
