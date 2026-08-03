# Misinformation Monitoring Chaincode

Hyperledger Fabric chaincode that anchors fact-checking reports on a tamper-evident,
permissioned ledger — the blockchain audit layer of the misinformation-monitoring
framework (Track B of the project plan).

## Design

- **Data model & decisions:** `DATA_MODEL.md` (Milestone B1 / sub-steps 1.a–1.g).
- **Style:** `contractapi` (Go), not `shim` (B2). `go/misinformation.go`.
- **On-chain fields:** `row_id`, `language`, `content_hash` (sha256 of text — never raw text),
  `proposed_label`, `confidence`, `model_version`, `timestamp`, `submitted_by` (MSP from tx
  context), `status`, `votes[]`, `finalized_by`, `finalized_at`.
- **Consortium review workflow (v1.1):** organisations self-register (`RegisterOrg`); a
  registered org submits a PENDING report; every registered org casts one `accept`/`reject`
  vote; the report is finalised immutably once **≥ 2/3 of registered orgs** have voted —
  `FINAL` if quorum accepted, else `REJECTED`. Votes close after finalisation.

## Functions (B3)

| Function | Type | Purpose |
|----------|------|---------|
| `RegisterOrg` | invoke | Self-enroll the calling org as a stakeholder |
| `ListRegisteredOrgs` | query | List enrolled stakeholder orgs |
| `SubmitReport` | invoke | Submit a PENDING report (registered org only) |
| `CastVote` | invoke | One `accept`/`reject` vote per org per report |
| `FinalizeReport` | invoke | Lock report once ≥ 2/3 orgs voted (FINAL/REJECTED) |
| `QueryReport` | query | Fetch report by `(language, row_id)` |
| `QueryAllReports` | query | Dump all reports |
| `GetReportCount` | query | Count reports |
| `QueryVotes` | query | Vote tally for a report |
| `QueryReportHistory` | query | Ledger key history (audit trail) |
| `ComputeContentHash` | query | sha256 of a text (verifies bridge hashes) |

## Topology & deployment (B4–B6)

- **Topology:** reuses the canonical `fabric-samples/test-network` (2 orgs, Raft, LevelDB) —
  deliberately simple, platform-agnostic per the proposal. Adding a 3rd org (robustness test):
  `test-network/addOrg3.sh` + switch the endorsement policy in `deploy.sh`
  (`-ccep "OutOf(2, 'Org1MSP.member','Org2MSP.member','Org3MSP.member')"`).
- **REST layer: deliberately deferred (B5).** All interaction is CLI (`peer`) or the Python
  bridge; the old `app.js`/api-1.4/api-2.0 balance-transfer wrappers are not used by this chaincode.

```bash
# lifecycle: package -> install (both orgs) -> approve (both orgs) -> commit
./scripts/deploy.sh            # up + createChannel + deployCC
./scripts/deploy.sh down       # tear down
./scripts/smoke-test.sh        # register -> submit -> vote -> finalize -> query (CLI)

# one-time dep for the fabric scripts:
#   install jq  (e.g. jq-linux-amd64 -> ~/.local/bin/jq)
```

## Python bridge (C1) and Colab/Kaggle packaging (C2)

- `apps/ai_service/v1-0-0/app/src/blockchain.py` — `Prediction` + `FabricBridge`
  (CLI shell-out; hashes with sha256, never sends raw text). Review-workflow methods:
  `register_org`, `list_orgs`, `submit_report`, `cast_vote`, `finalize_report`,
  plus `query_report`/`query_all`/`count`/`votes`/`history`. See `DEPLOYMENT.md`.

## Tests

```bash
cd go && go test ./...   # MockStub unit tests: registration, submit/query,
                         # validation, vote->finalize workflow, quorum, rejection, count/all
```
