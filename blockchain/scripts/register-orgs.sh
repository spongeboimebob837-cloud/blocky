#!/usr/bin/env bash
# Register every available org as a Tier-2 stakeholder on the ledger (genesis bootstrap).
#
# The chaincode allows the first `foundingOrgLimit` orgs to self-register via
# RegisterOrg; each registered org is then allowed to submit reports and vote.
# This replaces the manual Step 6.5 heredoc with one repeatable command.
#
# Usage:
#   ./scripts/register-orgs.sh                       # register org1..orgN (auto-detect)
#   ./scripts/register-orgs.sh --limit N             # assume founding limit N, register org1..N
#   ./scripts/register-orgs.sh org1 org2 org3        # register the named orgs only
#
# Auto-detection walks org1, org2, ... up to the limit (or as far as crypto exists)
# and registers every org whose MSP material is present under the test-network.
#
# Prereqs: network up + chaincode deployed (./scripts/deploy.sh).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEST_NETWORK="${FABRIC_SAMPLES:-${PROJECT_ROOT}/fabric-samples}/test-network"

CHANNEL="mychannel"
CC="misinformation"

LIMIT=0
ORGS=()

# parse args: --limit N must come before any org names.
while [ "$#" -gt 0 ]; do
  case "$1" in
    --limit)
      shift
      LIMIT="${1:-0}"
      if ! [[ "${LIMIT}" =~ ^[0-9]+$ ]] || [ "${LIMIT}" -lt 1 ]; then
        echo "ERROR: --limit must be a positive integer, got '${LIMIT}'" >&2
        exit 1
      fi
      ;;
    *)
      ORGS+=("$1")
      ;;
  esac
  shift
done

# If no orgs were named explicitly, build the org1..orgN list by checking which
# orgs' admin MSP dirs actually exist (org1 always does).
if [ "${#ORGS[@]}" -eq 0 ]; then
  MAX="${LIMIT:-3}"
  for i in $(seq 1 "${MAX}"); do
    msp_dir="${TEST_NETWORK}/organizations/peerOrganizations/org${i}.example.com/users/Admin@org${i}.example.com/msp"
    if [ -d "${msp_dir}" ]; then
      ORGS+=("org${i}")
    fi
  done
  if [ "${#ORGS[@]}" -eq 0 ]; then
    echo "ERROR: no org MSP material found under ${TEST_NETWORK}/organizations" >&2
    exit 1
  fi
fi

export PATH="${TEST_NETWORK}/../bin:${PATH}"
export FABRIC_CFG_PATH="${TEST_NETWORK}/../config"
export CORE_PEER_TLS_ENABLED=true

ORDERER_TLS="${TEST_NETWORK}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"

# use_org <org1|org2|...> switches the signing identity + peer address. Peer
# ports follow the fabric-samples convention: orgN peer0 = 7051 + 2000*(N-1).
use_org() {
  local org="$1"
  local n="${org#org}"
  if ! [[ "${n}" =~ ^[0-9]+$ ]]; then
    echo "ERROR: unknown org '${org}' (expected org<N>)" >&2
    exit 1
  fi
  local msp_dir="${TEST_NETWORK}/organizations/peerOrganizations/${org}.example.com/users/Admin@${org}.example.com/msp"
  if [ ! -d "${msp_dir}" ]; then
    echo "ERROR: no admin MSP for ${org} (${msp_dir} missing) — add the org crypto first" >&2
    exit 1
  fi
  local port=$((7051 + 2000 * (n - 1)))
  export CORE_PEER_LOCALMSPID="Org${n}MSP"
  export CORE_PEER_TLS_ROOTCERT_FILE="${TEST_NETWORK}/organizations/peerOrganizations/${org}.example.com/peers/peer0.${org}.example.com/tls/ca.crt"
  export CORE_PEER_MSPCONFIGPATH="${msp_dir}"
  export CORE_PEER_ADDRESS="localhost:${port}"
}

# org1's TLS root cert is the standard first endorser for invokes.
ORG1_TLS="${TEST_NETWORK}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt"

for org in "${ORGS[@]}"; do
  echo ">> RegisterOrg (${org})"
  use_org "${org}"
  payload=$(python3 -c "import json,sys; print(json.dumps({'function':'RegisterOrg','Args':[]}))")
  peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com \
    --tls --cafile "${ORDERER_TLS}" \
    -C "${CHANNEL}" -n "${CC}" \
    --peerAddresses localhost:7051 --tlsRootCertFiles "${ORG1_TLS}" \
    --peerAddresses localhost:9051 --tlsRootCertFiles "${TEST_NETWORK}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt" \
    --waitForEvent -c "${payload}" >/dev/null
done

echo ">> ListRegisteredOrgs"
payload=$(python3 -c "import json,sys; print(json.dumps({'function':'ListRegisteredOrgs','Args':[]}))")
use_org "${ORGS[0]}"
peer chaincode query -C "${CHANNEL}" -n "${CC}" -c "${payload}"
