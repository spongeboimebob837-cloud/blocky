# Chaincode Data Model — Decision Record

Milestone **B1** (sub-steps 1.a–1.f), extended for the consortium review workflow
(v1.1) and the **v2** report-lifecycle + verified-member upgrades. Status: **decided**.
Written for the dissertation's blockchain section.

## Overview

- Reports are audit records keyed by a caller-supplied **report_id**; raw text
  never touches the ledger — only a SHA-256 hash plus an `off_chain_uri` pointing
  to the full report (raw text lives off-chain).
- Stakeholder membership is **two-tier**: Tier-1 channel membership (Fabric,
  hard permissioning) plus an on-chain **admission vote** for Tier-2 stakeholder
  status, reusing the 2/3 quorum used for reports.
- Each report carries a **voting deadline**; undecided reports can become
  `EXPIRED`.

## 1.a — On-chain fields (never raw text)

| Field            | Type     | Meaning                                                        |
|------------------|----------|----------------------------------------------------------------|
| `report_id`      | string   | Ledger identity of the report (submitter-supplied, e.g. `nso-42-v1`) |
| `language`       | string   | Model input language: `nso` (Sepedi), `zul` (isiZulu), `eng`    |
| `content_hash`   | string   | SHA-256 hex of the canonical off-chain Report object (integrity anchor) |
| `proposed_label` | string   | `"0"` reliable / `"1"` misinformation (claimed by the submitter)|
| `confidence`     | float64  | Model confidence in `[0,1]`                                    |
| `model_version`  | string   | Model + dataset version, e.g. `afroxlmr-large-nso-v1.0`         |
| `timestamp`      | string   | RFC 3339 submission time (UTC)                                 |
| `submitted_by`   | string   | MSP ID of the submitting organisation (from tx context)        |
| `off_chain_uri`  | string   | URL to the full off-chain report (raw text lives there, never on-chain) |
| `voting_deadline`| string   | RFC 3339 UTC; submission tx-time + 72h (deterministic per peer) |
| `status`         | string   | `PENDING` → `FINAL`, `REJECTED`, or `EXPIRED`                  |
| `votes`          | Vote[]   | Each stakeholder org's `{voter_msp, verdict, txid}`            |
| `finalized_by`   | string   | MSP that ran `FinalizeReport`/`ExpireReport`                   |
| `finalized_at`   | string   | RFC 3339 time finalised / expired                              |

`content_hash` is computed over the **whole canonical Report object** (v2 §1.2),
not just the raw text, so tampering with any field is detectable. Canonical
serialization must be byte-stable (sorted keys, no whitespace) — see
`app/src/report.py`.

## 1.b — Lookup keys

- **Reports:** composite `pred:{report_id}` — each report is uniquely keyed by its
  report_id.
- **Stakeholder orgs:** composite `org:{mspid}` — one entry per registered
  fact-checking org.
- **Votes:** composite `vote:{report_id}:{mspid}` — one vote per org per report,
  guaranteeing one-vote-per-org at the storage level.
- **Admission requests:** composite `admission:{candidateMSP}`.
- No rich queries (CouchDB) needed for the core model; with a CouchDB state DB the
  API can filter by field (status/language/time) directly. State DB defaults to
  CouchDB in `deploy.sh` (v2 §6).

## 1.c — Struct draft (before Go)

Written in `go/misinformation.go` as `type ReportRecord`, `type Vote`,
`type RegisteredOrg`, `type OrgAdmissionRequest`.

## 1.d — Locked field types

- `timestamp` / `voting_deadline` / `finalized_at` — RFC 3339 UTC string (not epoch).
- `confidence` — JSON `float64` in `[0,1]`; validated in chaincode.
- `proposed_label` — string `"0"` / `"1"`.
- `content_hash` — lowercase hex SHA-256, validated length 64.
- `off_chain_uri` — non-empty string (the full report is off-chain by contract).
- `verdict` — string `"accept"` / `"reject"` (validated in `CastVote` /
  `VoteOnOrgAdmission`).

## 1.e — Immutability policy

- Reports start **PENDING**; votes are appended while pending (each prior value is
  preserved in ledger key history).
- `FinalizeReport` locks once **≥ 2/3 of registered orgs** have voted: `FINAL` if
  quorum accepted, else `REJECTED`. If called after `voting_deadline` without
  quorum, the report auto-transitions to `EXPIRED`.
- `ExpireReport` explicitly marks a past-deadline PENDING report `EXPIRED`.
- After `FINAL`/`REJECTED`/`EXPIRED` the record is immutable — no updates, no
  votes, no expiry.
- Prior values retrievable via `GetHistoryForKey` — the inherent Fabric audit
  trail (tamper-evidence + provenance).

## 1.f — Quorum rule

- **Quorum = ceil(2 × N / 3)** where N = number of registered stakeholder orgs.
- 2 orgs → 2, 3 → 2, 4 → 3. Verdict = `FINAL` if accepted votes ≥ quorum.
- The same `quorumFor` is reused for **org admission** votes.

## 1.g — Verified stakeholder membership (v2 §2)

Two tiers:

- **Tier-1 — channel membership (hard).** Only orgs whose MSP is part of the
  channel config can submit a signed transaction at all. Adding org3 required a
  channel config update (`addOrg3.sh`), not an API call.
- **Tier-2 — on-chain stakeholder status.** `RegisterOrg` is **genesis-only**
  (self-service until `foundingOrgLimit` = 3 orgs are registered; after that it
  errors). New orgs use:
  - `RequestOrgAdmission(orgName, orgType)` → PENDING `admission:{msp}`,
  - `VoteOnOrgAdmission(candidateMSP, verdict)` — only registered orgs may vote;
    a candidate cannot vote on itself,
  - `FinalizeOrgAdmission(candidateMSP)` — at 2/3 quorum writes `org:{msp}`
    (ADMITTED) or marks REJECTED.

> **Bootstrapping decision (explicit):** the first `foundingOrgLimit` (3) orgs are
> self-registered via `RegisterOrg` (genesis allowlist). Every org after that must
> be voted in by the existing consortium. Real-world vetting happens off-chain
> before admission (application form reviewed by the network operator).

## 1.h — Report lifecycle summary

`SubmitReport` (PENDING, with `off_chain_uri` + 72h `voting_deadline`) →
`CastVote` → `FinalizeReport` (FINAL/REJECTED) or `ExpireReport` (EXPIRED after
the deadline). A scheduled job in the API layer expires overdue PENDING reports
(chaincode is purely reactive and cannot run timers itself).

Decision summary (for dissertation):

> Fact-checking organisations are admitted to the consortium by genesis bootstrap
> and thereafter by on-chain admission vote (`admission:{msp}`). A registered org
> submits a report keyed `pred:{report_id}` carrying the model output, an integrity
> hash over the whole canonical report, and an `off_chain_uri` to the raw text —
> never the text itself. Every registered org casts one accept/reject vote
> (`vote:{report_id}:{mspid}`). Once ≥ 2/3 of registered orgs have voted, the report
> is finalised immutably (or expires after its 72h window), recording the full vote
> tally and provenance. Fabric's key-history API provides the tamper-evident trail
> of the whole lifecycle.