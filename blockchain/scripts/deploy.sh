#!/usr/bin/env bash
# Deploy the misinformation chaincode to the Hyperledger Fabric test-network.
#
# Track B, milestone B4-B6: reuses the canonical fabric-samples test-network
# (Raft consensus, CouchDB state DB) as the "deliberately simple,
# platform-agnostic" PoC topology. CLI-only — the REST/api layer is deferred.
#
# Lifecycle: package -> install (all orgs) -> approve (all orgs) -> commit.
# By default a 3-org network is brought up (Raft, CouchDB) and the chaincode is
# committed with a 2-of-3 endorsement policy (OutOf(2, Org1, Org2, Org3)).
#
# Usage:
#   ./scripts/deploy.sh            # 3-org network + deploy chaincode (2-of-3 policy)
#   ./scripts/deploy.sh -2         # 2-org baseline only (AND(Org1, Org2))
#   ./scripts/deploy.sh down       # tear down the network (containers + artifacts)

set -euo pipefail

# jq is required by the fabric lifecycle scripts; add a user-local install if present.
if ! command -v jq >/dev/null 2>&1 && [ -x "${HOME}/.local/bin/jq" ]; then
  export PATH="${HOME}/.local/bin:${PATH}"
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: 'jq' not found. Install it (e.g. jq-linux-amd64 -> ~/.local/bin/jq)." >&2
  exit 1
fi

# Prefer the modern `docker compose` plugin over the legacy `docker-compose`
# (v1.29.2 shadows it on some machines and cannot handle the current test-network
# compose files, which makes the peers get torn down mid-deploy).
if docker compose version >/dev/null 2>&1; then
  export CONTAINER_CLI_COMPOSE="docker compose"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CHAINCODE_PATH="${PROJECT_ROOT}/chaincode/misinformation/go"
FABRIC_SAMPLES="${FABRIC_SAMPLES:-${HOME}/fabric-samples}"
TEST_NETWORK="${FABRIC_SAMPLES}/test-network"

CC_NAME="misinformation"
CC_VERSION="2.1"
CC_SEQUENCE="1"
CC_SRC_LANGUAGE="go"
CHANNEL_NAME="mychannel"

# State database: CouchDB (rich JSON/Mango queries for the API + public
# dashboard, v2 §6). Set STATE_DB="" for LevelDB.
STATE_DB="${STATE_DB:--s couchdb}"
STATE_DB_ARGS=${STATE_DB}

# Endorsement policy. 2-org baseline = AND(Org1, Org2) (both orgs must endorse).
# 3-org default = OutOf(2, Org1MSP, Org2MSP, Org3MSP) — a 2-of-3 quorum,
# applied by onboarding org3 below.
CC_ENDORSEMENT_POLICY=""
THREE_ORG="1"   # set to "" for the 2-org baseline only

cd "${TEST_NETWORK}"

case "${1:-up}" in
  down)
    echo ">> Tearing down the network..."
    ./network.sh down
    exit 0
    ;;
  -2)
    THREE_ORG=""
    ;;
esac

echo ">> Starting the ${THREE_ORG:+3}${THREE_ORG:-2}-org test network (Raft consensus)..."
# shellcheck disable=SC2086
./network.sh up ${STATE_DB_ARGS}

echo ">> Creating channel ${CHANNEL_NAME}..."
./network.sh createChannel -c "${CHANNEL_NAME}"

echo ">> Deploying ${CC_NAME} v${CC_VERSION} (${CC_SRC_LANGUAGE})..."
./network.sh deployCC -c "${CHANNEL_NAME}" \
  -ccn "${CC_NAME}" \
  -ccv "${CC_VERSION}" \
  -ccs "${CC_SEQUENCE}" \
  -ccl "${CC_SRC_LANGUAGE}" \
  -ccp "${CHAINCODE_PATH}" \
  ${CC_ENDORSEMENT_POLICY}

if [ -n "${THREE_ORG}" ]; then
  echo ">> Onboarding org3 + switching to a 2-of-3 endorsement policy..."
  "${SCRIPT_DIR}/onboard-org3.sh"
fi

echo
if [ -n "${THREE_ORG}" ]; then
  echo ">> Network up and chaincode ${CC_NAME} committed (3-org, OutOf(2,...) policy)."
else
  echo ">> Network up and chaincode ${CC_NAME} committed (2-org, AND policy)."
fi
echo
