# How This Project Works, and How to Test It with Real Data

First: breathe. Nothing here is complicated once you see the flow. This file is
the plain-English version. If you want the long academic version, read
`blockchain/explanation` (that's the "how everything works" file) and
`blockchain/chaincode/misinformation/DATA_MODEL.md` (the data model).

## 1. What This Project Is (in one paragraph)

It's a fact-checking system with two halves:

- (a) an AI/NLP pipeline that reads tweets (English + Sepedi/isiZulu) and
  predicts whether each one is misinformation (label "1") or reliable
  (label "0"); and
- (b) a permissioned Hyperledger Fabric blockchain where several
  "fact-checking organisations" submit those predictions as reports and
  then VOTE on each other's reports. Once >= 2/3 of the orgs have voted,
  the report is finalised and locked forever (immutable).

The point of the blockchain part: prove that nobody tampered with a report and
record WHO submitted and voted on what (an audit trail).

Real tweet text NEVER goes on the blockchain. Only a SHA-256 "fingerprint"
(hash) of it goes on-chain. The full text lives in a normal database
(SQLite) next to the API server, and the ledger stores a link to it.

## 2. The Big Picture (the data flow)

```
tweets (real data) --> AI model predicts {label, confidence}
     --> a registered org builds a Report object (full text)
     --> stores the full report in SQLite (off-chain) + computes its hash
     --> submits ONLY {report_id, hash, label, confidence, ...} to the ledger
     --> report is PENDING
     --> every registered org votes accept/reject (one vote per org)
     --> once >= 2/3 of registered orgs have voted -> report becomes
         FINAL (accepted) or REJECTED. After that it is IMMUTABLE.
     --> anyone can call /verify to prove the stored copy matches the hash.
```

There are THREE layers:

| Layer | Description | Location |
| ----- | ----------- | -------- |
| Layer 1 | AI pipeline | `apps/ai_service/v1-0-0/` (Python + notebooks) |
| Layer 2 | Bridge/API | `apps/ai_service/v1-0-0/app/src/blockchain.py`<br>`apps/ai_service/v1-0-0/app/api/` (FastAPI server) |
| Layer 3 | Blockchain | `blockchain/chaincode/misinformation/go/` (the Go chaincode that runs inside Fabric) |

## 3. Where Everything Lives (file map)

```
Repo root /
  README.md                         - just the project title
  updates.md                        - design decision log (v1 -> v2 -> v3).
                                      NOTE: some code examples in it are
                                      OUTDATED (old function signatures).
  instructions.md                   - this file
  blockchain/
    explanation                     - the "how it all works" write-up
    chaincode/misinformation/
      DATA_MODEL.md                 - data model + lifecycle (decision record)
      DEPLOYMENT.md                 - Colab/Kaggle packaging notes
      go/misinformation.go          - THE CHAINCODE (all ledger logic)
      go/misinformation_test.go     - unit tests (run without a network)
      go/mockstub_test.go           - fake Fabric for testing
    scripts/
     deploy.sh                     - start the 3-org network + deploy chaincode (2-of-3)
     onboard-org3.sh               - add/regenerate a 3rd org + 2-of-3 voting
     register-orgs.sh              - register orgs on-chain (genesis bootstrap)
     bootstrap-keys.sh             - mint the gateway's first API keys
     start-ipfs.sh                 - start/stop the off-chain IPFS node
     smoke-test.sh                 - end-to-end CLI test with a fake report
  apps/ai_service/v1-0-0/
    app/
      api/server.py                 - FastAPI gateway (orgs talk to this over HTTP)
      api/storage.py                - off-chain reports (IPFS + SQLite fallback) + API keys
      src/blockchain.py             - Python bridge that calls the peer CLI
      src/report.py                 - builds Report objects + canonical hashing
      src/data_prep.py              - anonymizes tweet text (anonymize_text)
      src/preprocessing.py          - text cleaning helpers
    data/raw/                       - YOUR REAL DATASET (~1.8 GB, gitignored)
      twitter_data_eng_raw_anon.csv      # columns: Unnamed:0, set, text, label, source
      twitter_data_nso_raw_anon.csv      # columns: row_id, text, translated, back_translated, source, label
      complete_translations_and_backtranslations.jsonl
    requirements.txt                - python deps for the pipeline
    app/api/requirements.txt        - deps for the API server (fastapi/uvicorn)
```

A Python virtualenv is already set up at the repo root:
`venv_A_Blockchain_Enabled_Framework_for_Misinformation_Monitoring/`

Fabric tooling lives INSIDE the repo at `blockchain/fabric-samples` (already
installed: Go 1.22, Docker 29, jq, peer CLI, Python 3.12 all present).

## 4. First, a 60-Second Sanity Check (no network needed)

The chaincode has unit tests that run entirely in memory (no Docker). Run:

```bash
cd blockchain/chaincode/misinformation/go
go test ./...
```

You should see `ok ... misinformation` (all tests pass). These tests cover
register -> submit -> vote -> finalize, quorum math, and rejections. This is
proof the ledger logic is correct without touching the network.

## 5. Start the Blockchain Network + Run the Built-in Test

This takes a couple of minutes the first time (it pulls Docker images).

```bash
cd blockchain
./scripts/deploy.sh          # brings up 3 orgs + deploys the chaincode (2-of-3)
./scripts/smoke-test.sh      # runs register -> submit -> vote -> finalize
                             # with a fake report, end to end, on the live net
```

If you see "Smoke test complete." everything below will work too.

For the 2-org baseline only (AND policy, quick tests):
`cd blockchain && ./scripts/deploy.sh -2`

To stop everything later: `cd blockchain && ./scripts/deploy.sh down`

## 6. The Main Event: Test with Real Data

This is the full system demo. It reads REAL tweets from your dataset, puts them
through the real API gateway (which stores the full text off-chain on IPFS,
hashes it, and submits the hash to the real ledger), then has the orgs vote and
finalise, and finally proves nothing was tampered with.

### Step 6.1 - Network up (if not already running)

```bash
cd blockchain
./scripts/deploy.sh
```

### Step 6.2 - Install API server deps (one-time; venv already has them, skip if ok)

```bash
venv_A_Blockchain_Enabled_Framework_for_Misinformation_Monitoring/bin/pip install -r apps/ai_service/v1-0-0/app/api/requirements.txt
```

### Step 6.2.5 - (Optional) Start the IPFS off-chain storage node

Reports are stored off-chain on a local IPFS node by default
(`off_chain_uri = ipfs://<CID>`). Without it the gateway falls back to SQLite,
which still works for testing.

```bash
cd blockchain
./scripts/start-ipfs.sh        # down stops it
```

### Step 6.3 - Bootstrap API keys

The API requires an `X-API-Key` header on every request, and the very first key
can only be minted by writing straight into the SQLite DB (a known chicken-
and-egg in the current code). One command now does it:

```bash
cd blockchain
./scripts/bootstrap-keys.sh org1 org2 org3   # keys: key-org1 / key-org2 / key-org3
```

(`-r` also registers the orgs on the ledger, combining with Step 6.5.)

### Step 6.4 - Start the API server (keep this terminal open)

```bash
cd apps/ai_service/v1-0-0/app/api
../../../../../venv_A_Blockchain_Enabled_Framework_for_Misinformation_Monitoring/bin/uvicorn server:app --host 0.0.0.0 --port 8000
```

Wait for "Application startup complete". This server also runs a background
job that auto-expires reports nobody voted on after 72h. Check which storage
backend is live:
`curl -s -H "X-API-Key: key-org1" http://localhost:8000/api/status`

### Step 6.5 - Register the orgs on the ledger

Must be a registered org to submit/vote. The chaincode allows the first 3 orgs
to self-register:

```bash
cd blockchain
./scripts/register-orgs.sh    # registers org1, org2, org3
```

(Or via the Python bridge CLI:
`cd apps/ai_service/v1-0-0/app/src && python3 -m blockchain register-all`)

### Step 6.6 - Submit, vote, finalise, and verify REAL tweets

Edit the file path if you want a different CSV. This reads the actual tweet
text and the dataset's ground-truth label from `twitter_data_eng_raw_anon.csv`,
then:

- org1 submits the first 5 rows as PENDING reports (full text -> IPFS/SQLite,
  hash -> ledger)
- org1 and org2 each vote accept on every report
- org2 finalises (2/2 registered orgs voted -> quorum met -> FINAL)
- `/verify` recomputes the hash and compares it to the immutable on-chain one

```bash
cd apps/ai_service/v1-0-0
../../../venv_A_Blockchain_Enabled_Framework_for_Misinformation_Monitoring/bin/python - <<'EOF'
import csv, json, urllib.request

def api(method, path, body=None, key="key-org1"):
    req = urllib.request.Request(
        f"http://localhost:8000{path}", method=method,
        data=json.dumps(body).encode() if body else None,
        headers={"Content-Type": "application/json", "X-API-Key": key})
    with urllib.request.urlopen(req) as r:
        return json.loads(r.read().decode())

csv_path = "data/raw/twitter_data_eng_raw_anon.csv"
rows = []
with open(csv_path, encoding="utf-8") as f:
    for i, row in enumerate(csv.DictReader(f)):
        if i >= 5: break
        rows.append(row)

report_ids = []
for i, row in enumerate(rows):
        rid = f"real-tweet-{i}"
        api("POST", "/api/reports", {
            "report_id": rid, "language": "eng",
            "label": str(row["label"]), "confidence": 0.95,
            "model_version": "afroxlmr-large-eng-v1.0",
            "raw_text": row["text"]}, key="key-org1")
        report_ids.append(rid)

for rid in report_ids:
    api("POST", f"/api/reports/{rid}/vote", {"verdict": "accept"}, key="key-org1")
    api("POST", f"/api/reports/{rid}/vote", {"verdict": "accept"}, key="key-org2")

for rid in report_ids:
    api("POST", f"/api/reports/{rid}/finalize", None, key="key-org2")

for rid in report_ids:
    print(rid, "->", json.dumps(api("GET", f"/api/reports/{rid}/verify", key="key-org1"), indent=2))
EOF
```

Every report should print `"verified": true`. That is the whole system working
with real data: off-chain storage + on-chain hash + consortium vote + a
tamper-evidence proof.

### Step 6.7 - Look around (optional)

```bash
# The on-chain record for one report:
curl -s -H "X-API-Key: key-org1" http://localhost:8000/api/reports/real-tweet-0/chain | python3 -m json.tool
# The full off-chain report (with the real text):
curl -s -H "X-API-Key: key-org1" http://localhost:8000/api/reports/real-tweet-0 | python3 -m json.tool
# The ledger audit trail of who wrote what:
curl -s -H "X-API-Key: key-org1" http://localhost:8000/api/reports/real-tweet-0/history | python3 -m json.tool
```

## 7. The Quick Bridge Alternative (no API server needed)

If you don't want to bother with the HTTP server, the Python bridge talks to the
ledger directly. Same data flow minus the SQLite step:

```bash
cd apps/ai_service/v1-0-0/app/src
../../../../../venv_A_Blockchain_Enabled_Framework_for_Misinformation_Monitoring/bin/python - <<'EOF'
import csv, datetime as dt
from blockchain import FabricBridge

rows = []
with open("../../data/raw/twitter_data_eng_raw_anon.csv", encoding="utf-8") as f:
    for i, row in enumerate(csv.DictReader(f)):
        if i >= 3: break
        rows.append(row)

b1 = FabricBridge(org="org1", endorsers=["org1", "org2"])
b2 = FabricBridge(org="org2", endorsers=["org1", "org2"])
b1.register_org(); b2.register_org()

for i, row in enumerate(rows):
    rid = f"bridge-real-{i}"
    text = row["text"]
    content_hash = __import__("hashlib").sha256(text.encode()).hexdigest()
    ts = dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    b1.submit_report(rid, "eng", content_hash, str(row["label"]),
                     0.95, "afroxlmr-large-eng-v1.0", ts,
                     f"https://server/api/reports/{rid}")
    b1.cast_vote(rid, "accept")
    b2.cast_vote(rid, "accept")
    b2.finalize_report(rid)
    print(rid, "->", b1.query_report(rid)["status"])
EOF
```

Note: here we hash only the raw text. The API path (section 6) hashes the
WHOLE report object, which is the stronger, documented approach.

## 8. When You Want to See Misinformation Classified

The dataset has a ground-truth `label` column (0 = reliable, 1 = misinformation).
Try submitting a mix of rows and have org1 vote "accept" and org2 vote "reject"
on one of them; the report will come back REJECTED when finalised. That's the
consortium disagreement feature working as designed. To see the actual model
predictions you'd run the fine-tuning notebooks, but for testing the framework
the dataset labels are a perfectly good stand-in.

## 9. Common Errors and Fixes

- **"peer command failed" / "network isn't up"**
  The Fabric network isn't running. Run: `cd blockchain && ./scripts/deploy.sh`

- **"org X is not a registered stakeholder; call RegisterOrg first"**
  Run Step 6.5 (register the org) before submitting or voting.

- **"only N of M registered orgs have voted; X votes required for 2/3 quorum"**
  Not enough votes yet. Have the remaining orgs vote, then finalise again.

- **"org X already voted"**
  One vote per org per report is enforced. Use a fresh `report_id`.

- **"report ... already exists"**
  `report_id`s must be unique. Use a new `report_id` (add a counter/timestamp).

- **"401 unknown API key"**
  The key isn't in SQLite. Re-run Step 6.3 (`bootstrap-keys.sh`; the
  `offchain.db` must be the same file the server opens).

- **"ledger [mychannel] already exists with state [ACTIVE]" when redeploying**
  Old Docker volumes survived the teardown. Fix:
  ```bash
  cd blockchain && ./scripts/deploy.sh down
  docker volume rm compose_orderer.example.com \
    compose_peer0.org1.example.com compose_peer0.org2.example.com \
    compose_peer0.org3.example.com
  ./scripts/deploy.sh
  ```

- **Invoke fails with an endorsement error (org3)**
  The 2-of-3 policy needs >= 2 peers. Pass at least two endorsers, e.g.
  `FabricBridge(org="org1", endorsers=["org1","org3"])`.

- **onboard-org3.sh fails on missing org3 keys**
  A network redeploy wiped org3's crypto. It now regenerates it automatically
  (runs `addOrg3/addOrg3.sh up`); just re-run `./scripts/onboard-org3.sh`.

## 10. Clean Up / Reset

```bash
# stop the network (containers + chaincode)
cd blockchain && ./scripts/deploy.sh down
# stop the IPFS node (optional)
cd blockchain && ./scripts/start-ipfs.sh down
# reset the API's SQLite database (deletes stored reports + API keys)
rm -f apps/ai_service/v1-0-0/app/api/offchain.db
```

Re-running `deploy.sh` + Step 6.3 gives you a completely fresh system.
(The ~1.8 GB raw dataset is NEVER deleted by any of this - leave it alone.)

## 11. Key Points to Remember for Your Dissertation/Viva

- Raw text never touches the ledger - only a SHA-256 hash (proven by `/verify`).
- Reports are PENDING -> (>= 2/3 of registered orgs vote) -> FINAL/REJECTED,
  or EXPIRED after 72h if nobody decides.
- Once FINAL/REJECTED/EXPIRED the record is immutable (Fabric enforces this).
- `submitted_by` and every vote's voter MSP + txid come from real transaction
  identities, so provenance is automatic.
- Quorum = ceil(2 x N / 3): 2 orgs -> 2 votes, 3 orgs -> 2 votes.
