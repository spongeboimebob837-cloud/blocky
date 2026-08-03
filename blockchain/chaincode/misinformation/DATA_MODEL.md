# Chaincode Data Model — Decision Record

Milestone **B1** (sub-steps 1.a–1.f), extended for the consortium review workflow (v1.1).
Status: **decided**. Written for the dissertation's blockchain section.

## 1.a — Fields on-chain (not raw text)

The ledger stores the *model's audit fingerprint* for a text item, never the raw text.

| Field            | Type     | Meaning                                                              |
|------------------|----------|----------------------------------------------------------------------|
| `row_id`         | string   | Source dataset row identifier                                        |
| `language`       | string   | Model input language: `nso` (Sepedi), `zul` (isiZulu), `eng`         |
| `content_hash`   | string   | SHA-256 hex digest of the text (integrity anchor)                    |
| `proposed_label` | string   | `"0"` reliable / `"1"` misinformation (claimed by the submitter)     |
| `confidence`     | float64  | Model confidence in `[0,1]`                                          |
| `model_version`  | string   | Model + dataset version, e.g. `afroxlmr-large-nso-v1.0`              |
| `timestamp`      | string   | RFC 3339 submission time (UTC)                                       |
| `submitted_by`   | string   | MSP ID of the submitting organisation (from the transaction context) |
| `status`         | string   | `PENDING` → `FINAL` (accepted) or `REJECTED`                         |
| `votes`          | Vote[]   | Each stakeholder org's `{voter_msp, verdict, txid}`                  |
| `finalized_by`   | string   | MSP that ran `FinalizeReport`                                        |
| `finalized_at`   | string   | RFC 3339 time the report was finalised                               |

## 1.b — Lookup keys

- **Reports:** composite `pred:{language}:{row_id}`.
  The same row appears in multiple languages, so the key is the (language, row_id) pair.
- **Stakeholder orgs:** composite `org:{mspid}` — one entry per registered fact-checking org.
- **Votes:** composite `vote:{language}:{row_id}:{mspid}` — one vote per org per report,
  guaranteeing one-vote-per-org at the storage level.
- No rich queries (CouchDB) needed — state DB stays LevelDB for simplicity.

## 1.c — Struct draft (before Go)

Written in `go/misinformation.go` as `type ReportRecord`, `type Vote`, `type RegisteredOrg`.
JSON tags are the snake_case column names the Python bridge (Track C) will produce.

## 1.d — Locked field types

- `timestamp` / `finalized_at` — RFC 3339 UTC string (not epoch int).
- `confidence` — JSON number `float64` in `[0,1]`; validated in chaincode (`<0` or `>1` rejected).
- `proposed_label` — string `"0"` / `"1"` (kept as string so downstream tools never coerce booleans).
- `content_hash` — lowercase hex SHA-256, validated length 64.
- `verdict` — string `"accept"` / `"reject"` (validated in `CastVote`).

## 1.e — Immutability policy

- Reports start **PENDING**. While pending, votes are appended (the record is updated as the
  tally grows, and every prior value is preserved in the ledger's key history).
- `FinalizeReport` locks a report once **≥ 2/3 of registered orgs** have voted: `FINAL` if at
  least quorum accepted, `REJECTED` otherwise. After finalisation the record is immutable —
  no updates, no further votes (`CastVote` rejects on non-PENDING).
- Prior values of a key are preserved and retrievable via `GetHistoryForKey` — the inherent
  Fabric audit trail.
- This satisfies "tamper-evident audit of what the consortium decided, with provenance" — the
  framing in the proposal.

## 1.f — Quorum rule

- **Quorum = ceil(2 × N / 3)** where N = number of registered stakeholder orgs.
- With 2 orgs → 2 votes required (both). With 3 → 2. With 4 → 3. Etc.
- Final verdict = `FINAL` if accepted votes ≥ quorum, else `REJECTED`.

## 1.g — Decision summary (for dissertation)

> Fact-checking organisations self-register on-chain (`org:{mspid}`). Each registered org can
> submit a report (`pred:{language}:{row_id}`) carrying the model output and an integrity hash —
> never the raw text — and every registered org casts one `accept`/`reject` vote
> (`vote:{language}:{row_id}:{mspid}`). A report is finalised immutably once ≥ 2/3 of registered
> orgs have voted, recording the full vote tally and provenance (MSP + txid). The Hyperledger
> Fabric key-history API provides the tamper-evident trail of the whole lifecycle.
