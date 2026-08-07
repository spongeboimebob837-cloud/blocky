#!/usr/bin/env bash
# Register every org as a Tier-2 stakeholder on the ledger (genesis bootstrap).
#
# The chaincode allows the first `foundingOrgLimit` (3) orgs to self-register via
# RegisterOrg; each registered org is then allowed to submit reports and vote.
# This replaces the manual Step 6.5 heredoc with one repeatable command.
#
# Usage: ./scripts/register-orgs.sh [org1 org2 org3 ...]   (default: org1 org2 org3)
#
# Prereqs: network up + chaincode deployed (./scripts/deploy.sh).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEST_NETWORK="${FABRIC_SAMPLES:-${HOME}/fabric-samples}/test-network"

CHANNEL="mychannel"
CC="misinformation"

ORGS=("${@:-org1 org2 org3}")

export PATH="${TEST_NETWORK}/../bin:${PATH}"
export FABRIC_CFG_PATH="${TEST_NETWORK}/../config"
export CORE_PEER_TLS_ENABLED=true

ORDERER_TLS="${TEST_NETWORK}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"
ORG1_TLS="${TEST_NETWORK}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt"
ORG2_TLS="${TEST_NETWORK}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt"
ORG3_TLS="${TEST_NETWORK}/organizations/peerOrganizations/org3.example.com/peers/peer0.org3.example.com/tls/ca.crt"

# use_org <org1|org2|org3> switches the signing identity + peer address.
use_org() {
  local org="$1"
  case "${org}" in
    org1)
      export CORE_PEER_LOCALMSPID="Org1MSP"
      export CORE_PEER_TLS_ROOTCERT_FILE="${ORG1_TLS}"
      export CORE_PEER_MSPCONFIGPATH="${TEST_NETWORK}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp"
      export CORE_PEER_ADDRESS="localhost:7051"
      ;;
    org2)
      export CORE_PEER_LOCALMSPID="Org2MSP"
      export CORE_PEER_TLS_ROOTCERT_FILE="${ORG2_TLS}"
      export CORE_PEER_MSPCONFIGPATH="${TEST_NETWORK}/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp"
      export CORE_PEER_ADDRESS="localhost:9051"
      ;;
    org3)
      export CORE_PEER_LOCALMSPID="Org3MSP"
      export CORE_PEER_TLS_ROOTCERT_FILE="${ORG3_TLS}"
      export CORE_PEER_MSPCONFIGPATH="${TEST_NETWORK}/organizations/peerOrganizations/org3.example.com/users/Admin@org3.example.com/msp"
      export CORE_PEER_ADDRESS="localhost:11051"
      ;;
    *)
      echo "ERROR: unknown org '${org}' (expected org1|org2|org3)" >&2
      exit 1
      ;;
  esac
}

for org in "${ORGS[@]}"; do
  echo ">> RegisterOrg (${org})"
  use_org "${org}"
  payload=$(python3 -c "import json,sys; print(json.dumps({'function':'RegisterOrg','Args':[]}))")
  peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com \
    --tls --cafile "${ORDERER_TLS}" \
    -C "${CHANNEL}" -n "${CC}" \
    --peerAddresses localhost:7051 --tlsRootCertFiles "${ORG1_TLS}" \
    --peerAddresses localhost:9051 --tlsRootCertFiles "${ORG2_TLS}" \
    --waitForEvent -c "${payload}" >/dev/null
done

echo ">> ListRegisteredOrgs"
payload=$(python3 -c "import json,sys; print(json.dumps({'function':'ListRegisteredOrgs','Args':[]}))")
peer chaincode query -C "${CHANNEL}" -n "${CC}" -c "${payload}"
