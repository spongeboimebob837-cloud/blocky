# Colab / Kaggle Deployment Packaging (Track C, C2) + API Gateway (v2 §4)

The proposal lists "Colab/Kaggle deployment" as one of the four implementation areas.
Because the Fabric *network* runs on the local machine (Docker), the Colab/Kaggle
notebooks only need the **Python bridge**, not a live peer. A FastAPI **gateway**
(v2 §4) now wraps that same bridge for org-facing access.

## What goes into the notebook runtime

1. `app/src/blockchain.py` — the `FabricBridge` / `Prediction` module
   (pure Python stdlib; already validated). 
2. `app/src/report.py` — canonical report schema + whole-object hash (stdlib).
3. A mounted/bundled copy of the `peer` CLI + the org MSP material if the notebook
   itself must submit. Otherwise the notebook exports predictions to JSONL and the
   local machine anchors them (recommended for PoC).
4. `requirements.txt` — the **bridge** needs no extra dependency (`hashlib`, `json`,
   `subprocess`, `dataclasses` are stdlib). Only the **API gateway**
   (`app/api/requirements.txt`) needs `fastapi`/`uvicorn`/`pydantic`.

## Two supported modes

### Mode A — notebook only predicts (recommended, PoC)
The fine-tuning/eval notebook writes `[{report_id, text, label, confidence}]` rows.
A local step anchors them via the bridge or the API gateway:

```bash
# via the bridge (shells out to peer CLI)
python3 - <<'EOF'
from blockchain import FabricBridge
b = FabricBridge(org="org1")
# NOTE: v2 keys by report_id and requires off_chain_uri.
b.submit_report("nso-42-v1", "nso", "<64-hex-hash>", "1", 0.94,
                "afroxlmr-large-nso-v1.0", "2026-01-10T09:00:00Z",
                "https://server/api/reports/nso-42-v1")
EOF
```

### Mode B — notebook submits directly via the API gateway
Requires the gateway running and an API key (see below).

## API gateway (v2 §4)

```bash
pip install -r apps/ai_service/v1-0-0/app/api/requirements.txt

# optional: off-chain reports go to a local IPFS node (falls back to SQLite)
blockchain/scripts/start-ipfs.sh

# mint the first API keys (chicken-and-egg: /api/orgs/apply needs a key already)
blockchain/scripts/bootstrap-keys.sh org1 org2 org3

cd apps/ai_service/v1-0-0/app/api
uvicorn server:app --host 0.0.0.0 --port 8000
```

Endpoints (auth: `X-API-Key` header):

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/orgs/apply` | Issue an API key mapped to a signing org |
| POST | `/api/orgs/{msp}/admission` | Request Tier-2 stakeholder status |
| POST | `/api/orgs/{msp}/admission/vote` | Vote accept/reject on an applicant |
| POST | `/api/orgs/{msp}/admission/finalize` | Finalize an admission |
| GET  | `/api/orgs` | List registered stakeholder orgs |
| GET  | `/api/status` | Active off-chain storage backend (IPFS or SQLite fallback) |
| POST | `/api/reports` | Store off-chain + hash + SubmitReport |
| GET  | `/api/reports/{id}` | Fetch full off-chain report |
| GET  | `/api/reports/{id}/chain` | On-chain record (QueryReport) |
| GET  | `/api/reports?status=&language=` | Filtered report list |
| POST | `/api/reports/{id}/vote` | CastVote |
| POST | `/api/reports/{id}/finalize` | FinalizeReport |
| POST | `/api/reports/{id}/expire` | ExpireReport |
| GET  | `/api/reports/{id}/verify` | **Tamper-evidence demo**: off-chain hash == on-chain hash |
| GET  | `/api/reports/{id}/history` | Ledger key history |

The gateway also runs a **scheduled expiry job** (v2 §3.3) that marks overdue
PENDING reports EXPIRED (chaincode is purely reactive and can't run timers itself).

## Off-chain storage (v2 §3.4, implemented)

Full report objects (including raw text) are stored off-chain — **IPFS by
default**, SQLite as a fallback (`app/api/offchain.db`). On-chain only the hash +
`off_chain_uri` are stored. `off_chain_uri` becomes `ipfs://<CID>` when IPFS is
available, else the API URL. `GET /api/status` reports the active backend.

## Checklist (aligns with proposal section 4)

- [x] Pipeline produces `{report_id, text, label, confidence}` (data_prep.py)
- [x] Bridge hashes raw text with sha256 and never sends raw text (DATA_MODEL.md §1.a)
- [x] Bridge is stdlib-only so Colab/Kaggle needs no extra `pip install`
- [x] Gateway stores the full report off-chain and anchors only hash + URI (v2 §3)
- [x] `/verify` proves off-chain copy matches the on-chain hash (tamper-evidence demo)
- [x] Immutability + provenance enforced chaincode-side (MSP from tx context)
- [x] Expiry of undecided reports handled by the API scheduler + chaincode (v2 §3.3)
- [x] Verified stakeholder admission via on-chain voting (v2 §2)