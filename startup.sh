#!/usr/bin/env bash
# startup.sh — the v3 "start switch" for performance/stress experiments.
#
# The single entry point of the whole simulation: invoke it with variable args,
# and it brings the network up, deploys the chaincode, runs a load phase,
# records metrics, and tears everything down — all driven by the args you pass.
#
# Every numeric variable is DYNAMIC: pass any value within its domain (bounded by
# a hard max for the single-host test-network). Omitted args fall back to the
# defaults below. Values above a max are rejected with a clear error.
#
# Usage:
#   ./startup.sh --help
#   ./startup.sh                                        # all defaults
#   ./startup.sh --orgs 6 --concurrency 150 --rate 300  # dynamic values
#   ./startup.sh --orgs 3 --state-db leveldb --driver http --samples 500
#
# Environment overrides (before flags): FABRIC_SAMPLES, STATE_DB (for deploy.sh),
# CRYPTO_STYLE (for the onboarding gate), API_BASE_URL (load target).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCH_DIR="${SCRIPT_DIR}/benchmarks"
RESULTS_CSV="${BENCH_DIR}/results.csv"

# --------------------------------------------------------------------------- #
# Variable table: name default max_validator  (validator is a shell func)
# --------------------------------------------------------------------------- #
ORGS=3
PEERS_PER_ORG=1
ORDERERS=3
SERVER_SPEC="baseline"
BATCH_TIMEOUT="2s"
MAX_MESSAGE=10
STATE_DB_OPT=""          # "" -> deploy.sh default (couchdb); "leveldb" unsets -s
CONCURRENCY=24
RATE=25
RW_MIX=100               # % writes (0..100)
POLICY="2of3"
FAULT="none"
DRIVER="http"
SAMPLES=200
RUN_TAG="baseline"

MAX_ORGS=20
MAX_PEERS_PER_ORG=3
MAX_ORDERERS=5
MAX_BATCH_TIMEOUT_S=200
MAX_MAX_MESSAGE=1600
MAX_CONCURRENCY=200
MAX_RATE=400
MAX_SAMPLES=100000

usage() {
  cat <<'EOF'
startup.sh — v3 start switch for stress experiments.

  --orgs N            founding org limit (total), default 3, max 20
  --peers-per-org N   peers per org, default 1, max 3 (requires compose override)
  --orderers N        ordering nodes, default 3, max 5 (Raft; requires 3+ for FT)
  --server SPEC       host tag recorded per run (baseline | big), default baseline
  --batch-timeout D   block-cut timeout (e.g. 2s, 5s), default 2s, max 200s
  --max-message N     max tx per block, default 10, max 1600
  --state-db DB       couchdb (default) | leveldb
  --concurrency N     parallel workers, default 24, max 200
  --rate N            target submit rate (TPS), default 25, max 400 (stretch 3000)
  --rw-mix PCT        % writes in the workload (0=read-only..100=write-only), default 100
  --policy P          2of3 (default) | 3of3
  --crypto-style S    ecdsa (default) | ed25519 | rsa  (onboarding gate; CRYPTO_STYLE env)
  --fault F           none (default) | kill-peer | kill-orderer
  --driver D          http (default) | caliper
  --samples N         load samples/requests to drive, default 200
  --tag T             run tag recorded in results.csv, default baseline
  --help              show this help
EOF
  exit 0
}

# parse_flags: consume --key value pairs; unknown keys error out.
parse_flags() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --help) usage ;;
      --orgs) shift; ORGS="$1" ;;
      --peers-per-org) shift; PEERS_PER_ORG="$1" ;;
      --orderers) shift; ORDERERS="$1" ;;
      --server) shift; SERVER_SPEC="$1" ;;
      --batch-timeout) shift; BATCH_TIMEOUT="$1" ;;
      --max-message) shift; MAX_MESSAGE="$1" ;;
      --state-db) shift; STATE_DB_OPT="$1" ;;
      --concurrency) shift; CONCURRENCY="$1" ;;
      --rate) shift; RATE="$1" ;;
      --rw-mix) shift; RW_MIX="$1" ;;
      --policy) shift; POLICY="$1" ;;
      --crypto-style) shift; CRYPTO_STYLE="$1" ;;
      --fault) shift; FAULT="$1" ;;
      --driver) shift; DRIVER="$1" ;;
      --samples) shift; SAMPLES="$1" ;;
      --tag) shift; RUN_TAG="$1" ;;
      *) echo "ERROR: unknown flag '$1' (try --help)" >&2; exit 1 ;;
    esac
    shift
  done
}

is_num() { [[ "$1" =~ ^[0-9]+$ ]]; }

# validate: enforce the hard maxes for every numeric variable.
validate() {
  local err=""
  is_num "$ORGS"        || err+="--orgs must be an integer (got '$ORGS')\n"
  is_num "$PEERS_PER_ORG" || err+="--peers-per-org must be an integer\n"
  is_num "$ORDERERS"    || err+="--orderers must be an integer\n"
  is_num "$MAX_MESSAGE" || err+="--max-message must be an integer\n"
  is_num "$CONCURRENCY" || err+="--concurrency must be an integer\n"
  is_num "$RATE"        || err+="--rate must be an integer\n"
  is_num "$RW_MIX"      || err+="--rw-mix must be an integer\n"
  is_num "$SAMPLES"     || err+="--samples must be an integer\n"
  [ -n "$err" ] && { echo -e "ERROR: ${err}" >&2; exit 1; }

  [ "$ORGS" -ge 1 ] && [ "$ORGS" -le "$MAX_ORGS" ] || { echo "ERROR: --orgs must be 1..$MAX_ORGS" >&2; exit 1; }
  [ "$PEERS_PER_ORG" -ge 1 ] && [ "$PEERS_PER_ORG" -le "$MAX_PEERS_PER_ORG" ] || { echo "ERROR: --peers-per-org must be 1..$MAX_PEERS_PER_ORG" >&2; exit 1; }
  [ "$ORDERERS" -ge 1 ] && [ "$ORDERERS" -le "$MAX_ORDERERS" ] || { echo "ERROR: --orderers must be 1..$MAX_ORDERERS" >&2; exit 1; }
  [ "$MAX_MESSAGE" -ge 1 ] && [ "$MAX_MESSAGE" -le "$MAX_MAX_MESSAGE" ] || { echo "ERROR: --max-message must be 1..$MAX_MAX_MESSAGE" >&2; exit 1; }
  [ "$CONCURRENCY" -ge 1 ] && [ "$CONCURRENCY" -le "$MAX_CONCURRENCY" ] || { echo "ERROR: --concurrency must be 1..$MAX_CONCURRENCY" >&2; exit 1; }
  [ "$RATE" -ge 1 ] && [ "$RATE" -le "$MAX_RATE" ] || { echo "ERROR: --rate must be 1..$MAX_RATE" >&2; exit 1; }
  [ "$RW_MIX" -ge 0 ] && [ "$RW_MIX" -le 100 ] || { echo "ERROR: --rw-mix must be 0..100" >&2; exit 1; }
  [ "$SAMPLES" -ge 1 ] && [ "$SAMPLES" -le "$MAX_SAMPLES" ] || { echo "ERROR: --samples must be 1..$MAX_SAMPLES" >&2; exit 1; }

  case "$BATCH_TIMEOUT" in
    *[!0-9smh]*|"") echo "ERROR: --batch-timeout must be a duration like 2s" >&2; exit 1 ;;
  esac
  case "$STATE_DB_OPT" in couchdb|leveldb|"") ;; *) echo "ERROR: --state-db must be couchdb|leveldb" >&2; exit 1 ;; esac
  case "$POLICY" in 2of3|3of3) ;; *) echo "ERROR: --policy must be 2of3|3of3" >&2; exit 1 ;; esac
  case "$CRYPTO_STYLE" in ecdsa|ed25519|rsa|"") ;; *) echo "ERROR: --crypto-style must be ecdsa|ed25519|rsa" >&2; exit 1 ;; esac
  case "$FAULT" in none|kill-peer|kill-orderer) ;; *) echo "ERROR: --fault must be none|kill-peer|kill-orderer" >&2; exit 1 ;; esac
  case "$DRIVER" in http|caliper) ;; *) echo "ERROR: --driver must be http|caliper" >&2; exit 1 ;; esac
}

# resolve a duration string to whole seconds (2s -> 2, 500ms -> 1, 5m -> 300).
duration_seconds() {
  local v="$1"
  case "$v" in
    *ms) echo "$(( (${v%ms} + 999) / 1000 ))" ;;
    *s) echo "${v%s}" ;;
    *m) echo "$(( ${v%m} * 60 ))" ;;
    *h) echo "$(( ${v%h} * 3600 ))" ;;
    *) echo "${v}" ;;
  esac
}

parse_flags "$@"
# apply env fallbacks (CRYPTO_STYLE is read by the gate itself).
CRYPTO_STYLE="${CRYPTO_STYLE:-ecdsa}"
validate

BATCH_TIMEOUT_SEC="$(duration_seconds "$BATCH_TIMEOUT")"
[ "$BATCH_TIMEOUT_SEC" -le "$MAX_BATCH_TIMEOUT_S" ] || { echo "ERROR: --batch-timeout max ${MAX_BATCH_TIMEOUT_S}s" >&2; exit 1; }

mkdir -p "${BENCH_DIR}"

# --------------------------------------------------------------------------- #
# 1. Provision + deploy
# --------------------------------------------------------------------------- #
echo ">> [1/5] Provisioning ${ORGS}-org network (state_db=${STATE_DB_OPT:-couchdb}, policy=${POLICY})..."
DEPLOY_ARGS=()
if [ "$ORGS" -ne 3 ]; then
  DEPLOY_ARGS+=(--orgs "$ORGS")
fi
if [ "$STATE_DB_OPT" = "leveldb" ]; then
  export STATE_DB=""   # deploy.sh: no -s couchdb flag -> LevelDB
fi
case "$POLICY" in
  3of3) export CC_ENDORSEMENT_POLICY="-ccep OR((&(Org1MSP.member)(Org2MSP.member)(Org3MSP.member)))" ;;
  *) : ;;  # 2of3 is deploy.sh's default after onboarding org3
esac
"${SCRIPT_DIR}/blockchain/scripts/deploy.sh" "${DEPLOY_ARGS[@]:-up}"

# --------------------------------------------------------------------------- #
# 2. Load phase
# --------------------------------------------------------------------------- #
echo ">> [2/5] Load phase (driver=${DRIVER}, concurrency=${CONCURRENCY}, rate=${RATE}, rw_mix=${RW_MIX}, samples=${SAMPLES})..."

API_BASE_URL="${API_BASE_URL:-http://localhost:8000}"
if [ "$DRIVER" = "caliper" ]; then
  echo ">> Caliper driver requested — ensure a Caliper benchmark is configured; skipping HTTP load."
  LOAD_OUT=""
else
  LOAD_OUT="$("${BENCH_DIR}/load-http.py" \
    --base "${API_BASE_URL}" \
    --concurrency "${CONCURRENCY}" \
    --rate "${RATE}" \
    --rw-mix "${RW_MIX}" \
    --samples "${SAMPLES}" 2>&1)"
  echo "$LOAD_OUT"
fi

# --------------------------------------------------------------------------- #
# 3. Fault injection
# --------------------------------------------------------------------------- #
if [ "$FAULT" != "none" ]; then
  echo ">> [3/5] Fault injection: ${FAULT}"
  case "$FAULT" in
    kill-peer) docker stop peer0.org1.example.com >/dev/null 2>&1 || echo "  (peer container not running)";;
    kill-orderer) docker stop orderer.example.com >/dev/null 2>&1 || echo "  (orderer container not running)";;
  esac
  sleep 5
  echo ">> Restarting ${FAULT} target..."
  case "$FAULT" in
    kill-peer) docker start peer0.org1.example.com >/dev/null 2>&1 || true ;;
    kill-orderer) docker start orderer.example.com >/dev/null 2>&1 || true ;;
  esac
fi

# --------------------------------------------------------------------------- #
# 4. Record
# --------------------------------------------------------------------------- #
echo ">> [4/5] Recording to ${RESULTS_CSV}"
"${BENCH_DIR}/record.py" \
  --csv "${RESULTS_CSV}" \
  --tag "${RUN_TAG}" \
  --orgs "$ORGS" \
  --peers-per-org "$PEERS_PER_ORG" \
  --orderers "$ORDERERS" \
  --server "$SERVER_SPEC" \
  --batch-timeout "$BATCH_TIMEOUT" \
  --max-message "$MAX_MESSAGE" \
  --state-db "${STATE_DB_OPT:-couchdb}" \
  --policy "$POLICY" \
  --crypto-style "$CRYPTO_STYLE" \
  --concurrency "$CONCURRENCY" \
  --rate "$RATE" \
  --rw-mix "$RW_MIX" \
  --fault "$FAULT" \
  --driver "$DRIVER" \
  --load-out "$LOAD_OUT"

echo ">> [5/5] Done. Teardown with:  ${SCRIPT_DIR}/blockchain/scripts/deploy.sh down"
echo "     (or run 'docker-compose -f blockchain/fabric-samples/test-network/compose/compose-test-net.yaml down' manually)"
