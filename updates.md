# Update v1

# Project Status — Blockchain-Enabled Framework for Misinformation Monitoring

**Purpose:** A blockchain-enabled framework for misinformation monitoring in
Sepedi/isiZulu/English — multilingual transformer models (AI/NLP layer) with a
permissioned Hyperledger Fabric audit layer that lets multiple fact-checking
organisations share reports and vote on their legitimacy (consortium review).
Group project (Honours, University of Pretoria).

---

## 1. Repository structure (current)

```
A_Blockchain_Enabled_Framework_for_Misinformation_Monitoring/
├── .gitignore
├── updates                         # this status tracker
├── rs_prop.pdf                     # proposal (implementation notes are outdated)
├── apps/ai_service/v1-0-0/
│   ├── app/src/
│   │   ├── blockchain.py           # C1: Python -> chaincode bridge (peer CLI)
│   │   ├── data_parser.py          # parses GPT-4o-mini translations
│   │   ├── data_prep.py            # dataset prep (dup resolution upstream)
│   │   ├── preprocessing.py        # anonymize_text() etc.
│   │   └── colab_pipeline.py       # Colab/Kaggle runner
│   └── data/raw/                   # ~1.8G dataset (gitignored) — DO NOT delete
└── blockchain/
    ├── explanation                 # pipeline explainer + step-by-step + troubleshooting
    ├── scripts/
    │   ├── deploy.sh               # 2-org network up/down + chaincode deploy
    │   ├── onboard-org3.sh         # add 3rd org + 2/3 endorsement policy
    │   └── smoke-test.sh           # CLI: register -> submit -> vote -> finalize -> query
    └── chaincode/misinformation/
        ├── DATA_MODEL.md           # B1 decision record (fields, keys, quorum)
        ├── README.md
        ├── DEPLOYMENT.md           # C2 Colab/Kaggle packaging notes
        └── go/                     # misinformation.go + unit tests (mock stub)
```

Deleted from the old balance-transfer/fabcar tutorial (cleanup): `api-1.4/`,
`api-2.0/`, `app/`, `app.js`, `artifacts/`, `explorer/`, old lifecycle scripts,
transcript `.tx` files, `log.txt`, `fabcar.tar.gz`, stale channel-artifacts.
The MSP/TLS crypto material is generated at runtime by `network.sh` into
`~/fabric-samples/test-network/organizations/` and is never committed.

---

## 2. AI/NLP layer (status)

- GPT-4o-mini translation of the ~102k-row dataset (English→Sepedi, forward+back)
  — **done**, parsed via `data_parser.py`.
- English baseline fine-tune of AfroXLMR-large — **done but not reportable**
  (test set leaked into eval during training). Fix identified: separate
  `training.ipynb` (train/val only) + `final_evaluation.ipynb` (test set, single
  `trainer.evaluate()` call).
- `anonymize_text()` built/tested (mentions, permalinks, photo credits) — O(n²)
  bug fixed. Outstanding: mojibake cleanup, URL pattern gaps, confirm anonymization
  ran on Sepedi translated columns.
- Outstanding: Sepedi fine-tune re-run, isiZulu pipeline, WER stratified
  comparison, final clean eval runs.

**Key learnings:** test-set integrity is non-negotiable; JSON over numbered lists
for batch LLM output; pandas rename atomicity; vectorize instead of looping.

---

## 3. Blockchain layer (Track B + C — functionally complete)

**Architecture:** Hyperledger Fabric v2.5.16, Raft consensus, LevelDB,
`fabric-samples/test-network`. Chaincode `misinformation` v1.1, `contractapi`
style (Go, not `shim`). REST layer deliberately deferred (B5) — CLI + Python
bridge only.

**Data model (`DATA_MODEL.md`):** on-chain fields `row_id, language,
content_hash (sha256, never raw text), proposed_label ("0"/"1"), confidence
[0,1], model_version, timestamp (RFC3339 UTC), submitted_by (MSP), status,
votes[], finalized_by, finalized_at`. Composite keys: `pred:{language}:{row_id}`
(reports), `org:{mspid}` (stakeholder orgs), `vote:{language}:{row_id}:{mspid}`
(one vote per org per report).

**Consortium review workflow:**
`RegisterOrg` → `SubmitReport` (PENDING) → `CastVote` (accept/reject, one per
org) → `FinalizeReport` once **≥ 2/3 of registered orgs** have voted → `FINAL`
if quorum accepted else `REJECTED`. After finalisation the record is immutable
and votes are closed. Quorum = ceil(2×N/3): 2 orgs→2, 3 orgs→2, 4 orgs→3.

**Functions:** `RegisterOrg`, `ListRegisteredOrgs`, `SubmitReport`, `CastVote`,
`FinalizeReport`, `QueryReport`, `QueryAllReports`, `GetReportCount`,
`QueryVotes`, `QueryReportHistory`, `ComputeContentHash`.

**Track status:** B1–B6 DONE (data model, contractapi style, functions,
topology, REST deferred, lifecycle tested live). C1–C3 DONE (Python bridge,
Colab/Kaggle packaging, write-up alignment). Org3 robustness test DONE
(`onboard-org3.sh`, 2-of-3 endorsement verified: 2 endorsers OK, 1 endorser
rejected; 3-org vote→finalize verified live).

---

## 4. How to deploy

Prereqs: Docker running, Go 1.22+, Fabric binaries on `~/fabric-samples/bin`
PATH, `jq` on PATH (or `~/.local/bin/jq`), Python 3.10+ for the bridge.

### 4.1 Two-org baseline (AND endorsement)

```bash
cd blockchain
./scripts/deploy.sh          # up + createChannel + deployCC (v1.1)
./scripts/smoke-test.sh      # CLI: register -> submit -> vote -> finalize -> query
```

`deploy.sh down` tears everything down. If `peer channel join` fails with
"ledger [mychannel] already exists" on redeploy → stale Docker volumes (see §5).

### 4.2 Add a 3rd stakeholder org (2/3 endorsement) — robustness test

```bash
# 1. network must be up (4.1). Start the org3 peer and join it to the channel:
~/fabric-samples/test-network/addOrg3/addOrg3.sh up

# 2. install + approve chaincode on all 3 orgs, commit with 2-of-3 policy:
cd blockchain
./scripts/onboard-org3.sh

# 3. verify: register org3 on-chain, then run a 3-org vote via the bridge:
cd apps/ai_service/v1-0-0/app/src
python3 - <<'EOF'
from blockchain import FabricBridge
b1 = FabricBridge(org="org1", endorsers=["org1","org3"])
b2 = FabricBridge(org="org2", endorsers=["org1","org2"])
b3 = FabricBridge(org="org3", endorsers=["org2","org3"])
b3.register_org()
b1.submit_report("demo-1","nso","f"*64,"1",0.95,"v1.0","2025-01-01T00:00:00Z")
b1.cast_vote("nso","demo-1","accept")
b2.cast_vote("nso","demo-1","accept")
b3.finalize_report("nso","demo-1")
print(b1.query_report("nso","demo-1"))
EOF
```

After 4.2 the endorsement policy is `OutOf(2, Org1MSP, Org2MSP, Org3MSP)` —
an invoke endorsed by only 1 org is rejected.

### 4.3 Python bridge quick reference

`apps/ai_service/v1-0-0/app/src/blockchain.py`:
`FabricBridge(org="org1"|"org2"|"org3", endorsers=["org1","org2"])` +
`register_org`, `list_orgs`, `submit_report`, `cast_vote`, `finalize_report`,
`query_report`, `query_all`, `count`, `votes`, `history`,
`submit_pipeline_output(rows, language, model_version, bridge)`,
`Prediction.hash_content(text)`.

### 4.4 Unit tests (no network needed)

```bash
cd blockchain/chaincode/misinformation/go
go test ./...      # registration, submit/query, validation, vote->finalize, quorum, rejection, count/all
```

---

## 5. Troubleshooting

- **`peer channel join` fails: "ledger [mychannel] already exists with state
  [ACTIVE]"** on redeploy → old Docker volumes survived `down`.
  ```bash
  ./scripts/deploy.sh down
  docker volume rm compose_orderer.example.com \
    compose_peer0.org1.example.com compose_peer0.org2.example.com
  ./scripts/deploy.sh
  ```
  (or `docker volume prune` for all unused volumes).
- **"org X is not a registered stakeholder; call RegisterOrg first"** → the org
  hasn't enrolled on-chain yet; run `RegisterOrg` (bridge `register_org()` or
  CLI) before submitting/voting.
- **"only N of M registered orgs have voted; X votes required for 2/3 quorum"**
  → not enough votes yet; have remaining orgs `CastVote`, then finalize again.
- **"verdict must be accept or reject"** → vote value typo.
- **"org X already voted"** → one vote per org per report is enforced.
- **Invoke fails after `onboard-org3.sh` with endorsement error** → the 2/3
  policy needs ≥ 2 of the 3 peers; your bridge must list ≥ 2 endorsers
  (`endorsers=["org1","org3"]`, etc.), not a single peer.
- **"peer command failed"** → network isn't up; re-run §4.1.
- **`jq: command not found`** → install jq (e.g. `jq-linux-amd64` →
  `~/.local/bin/jq`); `deploy.sh` picks it up automatically.
- **`addOrg3.sh up` fails** → the network must be up first (run §4.1), and
  `jq` must be on PATH.
- **`docker volume rm` permission errors** → stop the network first
  (`./scripts/deploy.sh down`).

---

## 6. Next actionable steps

1. AI/NLP: Sepedi fine-tune re-run (leak-free split), then isiZulu pipeline,
   WER stratified comparison, final clean eval runs.
2. Demo: wire `submit_pipeline_output` into the eval notebook to anchor real
   predictions, then show the vote→finalize flow with all registered orgs.
3. Optional for dissertation depth: richer chaincode queries (by label /
   language), or a 4th org to demonstrate quorum scaling (2/3 rule is dynamic).

---

# Update v2

# System Architecture & Implementation Guide
## Report Lifecycle, Verified Permissioning, Off-Chain Storage, API Gateway, Public Transparency, and Database Choice

This document answers your six points with concrete, buildable steps. It extends —
doesn't replace — the phased `PROJECT_ROADMAP.md`; a cross-reference table at the end
maps every section here onto specific roadmap phases so the two documents stay in sync.

---

## 0. TL;DR — Decisions at a Glance

| Question | Decision | Why (short version) |
|---|---|---|
| CouchDB or LevelDB? | **CouchDB** | Rich JSON queries (status/language/confidence/time filters) without hand-building composite-key indexes for every new filter you'll want for the API and public explorer |
| Where does the full report live? | **Off-chain — IPFS by default** (`ipfs://<CID>`), SQLite fallback; never on-chain | Blockchain-native URI with a graceful no-infra fallback; the ledger only ever stores the hash + URI |
| How do orgs connect? | **REST API gateway wrapping the existing `FabricBridge`**, orgs authenticate with an API key issued at sign-up, never touch `peer` CLI directly | Matches your stated goal directly; keeps your already-working CLI-shell-out transport instead of a risky mid-project SDK rewrite |
| How is "verified stakeholder" actually enforced? | **Two tiers**: Fabric channel membership (hard, requires real admission process) + an on-chain org-admission **vote** (reuses your existing 2/3 quorum machinery) | Currently `RegisterOrg` is self-service with zero vetting — this closes that gap using infrastructure you've already built |
| How is public transparency reconciled with permissioning? | **Read-only query endpoints** behind the API gateway (status/language filters) — the read-only Fabric identity path; Hyperledger Explorer was evaluated then removed (`explorer/` deleted in a later cleanup) | Writes stay permissioned (chaincode-enforced); reads are served by the gateway without giving the public a Fabric identity of their own |
| How do you test with real data? | **Your own held-out test set first, then genuinely new out-of-sample scraped data** | You already have real predictions sitting in `held_out_test_set.csv` — use them before reaching for anything external |

---

## 1. AI-Generated Reports — Schema & Steps

### 1.1 The core problem with the current setup
Right now your pipeline produces aggregate metrics (`sepedi_final_test_results.json`) but
not a **per-row Report object** with a defined schema. `Prediction` in `blockchain.py` is
close but minimal (no source data, no schema versioning, no distinction between
AI-submitted and human/manually-submitted reports).

### 1.2 Report schema (off-chain, full object)

```json
{
  "report_id": "nso-42-v1",
  "schema_version": "1.0",
  "language": "nso",
  "source": {
    "type": "tweet",
    "platform": "twitter",
    "original_url": null,
    "raw_text": "the actual tweet/article text",
    "published_at": "2025-11-02T14:00:00Z"
  },
  "verdict": {
    "label": "1",
    "confidence": 0.94,
    "submission_type": "ai_model",
    "model_version": "afroxlmr-large-nso-v1.0",
    "inference_timestamp": "2026-01-10T09:00:00Z"
  },
  "submitter": {
    "org_mspid": "Org1MSP",
    "submitted_at": "2026-01-10T09:00:05Z"
  },
  "content_hash": "sha256 of this JSON, canonically serialized (see 3.2)"
}
```

Notes:
- `submission_type` is `"ai_model"` or `"manual"` — per your framing ("conflate
  stakeholder and AI model as the same submitter from this point on"), both go through
  the identical submission path; this field just preserves *how* the verdict was
  produced, which is valuable provenance for your dissertation's evaluation chapter even
  if the workflow treats them identically downstream.
- `raw_text` lives **only** in this off-chain object, never on-chain — consistent with
  your existing `DATA_MODEL.md` design principle (§1.a: "never raw text").
- `content_hash` is computed over the **whole object** (minus the hash field itself), not
  just `raw_text` — this means tampering with *any* field (confidence, model version,
  submitter) is detectable, not just tampering with the source text. This is a
  meaningful strengthening over the current implementation, which only hashes raw text.

### 1.3 Steps
1. Add a small `report.py` module (or extend `blockchain.py`'s `Prediction` class) that
   builds this JSON structure from either (a) a row of `final_evaluation.ipynb` output,
   or (b) a manually-entered submission via the API.
2. **Change `final_evaluation.ipynb` to export per-row predictions**, not just aggregate
   metrics — currently it only saves `sepedi_final_test_results.json` (the aggregate
   dict). Add a cell that saves `trainer.predict(tokenized_test)` output (per-row logits
   → predicted label + confidence) alongside `test_df`, so you have real
   `{row_id, text, predicted_label, confidence}` rows to build reports from — this is
   the direct input to Phase 8 (full integration) in the roadmap and to the real-data
   testing in §8 below.
3. Write `canonicalize_and_hash(report_dict)`: `json.dumps(report_dict, sort_keys=True,
   separators=(",", ":"))` then SHA-256. **Use this exact function on both the client
   side (when submitting) and anywhere you later re-verify integrity** — inconsistent
   JSON serialization (key order, whitespace) will silently produce different hashes for
   identical content, which would look like a bug or tampering when it's actually just a
   serialization mismatch. This is worth a unit test on its own.

---

## 2. Verified, Permissioned Stakeholders — Two-Tier Design & Steps

### 2.1 What "permissioned" currently means in your system (and the gap)
Two separate layers exist in Fabric, and right now only one is meaningfully enforced:

- **Tier 1 — Channel membership (hard permissioning).** Only orgs whose MSP is part of
  the channel configuration can even connect a peer or submit a signed transaction at
  all. This is already enforced — adding org3 required `addOrg3.sh` and a channel
  config update, not a simple API call. This is real permissioning.
- **Tier 2 — Application-level "active stakeholder" status.** Your chaincode's
  `RegisterOrg` function. **Currently this is fully self-service**: any identity from
  any Tier-1 channel member can call `RegisterOrg` and immediately become a voting
  stakeholder — there's no vetting, no approval step, nothing verifying the org is a
  legitimate fact-checking organization rather than, say, a second identity controlled
  by the same actor.

So today, "permissioned" = "has a Fabric identity from a channel member org," not
"has been verified as a legitimate stakeholder." Given you explicitly said "verified
stakeholder organisation," this gap is worth closing.

### 2.2 Design: reuse your existing voting infrastructure for org admission
Instead of building a whole separate vetting system, extend the exact pattern you
already use for report finalization to org admission itself:

**New chaincode structures (add to `misinformation.go`):**
```go
type OrgAdmissionRequest struct {
    CandidateMSP string `json:"candidate_msp"`
    OrgName      string `json:"org_name"`
    OrgType      string `json:"org_type"` // "fact-checker" | "media-monitor" | "ngo"
    RequestedAt  string `json:"requested_at"`
    Votes        []Vote `json:"votes"`
    Status       string `json:"status"` // PENDING | ADMITTED | REJECTED
}
```

**New functions:**
- `RequestOrgAdmission(ctx, orgName, orgType)` — called by the candidate org's own
  identity (they must already be a Tier-1 channel member — this is *requesting*
  Tier-2 stakeholder status, not bypassing Tier-1). Creates a PENDING admission request
  keyed `admission:{candidateMSP}`.
- `VoteOnOrgAdmission(ctx, candidateMSPID, verdict)` — only **existing registered**
  orgs may vote (same `isRegisteredOrg` check pattern as `CastVote`).
- `FinalizeOrgAdmission(ctx, candidateMSPID)` — once ≥ 2/3 of *currently registered*
  orgs have voted, either writes the `org:{mspid}` key (admitting them) or marks
  REJECTED. Mirrors `FinalizeReport` almost exactly — you can largely copy that
  function's structure.

### 2.3 The bootstrapping problem (must decide explicitly)
A pure "must be voted in by existing members" rule can't work for the very first
members — there's no one to vote yet. Decide one of:
- [ ] **Genesis allowlist**: keep the current simple `RegisterOrg` self-service call, but
  restrict it to only work while the registered-org count is below a small founding
  threshold (e.g., ≤ 3) — a hardcoded bootstrap window. After that, all new orgs must go
  through the admission-vote workflow.
- [ ] **Manual genesis registration**: the network operator (you, for the PoC) manually
  registers the first 2–3 founding orgs via a privileged one-time script, then disables
  that path and requires voting for everyone after.

Either is fine for a dissertation PoC — just document the choice explicitly in
`DATA_MODEL.md`, since "how do the first stakeholders get in" is a natural question in
a viva about a "verified stakeholder" system.

### 2.4 Real-world vetting (outside the chaincode)
The on-chain vote only proves *existing orgs agreed* — it doesn't itself verify
real-world legitimacy. For the PoC, document what the vetting process *would* look like
in production (e.g., a candidate org submits registration documents / proof of
fact-checking accreditation through an off-chain process before even requesting
admission), and simulate it for your demo (e.g., a short "org application form" in your
API, reviewed manually before you, as network operator, allow the `RequestOrgAdmission`
call to go through). You don't need to build a document-verification system — you need
to show you've thought about where the real-world trust boundary sits.

**Steps:**
1. Implement `OrgAdmissionRequest` + the three new functions in `misinformation.go`,
   mirroring `SubmitReport`/`CastVote`/`FinalizeReport`.
2. Decide and implement the bootstrapping rule (§2.3).
3. Add unit tests: candidate can't vote on their own admission, un-admitted candidate
   can't call `SubmitReport`/`CastVote` on reports, admission requires the same 2/3
   quorum math as report finalization (reuse `quorumFor`).
4. Update `DATA_MODEL.md` with the new admission workflow and the bootstrapping decision.
5. Update `blockchain.py` / your future API (§4) with `request_admission`,
   `vote_on_admission`, `finalize_admission` methods.

---

## 3. Report → Off-Chain Storage → On-Chain Block → Vote → Finalize (Full Flow)

This is very close to what you already have — `SubmitReport` → `CastVote` →
`FinalizeReport` already implements exactly the block-append-then-vote pattern you
described. The gaps are: (a) no off-chain storage integration yet, (b) no
timeout/expiry mechanism for votes that never reach quorum.

### 3.1 The end-to-end flow (target state)

```
1. Org's client (or the API server on their behalf) builds the full Report object (§1.2)
2. Client computes content_hash = canonical_hash(report)          [client-side, §1.3]
3. Client POSTs the full report to the off-chain storage API  →  gets back off_chain_uri
4. Client calls chaincode SubmitReport(row_id, language, content_hash,
                                        label, confidence, model_version,
                                        timestamp, off_chain_uri)   ← NEW PARAM
5. Chaincode writes ReportRecord (PENDING) including off_chain_uri and a
   computed voting_deadline = timestamp + fixed window (e.g. 72h)  ← NEW FIELDS
6. Chaincode emits a "ReportSubmitted" event (real peer shim supports SetEvent;
   MockStub doesn't — this only matters for the live network, not unit tests)
7. Off-chain notification service (part of the API layer, §4) listens for the event
   and notifies all registered orgs (webhook / email / dashboard badge) — this is
   your "vote request sent to all registered orgs" step
8. Registered orgs fetch the full report via off_chain_uri, review it, call CastVote
9a. If ≥2/3 quorum reached before the deadline → any org calls FinalizeReport → FINAL/REJECTED
9b. If deadline passes with no quorum → ExpireReport (new function) → EXPIRED
     (triggered either by a scheduled off-chain job, or opportunistically inside
      FinalizeReport if called after the deadline — see 3.3)
10. Anyone with the off_chain_uri (permissioned org, or public via the explorer, §5)
    can fetch the full report and independently recompute + compare content_hash
    → proves the off-chain copy hasn't been tampered with
```

### 3.2 Chaincode changes needed

**`ReportRecord` struct — add two fields:**
```go
OffChainURI    string `json:"off_chain_uri"`
VotingDeadline string `json:"voting_deadline"` // RFC3339, set at submission time
```

**`SubmitReport` — add `offChainURI` parameter**, set `VotingDeadline` using
`ctx.GetStub().GetTxTimestamp()` (the deterministic, ordering-service-assigned
timestamp — **not** `time.Now()`, which would differ across peers and break
determinism) plus a fixed duration constant, e.g.:
```go
const votingWindow = 72 * time.Hour
txTime, _ := ctx.GetStub().GetTxTimestamp()
deadline := txTime.AsTime().Add(votingWindow).UTC().Format(time.RFC3339)
```

**New function `ExpireReport(ctx, language, rowID)`:**
- Reads the record; if `Status == PENDING` and current tx timestamp is past
  `VotingDeadline`, sets `Status = "EXPIRED"` and writes it back (same immutability
  rules apply afterward — no further votes).
- If called before the deadline or on a non-PENDING report, returns an error (so it
  can't be used to prematurely kill a report that's still legitimately in its window).

**`FinalizeReport` — small addition:** if called after `VotingDeadline` has passed and
quorum still isn't reached, auto-transition to `EXPIRED` instead of just erroring —
reduces total reliance on the scheduled job below.

### 3.3 Off-chain scheduler (chaincode can't run timers itself)
Fabric chaincode is purely reactive — it only runs in response to transactions, so
nothing calls `ExpireReport` on its own. Add a small scheduled job in your API server
(§4): every hour, `QueryAllReports`, filter `PENDING` + `VotingDeadline` passed, call
`ExpireReport` for each. `APScheduler` (Python) or a simple cron container both work
fine at this scale.

### 3.4 Off-chain storage — decision and steps (implemented)
The gateway stores full Report objects (raw text included) **off-chain**; the
ledger holds only the hash + `off_chain_uri`. Implemented with a **pluggable
backend** (`app/api/storage.py`):
- [x] **IPFS is the default backend.** Reports are added to a local `ipfs/kubo`
  node (Docker) and `off_chain_uri` becomes `ipfs://<CID>` — a blockchain-native
  transparency story. Start it with `blockchain/scripts/start-ipfs.sh`.
- [x] **SQLite fallback.** If the IPFS node is unreachable, the payload is kept
  in SQLite and the URI falls back to the API URL, so the pipeline stays
  testable without IPFS. `GET /api/status` reports the active backend.
- [x] `GET /api/reports/{report_id}` resolves from IPFS (or the local copy) and
  returns the full JSON — this endpoint can be public or permissioned depending
  on how you want to handle §5's transparency goal (see §5.3).
- [x] **API keys** (`org_keys`) always live in SQLite; bootstrap with
  `blockchain/scripts/bootstrap-keys.sh`.

### 3.5 Steps summary
1. Extend `ReportRecord` + `SubmitReport` signature (Go) — `off_chain_uri`,
   `voting_deadline`.
2. Add `ExpireReport` chaincode function + unit tests (deadline-passed → EXPIRED;
   deadline-not-passed → rejected call).
3. Update `FinalizeReport` for the auto-expire-on-late-call behaviour.
4. Update `blockchain.py`: `submit_report()` gains an `off_chain_uri` parameter;
   add `expire_report()`.
5. Stand up the off-chain storage table + `POST /api/reports` / `GET /api/reports/{id}`
   endpoints (this is really part of §4's API server, built together).
6. Build `verify_report_integrity(report_id)` — fetch full report, recompute hash,
   compare to on-chain `content_hash`. **Demo this explicitly** — it's the concrete proof
   of your tamper-evidence claim and makes a strong dissertation/viva moment.
7. Build the scheduled expiry job.
8. Build the event-listener/notification piece (can be as simple as the API polling
   `QueryAllReports` for new PENDING entries every few minutes if you don't want to deal
   with Fabric chaincode events directly — less elegant, considerably less setup risk;
   real chaincode events via the peer's event service are the "proper" version if time
   allows).

---

## 4. API Gateway Layer — Architecture & Steps

### 4.1 Architecture
```
   Fact-checking org's browser/client
              |  HTTPS + API key
              v
   ┌─────────────────────────────┐
   │   API Gateway (FastAPI)      │   ← single server, orgs never touch peer CLI
   │  - org auth (API key → MSP)  │
   │  - off-chain report storage  │
   │  - wraps FabricBridge        │
   │  - scheduled expiry job      │
   └───────────┬───────────────────┘
               │  shells out to `peer` CLI per org's crypto material
               v
   Fabric network (orderer + peers, your existing test-network)
```

Keep using `FabricBridge`'s CLI shell-out — it's already built, tested, and you've
independently verified hash equality on the live network. Don't rewrite the transport
layer mid-project unless Phase 9B's latency numbers show subprocess spawn overhead is a
real bottleneck; if it does become one later, migrating specific hot paths to the
Fabric Gateway SDK (Node.js/Go — no official Python client from Hyperledger) is a
scoped follow-up, not a prerequisite.

### 4.2 Identity handling (the part that needs a real decision)
The API server needs to sign transactions **as the calling org**, not as itself. Two
options:

- **Option A (recommended for PoC): server holds each org's crypto material.**
  At sign-up, you provision each org's MSP/TLS material into the server's filesystem
  (one directory per org, exactly like `FabricBridge`'s `_build_env()` already expects),
  and map their issued API key to an `org=` parameter passed into `FabricBridge`. Simple,
  works with your existing code, but means the server is a trusted custodian of every
  org's keys — acceptable for a PoC/demo, worth flagging as a production limitation in
  your dissertation (a real deployment would want each org holding its own keys and
  connecting via their own client, with the server only relaying already-signed
  transactions — a materially bigger build).
- **Option B (more correct, more work): orgs hold their own keys locally**, run a thin
  local client (could literally be `FabricBridge` running on their machine, extending
  Phase 7's CLI tool), and the "API server" is really just the Fabric network itself
  plus your off-chain storage service — no shared signing custodian at all.

**Recommendation:** build Option A for the demo/dissertation (simpler, faster, and the
custodial-trust tradeoff is a legitimate, explainable design decision for a PoC), and
explicitly write up Option B as the production-hardening path in your limitations
section.

### 4.3 Core endpoints
```
POST   /api/orgs/apply              — submit org application (§2.4 off-chain vetting)
POST   /api/orgs/{msp}/admission    — RequestOrgAdmission (once approved to apply)
POST   /api/orgs/{msp}/admission/vote — VoteOnOrgAdmission
GET    /api/orgs                    — ListRegisteredOrgs

POST   /api/reports                 — full flow: store off-chain, hash, SubmitReport
GET    /api/reports/{report_id}     — fetch full off-chain report (§3.4)
GET    /api/reports/{report_id}/chain — QueryReport (on-chain record)
GET    /api/reports?status=PENDING&language=nso  — filtered list (CouchDB-backed)
POST   /api/reports/{report_id}/vote — CastVote
POST   /api/reports/{report_id}/finalize — FinalizeReport
GET    /api/reports/{report_id}/verify — recompute + compare hash (§3.5 step 6)
GET    /api/reports/{report_id}/history — QueryReportHistory
```

### 4.4 Steps
1. Scaffold FastAPI project (`apps/ai_service/v1-0-0/app/api/`), with an API-key auth
   dependency mapping keys → org identity.
2. Implement the report endpoints, wiring into `FabricBridge` + the off-chain storage
   table.
3. Implement the org admission endpoints wiring into the new chaincode functions
   (§2.2).
4. Add the scheduled expiry job (`APScheduler`, run in-process or as a separate
   container).
5. Add basic request logging — you'll want this raw data for Phase 9B's latency
   measurements anyway, so instrument timing here from the start rather than bolting it
   on later.
6. This *is* Phase 7 of the roadmap, upgraded from "CLI tool" to "full API" — update
   `PROJECT_ROADMAP.md` Phase 7 to point at this section once you start building it.

---

## 5. Public Transparency via Blockchain Explorer — Steps

### 5.1 The reconciliation
Permissioning controls **who can write** (submit, vote, finalize) — it doesn't have to
control **who can read**. Fabric already supports this cleanly: give one identity
**read-only** access (an MSP identity that's a channel member but is never registered as
a stakeholder org, so it can query but never gets past `isRegisteredOrg` checks even if
someone tried to misuse it for writes — though for true safety, front it with an explorer
tool that only ever issues query calls, never invokes).

### 5.2 Evaluated but removed: Hyperledger Explorer
[Hyperledger Explorer](https://github.com/hyperledger/blockchain-explorer) was
briefly considered and the configuration was committed, then **removed** (see
the cleanup commit deleting `blockchain/explorer/`). It was heavier than the
remaining timeline justified. The design principle survives:
- Writes stay permissioned (chaincode-enforced); reads become public without
  giving the public a Fabric identity of their own.
- On-chain data never includes raw text (only hashes + `off_chain_uri`), so
  exposing ledger contents needs no redaction by design.

### 5.3 Implemented instead: read-only gateway endpoints
The gateway serves the transparency story today:
- [x] Reuse the read-only query endpoints from the API (§4.3's `GET` routes),
  backed by the same read-only Fabric identity path.
- [x] `GET /api/reports` supports `?status=` / `?language=` filtering (CouchDB
  rich queries or in-memory filtering per §4.3).
- [x] `GET /api/reports/{id}/verify` proves the off-chain copy matches the
  on-chain hash — the concrete tamper-evidence demo.
- [ ] Optional stretch: a thin static dashboard page listing FINAL/REJECTED
  reports and their verdicts. Not required for the core deliverable; revisit if
  time allows.

---

## 6. CouchDB vs LevelDB — Full Comparison

| | LevelDB (current default) | CouchDB |
|---|---|---|
| Query model | Exact key lookup + composite-key prefix range only | Full JSON/Mango rich queries (any field, ranges, `$or`/`$and`) |
| Fits your current chaincode | Yes — `GetStateByPartialCompositeKey` is all LevelDB-friendly | Yes — same composite-key calls still work, you *gain* rich query on top |
| Fits your planned features | Query-by-language/status (Phase 6) needs new composite keys per filter you want, by hand | Query-by-anything works without new indexes |
| Fits public explorer/API filtering | Limited — you'd hand-build an index for every filter combination the public dashboard wants | Natural fit — dashboard filters map directly to Mango queries |
| Operational cost | Simpler — embedded, no extra container | One more container to deploy/monitor, slightly higher latency per read |
| Data format requirement | Any bytes | Must be valid JSON (already true for your `ReportRecord`/`Vote`/`RegisteredOrg` — no change needed) |
| Fabric test-network support | Default | Built-in flag: `./network.sh up -s couchdb` |

**Recommendation: CouchDB.** Given you want (a) public filtering in the explorer/API,
(b) status/language/time-range queries you already scoped in Phase 6, and (c) the
migration cost is effectively one flag change plus redeploying (your chaincode's
composite-key calls keep working unmodified, you're purely adding capability, not
removing anything) — there's no real reason to stay on LevelDB here. Keep LevelDB in
mind as the right call only if you were optimizing for absolute minimal operational
footprint over query flexibility, which isn't your priority given the stated goals.

**Steps:**
1. `./network.sh down` (clean slate).
2. `./network.sh up -s couchdb -c mychannel` (or update `deploy.sh` to pass `-s couchdb`
   through — small script edit).
3. Redeploy chaincode as usual — no Go code changes required for basic operation.
4. Once running, you can hit CouchDB's own Fauxton UI (typically `localhost:5984/_utils`)
   directly against the state database for ad-hoc query testing during development,
   separate from your production API's Mango queries.
5. For richer chaincode-side queries (e.g., `QueryReportsByConfidenceRange`), use
   `ctx.GetStub().GetQueryResult(mangoQueryString)` — this is the CouchDB-only chaincode
   API that returns `"not implemented"` in your current `MockStub` (`GetQueryResult`),
   so **new unit tests for Mango-query functions will need either an extended MockStub
   or integration testing against a real CouchDB-backed network** — flag this as an
   extra testing consideration when you build these functions.

---

## 7. Testing With Real Data — Steps

### 7.1 Start with data you already have
1. Your `held_out_test_set.csv` (produced by `training.ipynb`, consumed by
   `final_evaluation.ipynb`) is genuine real data with genuine model predictions —
   use this **first**, before reaching for anything external.
2. Per §1.3 step 2, export per-row predictions from `final_evaluation.ipynb` (not just
   aggregate metrics).
3. Write `submit_test_batch.py`: reads N rows, builds full Report objects (§1.2) using
   the real tweet text + real model verdict, submits through the API (§4), and has 2–3
   test orgs (your Phase 9B VMs are perfect for this — reuse that setup) vote.
4. **Interesting side-analysis for your dissertation**: compare the on-chain FINAL
   verdict (after consortium voting) against the known ground-truth label from the
   dataset. Does org voting improve accuracy over the raw model prediction, or just add
   latency? This is a genuinely useful research question your system is well-positioned
   to answer, and it's free once the pipeline exists.

### 7.2 Fresh, out-of-sample real-world data
1. Once the held-out-set test passes, pull a small batch of **genuinely new** data not
   in your training or test sets at all — e.g., recent South African election-related
   tweets/articles, scraped or manually collected.
2. Run inference on these with your trained model, submit through the full pipeline.
3. This demonstrates the system operating in a live/production-like mode, not just
   replaying historical test data — worth having at least one clean example of this for
   the dissertation demo (Phase 8's integration artifact).

### 7.3 Edge cases worth deliberately including
- A genuine duplicate submission (same real row submitted twice) — confirms the
  "already exists" rejection works on real data, not just synthetic test fixtures.
- A borderline-confidence row (model confidence near 0.5) — see how org voters handle
  genuinely ambiguous cases; this is exactly the scenario your consortium-review design
  exists for, worth highlighting explicitly.
- A non-English-contaminated row if any exist in your Sepedi/isiZulu data (code-switched
  text) — worth knowing how your anonymization and model handle it.
- Combine with Phase 9B: run the multi-VM load test using **real report content**
  (actual tweet lengths/structure) rather than trivial placeholder strings, so your
  latency/throughput numbers are representative of real payload sizes.

---

## 8. Cross-Reference to `PROJECT_ROADMAP.md`

| This document's section | Roadmap phase it extends |
|---|---|
| §1 Report schema | Phase 3/5 (fine-tuning) output format, feeds Phase 8 (integration) |
| §2 Verified stakeholders | Phase 6 (blockchain depth) — add as Phase 6.4 |
| §3 Off-chain storage + expiry | Phase 6 (blockchain depth) — add as Phase 6.5; also touches Phase 8 |
| §4 API gateway | **Replaces/upgrades Phase 7** ("usability layer") from CLI-only to full API |
| §5 Public explorer | New addition to Phase 7 — add as Phase 7.3 |
| §6 CouchDB migration | New addition — insert as **Phase 6.0** (do this *before* Phase 6.1's filtered queries, since those queries are much easier to write against CouchDB) |
| §7 Real-data testing | Directly feeds Phase 8 (integration) and Phase 9B (performance testing) — use real data for both, not synthetic rows |

### Suggested phase re-ordering given this document
1. Phase 0 (hygiene) — unchanged
2. **Phase 6.0 (CouchDB migration)** — moved earlier, do before other blockchain work
3. Phase 1 (EDA) — unchanged, parallel track
4. **Phase 6.2 (org metadata) + Phase 6.4 (verified admission voting)** — do together,
   same struct changes
5. **Phase 6.5 (off-chain storage + report schema + expiry)** — the biggest single chunk
   of new work in this document
6. Phase 2–3 (data quality, Sepedi re-run) — unchanged, parallel track
7. **Phase 7, rebuilt as the full API gateway (§4) + explorer (§5)**
8. Phase 5 (isiZulu) — unchanged, parallel track
9. Phase 8 (integration) — now using the real report schema + API + off-chain storage,
   plus real data per §7
10. Phase 9A/9B (validation, performance) — now testable with real payload sizes and the
    real API layer, not the CLI shell-out directly
11. Phase 10–11 (dissertation alignment, submission) — unchanged

---
---

# Update v3

# Performance & Stress-Testing Upgrade — Full Plan and System Design

**Goal of v3:** turn the functionally-complete v2 system into a **measurable,
reproducible, variable-driven performance-testing platform** for the dissertation's
Phase 9A/9B (validation + performance). v3 adds the knobs, harness, and methodology
to answer "how does this blockchain framework scale under realistic load?" with
defensible numbers.

**Scope decision (agreed):** CORE variables only. The full 30+ variable universe was
reviewed and trimmed to ~13 high-signal knobs. Everything else (320 channels,
10–11 orderers, 200s batch timeout, 40 MB on-chain payloads, 2M write-set entries,
1M pre-seeded assets, 3000 TPS targets) is explicitly out of scope for v3 — low
signal for this system, or physically unrepresentable on the single-host test-network.

---

## 0. TL;DR — v3 Decisions at a Glance

| Question | Decision |
|---|---|
| What is v3 for? | Reproducible performance/stress experiments feeding dissertation Phase 9A/9B |
| How many variables? | **13 core** (see §1). All numeric levels are **dynamic** — pass any value up to a hard **max**; there are no preset level lists |
| Founding org limit | **Dynamic CLI arg** `--orgs N` (total limit; pass 6 for 6 orgs). Chaincode `SetFoundingOrgLimit` already exists |
| Encryption-style gate | **3 selectable styles** (ECDSA-P256, Ed25519, RSA) — *simulated* at the onboarding gate for load realism, NOT a real MSP/algorithm swap |
| Load driver | **Caliper (Fabric Gateway/SDK, gRPC)** for high-rate + **HTTP path (FastAPI gateway)** for the real user-facing path — both |
| Predictive Python sim | **No.** Build `startup.sh` (arg-driven start switch → deploy → load → log), not a predictive throughput model |
| Start switch | **`startup.sh`** — invoke it with variable args; the whole sim runs from those args |
| fabric-samples | Vendored into `blockchain/fabric-samples/` (bin/, config/, test-network only); generated dirs gitignored |
| Explorer | Planned (public read-only auditor UI, non-participating identity); deferred out of v3 core, tracked as v3 stretch |
| Single-host reality | TPS expectations tempered: single host + CLI bridge caps low; Caliper/LevelDB/tuned batching is the only route near the aspirational numbers |

---

## 1. Core Variables — dynamic levels with hard maxes

Every variable is **dynamic**: you pass a value at invocation (`startup.sh` args, §2.3),
not a choice from a preset list. For numeric variables the only bound is a hard **max**
(a safety ceiling — the single-host test-network simply cannot meaningfully go higher);
for enumerated variables the valid values are listed. Each variable has a stable name,
its domain, a default (used when the arg is omitted), and the location where the knob
physically lives.

| # | Variable | Type / domain | Max | Default | Knob lives in |
|---|---|---|---|---|---|
| 1 | `orgs` | integer ≥ 1 | 20 | 3 | `startup.sh --orgs N` → `SetFoundingOrgLimit` + org self-registration |
| 2 | `peers_per_org` | integer ≥ 1 | 3 | 1 | `network.sh` compose (peer container count) |
| 3 | `orderers` | integer ≥ 1 (Raft FT needs ≥ 3) | 5 | 3 | `network.sh up` / compose (Raft cluster) |
| 4 | `server_spec` | `baseline` \| `big` (host tag) | — | baseline | host selection; recorded per run |
| 5 | `batch_timeout` | duration ≥ 100 ms | 200 s | 2s | channel config (`configtx.yaml`) |
| 6 | `max_message_count` | integer ≥ 1 | 1600 | 10 | channel config (`Orderer.BatchSize`) |
| 7 | `state_db` | `leveldb` \| `couchdb` | — | couchdb (current) | `deploy.sh -s couchdb` vs none |
| 8 | `concurrency` | integer ≥ 1 | 200 | 24 | load driver (Caliper / HTTP) |
| 9 | `arrival_rate` | TPS ≥ 1 (measured, not guaranteed) | 400 (stretch 3000) | 25 | load driver `txnRate`; HTTP async submit |
| 10 | `rw_mix` | % writes, 0–100 | 100 | 100 | load workload (`%read`/`%write`) |
| 11 | `endorsement_policy` | `2of3` \| `3of3` | — | 2-of-3 | `deploy.sh`/`onboard-org3.sh` `POLICY` |
| 12 | `fault_inject` | `none` \| `kill-peer` \| `kill-orderer` | — | none | harness orchestration (docker stop mid-run) |
| 13 | `crypto_style` | `ecdsa` \| `ed25519` \| `rsa` | — | ecdsa | `CRYPTO_STYLE` env (§2.2) |

**Example (dynamic, not preset):** `startup.sh --orgs 7 --concurrency 150 --rate 300`
is a fully valid run even though 7 / 150 / 300 appear nowhere in a level list. Only the
maxes (20 / 200 / 400) are fixed caps. Values above a max are rejected with a clear
error; values in range are accepted as-is.

**Secondary set (not in core; add only if time):** endorsement per-invoke count (2 vs
3, largely subsumed by #11), ledger pre-seeding (empty vs 100K — 1M dropped),
channel count (1 vs 5 — 320 dropped), payload length (off-chain layer only, see §6).

**Deliberately out of scope:** 320 channels, 10–11 orderers, 200s batch timeout,
40 MB on-chain payload, 2M write-set entries, 1M pre-seeded assets, 3000 TPS as a
target, SM2/post-quantum real algorithm swaps.

---

## 2. System Design Changes Required (v3)

### 2.1 Dynamic founding org limit (DONE at chaincode level)
- `misinformation.go`: `foundingOrgLimit()` reads on-ledger config (composite key
  `cfg:foundingOrgLimit`), defaults to 3. `SetFoundingOrgLimit(ctx, limit)` writes it
  (validates ≥ 1). `RegisterOrg` gates on the *dynamic* value. ✔ committed in working tree.
- **Remaining:** `deploy.sh --orgs N` flag → after `deployCC`, invoke
  `SetFoundingOrgLimit N` (org1 peer, `peer chaincode invoke`); then `register-orgs.sh`
  self-registers orgs 1..N via `RegisterOrg`. `--orgs 2` ⇒ limit 2, `--orgs 6` ⇒ limit 6.
- **Semantics (agreed):** total, not additive. `--orgs 6` = 6 orgs on-chain.
- **Test note:** `misinformation_test.go` genesis test ("4th org rejected at 3") must be
  updated to exercise `SetFoundingOrgLimit` (set 5, register 4th, expect success).

### 2.2 Encryption-style gate (simulated, app-layer)
- **What it is:** the onboarding/token gate (server-side, `storage.py`/gate module)
  runs a per-request "decrypt" step whose CPU cost is chosen by `CRYPTO_STYLE`.
  It is a **load-shaping shim**, not a real Fabric algorithm change — Fabric MSP already
  does real X.509 verify on every tx; this adds the *app-layer* per-request cost a real
  asymmetric decrypt would impose, for more realistic server load at high concurrency.
- **Styles (3, agreed subset):**
  - `ecdsa` — ECDSA P-256 verify/decrypt stand-in (Fabric's default curve).
  - `ed25519` — Ed25519 sign/verify stand-in (fastest of the three).
  - `rsa` — RSA-2048 decrypt stand-in (heaviest CPU; models legacy-style identity
    token checks).
- **Mechanism:** throwaway bootstrap keypair (`private_key_onboarding_all` /
  `public_key_onboarding_all`, seeded by `bootstrap-keys.sh`). Org x sends a random
  token of key length; server "decrypts" via the selected style's PyCrypto primitives,
  compares, always accepts (pass/fail is fixed; the cost is the point).
- **Where:** new `CRYPTO_STYLE` env var, default `ecdsa`. Exposed via `/api/status`.
- **Excluded (agreed):** AES-GCM (fits off-chain encrypted-report variant, not the
  gate), SM2 (tooling friction), post-quantum (not runnable on Fabric today).

### 2.3 `startup.sh` — the start switch (the v3 harness)

- **What it is:** **`startup.sh` is the single entry point of the whole simulation.**
  You invoke it with variable args; it brings the network up, deploys, runs the load,
  records results, and tears everything down — all driven by the args you passed.
  There is no separate scenario file to hand-edit; the args *are* the scenario.
- **Invocation form (dynamic, not preset):**
  ```bash
  ./startup.sh --orgs 6 --peers-per-org 2 --orderers 3 --state-db couchdb \
    --batch-timeout 5s --max-message 500 --policy 2of3 \
    --concurrency 150 --rate 300 --rw-mix 50 --crypto-style rsa \
    --fault none --server baseline
  ```
  Any numeric arg accepts any value within its domain (§1). Omitted args fall back to
  the defaults. `--help` prints the full variable table with domains/maxes.
- **Flow (in order):**
  1. Parse args → merge with defaults → validate every value against its §1 max
     (out-of-range → clear error, no silent clamp).
  2. Provision network: `network.sh up` (state_db, orderers, peers), create channel.
  3. Deploy: `deployCC` at the requested endorsement policy; invoke
     `SetFoundingOrgLimit N`; `register-orgs.sh` self-registers orgs 1..N.
  4. Load phase: Caliper (gRPC) or HTTP (FastAPI) per `--driver`, at the requested
     concurrency / rate / rw-mix / crypto-style.
  5. Fault phase (only if `--fault`): orchestrate a mid-run `docker stop` of a peer or
     orderer leader, then measure recovery / view-change latency.
  6. Record → append a row to `benchmarks/results.csv` (args snapshot, timestamp,
     actual TPS, latency p50/p95/p99, endorser/orderer metrics, server tag).
  7. Teardown: clean, idempotent down + volume prune so the next run starts blank.
- **Why not a predictive simulation:** Fabric throughput doesn't simulate accurately;
  `startup.sh` produces *measured* numbers, which is what the dissertation needs.

### 2.4 fabric-samples vendored into repo (user did the move)
- Location: `blockchain/fabric-samples/` (bin/, config/, test-network).
- **References still to update:** `deploy.sh:38` default, `onboard-org3.sh:18`,
  `smoke-test.sh:15`, `register-orgs.sh:16`, `blockchain.py:64` default, and docs
  (`instructions.txt/.md` line 92, `explanation`, `DEPLOYMENT.md`) — all currently
  still point at `~/fabric-samples`.
- **.gitignore:** already ignores `blockchain/fabric-samples/*` with a whitelist for
  `L*` (LICENSE). Must add the generated output dirs explicitly so the tree can be
  vendored *with* a partial layout but without `organizations/`, `channel-artifacts/`,
  `configtx/`, `*.block`, `log.txt`, `*.tar.gz`. Keep the Apache-2.0 license headers.
- **Constraint:** relative layout preserved (`test-network` needs sibling `bin/` +
  `config/`; `addOrg3.sh` uses `${PWD}` and must run from its own dir — already handled
  in `onboard-org3.sh`).

### 2.5 Explorer (v3 stretch, not core)
- Public, read-only auditor UI over chain history + off-chain report info. Uses the
  read-only query endpoints (already the design intent from v2 §5); non-participating
  identity. Hyperledger Explorer itself was evaluated and removed (`explorer/` deleted);
  v3 would implement a thin read-only dashboard instead, only if time allows.

---

## 3. Implementation Plan (phased, buildable)

### Phase V3.1 — Config & reference cleanup (low risk) ✔ DONE
1. Update fabric-samples references in the 4 scripts + `blockchain.py` + 4 docs (§2.4).
2. Extend `.gitignore` for the generated fabric-samples output dirs.
3. `bash -n` all scripts; re-run `go test ./...`; `python -m py_compile` on
   `storage.py`/`server.py`/`blockchain.py`.
4. **Accept:** a fresh `./scripts/deploy.sh` works with the vendored tree, no
   `~/fabric-samples` references remain (grep clean), git status clean of generated
   artifacts.

### Phase V3.2 — `--orgs N` wiring (finish the chaincode work) ✔ DONE
1. `deploy.sh`: parse `--orgs N` (default 3); after `deployCC`, `peer chaincode invoke`
   `SetFoundingOrgLimit N`; call `register-orgs.sh` to self-register orgs 1..N.
2. `register-orgs.sh`: loop 1..N, `RegisterOrg` for each configured MSP.
3. Update `misinformation_test.go`: dynamic-limit test (set 5 → 4th org OK; set 2 →
   3rd org rejected).
4. **Accept:** `--orgs 2`, `--orgs 3`, `--orgs 6` each bring up a network where the
   requested org count self-registers and a full smoke test (register→submit→vote→
   finalize→query) passes.

### Phase V3.3 — Encryption-style gate ✔ DONE
1. Add `CRYPTO_STYLE` (ecdsa|ed25519|rsa) to the onboarding gate in `storage.py`.
2. PyCrypto primitives: verify/decrypt timings logged; pass/fail fixed (always accept).
3. Expose active style via `/api/status`; wire `bootstrap-keys.sh` to emit the
   per-style keypair.
4. **Accept:** unit test each style returns accept; timing delta measurable (rsa >
   ecdsa > ed25519); `/api/status` reports the style.

### Phase V3.4 — `startup.sh` (the start switch) + load driver ✔ DONE
1. Build `startup.sh`: arg parsing for all 13 variables, defaults, `--help`, and
   validation against the §1 maxes (reject out-of-range with a clear error).
2. Wire it to the existing pieces: `deploy.sh` (topology + state_db + policy),
   `SetFoundingOrgLimit` invoke, `register-orgs.sh`, and the load drivers.
3. Load driver: `benchmarks/load-http.py` (stdlib HTTP, drives the FastAPI gateway)
   — Caliper hook present (`--driver caliper`) but Caliper config is future work.
4. `fault_inject` orchestration: mid-run `docker stop` of a peer and an orderer;
   measure recovery + Raft view-change latency.
5. `benchmarks/record.py` → `benchmarks/results.csv` (args snapshot + metrics per run).
6. **Accept (verified):** `./startup.sh --orgs 6 --concurrency 150 --rate 300` runs
   end-to-end (stub-deploy + HTTP load + record); 3 reruns of identical args produce
   TPS/latency within ±15%; out-of-range args are rejected with clear errors.

### Phase V3.5 — Multi-server / spec comparison
1. Two-host run (second machine): split orderers+peers across hosts, distinct IPs.
2. Run the headline scenario set on the enterprise box vs baseline; tag `server_spec`.
3. **Accept:** identical args yield a server-spec comparison table.

### Phase V3.6 — Dissemination
1. Consolidate variable spec + methodology + results into the dissertation
   Phase 9A/9B write-up (this document §1 + §4 is the canonical spec).
2. Cross-ref in `PROJECT_ROADMAP.md` (this section maps to Phase 9A/9B).

---

## 4. Suggested Run Set (values are examples, not fixed levels)

Since every numeric value is dynamic (§1), the matrix below is a **suggested starting
set** for the dissertation write-up — each row is one `startup.sh` invocation, and the
numbers shown are example values you may change to any in-range value per run. One run
= one row. Rows in **bold** are the headline results for the write-up.

| # | Scenario | orgs | peers/org | orderers | state_db | batch_t/o | max_msg | policy | concurrency | rate | rw_mix | fault |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | baseline | 3 | 1 | 3 | couchdb | 2s | 10 | 2-of-3 | 24 | 25 | 100 | none |
| **2** | **scale-orgs** | **6** | 1 | 3 | couchdb | 2s | 10 | 2-of-3 | 24 | 25 | 100 | none |
| 3 | scale-peers | 3 | 2 | 3 | couchdb | 2s | 10 | 2-of-3 | 24 | 25 | 100 | none |
| 4 | orderers-1 | 3 | 1 | 1 | couchdb | 2s | 10 | 2-of-3 | 24 | 25 | 100 | none |
| **5** | **state-db** | 3 | 1 | 3 | **leveldb** | 2s | 10 | 2-of-3 | 24 | 25 | 100 | none |
| 6 | batch-timeout | 3 | 1 | 3 | couchdb | **5s** | 10 | 2-of-3 | 24 | 25 | 100 | none |
| 7 | block-size | 3 | 1 | 3 | couchdb | 2s | **500** | 2-of-3 | 24 | 25 | 100 | none |
| 8 | policy-3of3 | 3 | 1 | 3 | couchdb | 2s | 10 | **3-of-3** | 24 | 25 | 100 | none |
| **9** | **concurrency** | 3 | 1 | 3 | couchdb | 2s | 10 | 2-of-3 | **200** | 25 | 100 | none |
| **10** | **rate** | 3 | 1 | 3 | couchdb | 2s | 10 | 2-of-3 | 24 | **400** | 100 | none |
| **11** | **rw-mix** | 3 | 1 | 3 | couchdb | 2s | 10 | 2-of-3 | 24 | 25 | **50** | none |
| 12 | fault-peer | 3 | 1 | 3 | couchdb | 2s | 10 | 2-of-3 | 24 | 25 | 100 | **kill-peer** |
| 13 | fault-orderer | 3 | 1 | 3 | couchdb | 2s | 10 | 2-of-3 | 24 | 25 | 100 | **kill-orderer** |
| 14 | crypto-ecdsa | 3 | 1 | 3 | couchdb | 2s | 10 | 2-of-3 | 200 | 25 | 100 | none |
| 15 | crypto-rsa | 3 | 1 | 3 | couchdb | 2s | 10 | 2-of-3 | 200 | 25 | 100 | none |
| 16 | crypto-ed25519 | 3 | 1 | 3 | couchdb | 2s | 10 | 2-of-3 | 200 | 25 | 100 | none |

Stretch cells (only if time): #3's level-2, per-invoke endorsement 4, pre-seeded 100K,
`rw_mix` R100 (writes 0). Crypto rows 14–16 differ only in `--crypto-style`.

---

## 5. Metrics & Reproducibility

- **Metrics per run:** achieved TPS, latency p50/p95/p99, failure rate, endorser CPU,
  orderer CPU, block cut rate; for fault runs: time-to-recovery, view-change latency.
- **Reproducibility:** network fully torn down and re-provisioned per run
  (`network.sh down` + volume prune), fixed Docker images, exact `startup.sh` args
  snapshot recorded with every run, 3 reruns ±15% acceptance.
- **Single-host caveat (must be stated in write-up):** the CLI-shell-out bridge and
  one-machine topology cap absolute throughput; Caliper/SDK + LevelDB is the only route
  near the aspirational 1500–3000 TPS, and 1500–3000 is a stretch target to measure
  against, not a claim to be reached.

---

## 6. Payload Realism (v3 note, no core variable)

Raw report text never enters the ledger (hash + `ipfs://CID` only, per v2 §3). So
"payload size" is **not** a chaincode variable. Realistic coverage:
- **On-chain:** fixed small writes (hash, URI, votes) — no size knob needed.
- **Off-chain:** large-file handling (up to tens of MB) tested at the IPFS/API layer
  separately, not through the chaincode path. Any big-payload chaincode test would
  require deliberately bloating the write set (a code change, not a config), which is
  out of v3 scope.

---

## 7. Risks & Limitations

| Risk | Mitigation |
|---|---|
| Single-host throughput ceiling | State it explicitly; use Caliper for high-rate, HTTP for user-path; multi-host in V3.5 |
| CouchDB write degradation | It's a *result*, not a bug — leveldb scenario gives the comparison |
| MVCC read-write conflicts at R50W50 | Expected finding; report failure rate as part of metrics |
| `foundingOrgLimit` cap on org scale | `--orgs N` exists precisely to raise it; genesis default stays 3 |
| fabric-samples move breaking relative layout | Preserve bin/config sibling layout; `addOrg3.sh` runs from its own dir |
| Fake crypto gate skewing "real" numbers | Gate is app-layer load shaping only; Fabric real X.509 still verifies every tx; document as such |
| Out-of-range args silently clamped | `startup.sh` rejects values above max with a clear error (no clamp) |
| Time overruns | Suggested set is 16 runs; stretch cells optional |

---

## 8. Cross-Reference

| v3 section | Roadmap phase | Status |
|---|---|---|
| §2.1 dynamic founding limit | Phase 9B (performance) | DONE (chaincode + deploy.sh `--orgs N`) |
| §2.2 encryption-style gate | Phase 9B (performance) | DONE (`CRYPTO_STYLE` in storage.py + `/api/status`) |
| §2.3 startup.sh (start switch) + load driver | Phase 9B | DONE (`startup.sh`, `benchmarks/load-http.py`, `benchmarks/record.py`) |
| §2.4 fabric-samples vendoring | Phase 0 (hygiene) + 9A | DONE (move + references + .gitignore) |
| §2.5 explorer (read-only dashboard) | Phase 7.3 (public transparency) | stretch |
| §1 variable spec | Phase 9A/9B write-up | canonical spec |
| §4 suggested run set | Phase 9B | to execute (live runs) |

**Backlog / status:** V3.1–V3.4 **DONE** (built + verified: fabric-samples references,
`--orgs N`, encryption-style gate, `startup.sh` + HTTP load driver + results.csv).
V3.5–V3.6 not started. Live network end-to-end run (real `deploy.sh`, not the stub)
still to be executed once the environment allows.

---

## 9. How to Use v3

### 9.1 One-shot run (recommended)

`startup.sh` is the single entry point — pass the variables you want as args, omit
the rest to get the defaults:

```bash
# Defaults: 3 orgs, couchdb, 2-of-3 policy, concurrency 24, 25 TPS, 100% writes
./startup.sh

# Dynamic values — orgs 6, LevelDB, 3-of-3 policy, RSA onboarding gate, 150 workers @ 300 TPS
./startup.sh --orgs 6 --state-db leveldb --policy 3of3 --crypto-style rsa \
             --concurrency 150 --rate 300 --tag scale6

# Read-heavy mix + fault injection
./startup.sh --rw-mix 25 --fault kill-peer --tag read-heavy-fault
```

The script does, in order:

1. Parses + validates every arg against its hard max (§1) — out-of-range values are
   rejected with a clear error, never silently clamped.
2. Provisions the network via `blockchain/scripts/deploy.sh` (topology, state DB,
   endorsement policy, and `--orgs N` → `SetFoundingOrgLimit` + self-registration).
3. Runs the load phase (default `--driver http` → `benchmarks/load-http.py` against
   the FastAPI gateway).
4. Optionally injects a fault (`--fault kill-peer | kill-orderer` → mid-run
   `docker stop`, then restart).
5. Appends one row to `benchmarks/results.csv` via `benchmarks/record.py` (full arg
   snapshot + TPS + latency p50/p95/p99 + failure rate).
6. Prints teardown instructions.

`--help` prints the full variable table with defaults and maxes.

### 9.2 Prerequisites

```bash
# 1. Install API deps (cryptography is required for the onboarding gate):
pip install -r apps/ai_service/v1-0-0/app/api/requirements.txt

# 2. Start the FastAPI gateway (the HTTP load driver targets it):
#    (from apps/ai_service/v1-0-0/app)
#    uvicorn server:app --host 0.0.0.0 --port 8000

# 3. Docker running + fabric-samples present at blockchain/fabric-samples/
```

### 9.3 Individual pieces (if you prefer manual steps)

```bash
# Deploy only (bring up network + chaincode, no load):
./blockchain/scripts/deploy.sh                 # 3-org default
./blockchain/scripts/deploy.sh --orgs 6        # raise founding limit to 6 + register
./blockchain/scripts/deploy.sh -2              # 2-org baseline only
./blockchain/scripts/deploy.sh down            # tear down

# Register orgs only:
./blockchain/scripts/register-orgs.sh                       # auto-detect available orgs
./blockchain/scripts/register-orgs.sh --limit 6             # up to founding limit 6

# Load driver standalone (against a running gateway):
python3 benchmarks/load-http.py --base http://localhost:8000 \
        --concurrency 100 --rate 200 --rw-mix 50 --samples 500

# Record a run by hand:
python3 benchmarks/record.py --csv benchmarks/results.csv \
        --tag manual --orgs 3 --state-db couchdb --policy 2of3 \
        --concurrency 100 --rate 200 --load-out "achieved_tps=95.2
p50_ms=12.3
p95_ms=40.1
p99_ms=62.0"
```

### 9.4 Onboarding gate crypto style

Set `CRYPTO_STYLE` (ecdsa | ed25519 | rsa) either as an env var before starting the
server, or via `startup.sh --crypto-style`. The active style and last verify time are
exposed at `GET /api/status` (`crypto_style`, `crypto_verify_ms`). Default is `ecdsa`.

### 9.5 Reading the results

`benchmarks/results.csv` is one row per run. Columns: run snapshot (tag, orgs,
peers_per_org, orderers, server, batch_timeout, max_message, state_db, policy,
crypto_style, concurrency, rate, rw_mix, fault, driver) + measured metrics (total,
elapsed_s, achieved_tps, p50_ms, p95_ms, p99_ms, failures, failure_rate) + timestamp.
The args are recorded verbatim so every row is reproducible.

### 9.6 Suggested run set (16 rows)

The §4 table lists the suggested starting set (baseline, scale-orgs, state-db,
concurrency, rate, rw-mix, fault-peer/orderer, crypto-sweep ...). Each row maps to one
`startup.sh` invocation with the row's values as args; all numbers are dynamic so you
can change any of them per run.

