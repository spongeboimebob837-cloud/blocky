# A Blockchain-Enabled Framework for Misinformation Monitoring
## Full Project Roadmap — From Current State to Submission

**Stated purpose (anchor for every decision below):** a permissioned Hyperledger Fabric
ledger that fact-checking organisations can actually use as a shared, tamper-evident
system to submit, review, and vote on AI-flagged misinformation in low-resource South
African languages (Sepedi/Northern Sotho, isiZulu), with the AI/NLP layer producing the
flagged content and the blockchain layer providing consortium-level auditability.

Every phase below is written against that anchor. Where something looks technically
impressive but doesn't serve "a fact-checker could use this," it's flagged as polish,
not core.

---

## How to read this document

- **Phase** = a logical unit of work, roughly session-sized or a small multi-session block.
- **Depends on** = what must be done first.
- **Effort** = rough sessions (a "session" ≈ 2-4 focused hours), not calendar time.
- **Definition of Done (DoD)** = the concrete, checkable thing that proves the phase is finished.
- **Risk** = what's likely to go wrong, so you can budget slack.

Track letters (A/B/C/D...) map to workstreams so you can jump around; Phase numbers give
you the recommended execution order.

---

## PHASE 0 — Repo Hygiene & Stabilization
**Track:** Housekeeping | **Depends on:** nothing | **Effort:** 1 session

Nothing else in this roadmap is trustworthy until the repo actually runs. This phase is
about making every file in the repo either work or honestly not exist.

### 0.1 Fix broken files
- [ ] `preprocessing.py`: fix the empty `if __name__ == "__main__":` block (code below it
  is currently unindented — will either raise `IndentationError` or run at import time).
- [ ] `preprocessing.py`: fix the duplicated `stats.min`/`stats.std` typo in the print
  statement (currently prints `stats.std` twice, one mislabeled as min).
- [ ] `colab_pipeline.py`: replace the placeholder `print("Asshole")` stub with either:
  - a real Colab/Kaggle entrypoint that chains `data_prep.py` → `data_parser.py` →
    training notebook invocation, **or**
  - delete the file and update `README.md`/`updates`/`tree.txt` references to point at
    the actual notebooks (`notebook_launcher.ipynb`, `fine_tuning_training.ipynb`).
  - **Decide which within this phase** — don't leave it ambiguous into later phases.
- [ ] `model_config.py`, `__init__.py`: confirm nothing imports from these expecting
  content. If genuinely unused, either populate with real shared config (model names,
  paths, label maps currently duplicated across notebooks) or remove.

### 0.2 Dependency sanity check
- [ ] `notebook_launcher.ipynb` pins `transformers>=5.12.0,<6.0.0` and
  `datasets>=5.0.0,<6.0.0`. Run the install cell fresh in a clean Kaggle/Colab kernel and
  confirm these resolve to real, intended package versions (not a typo for `4.x`).
- [ ] Confirm `sacremoses`, `jiwer`, `scikit-learn`, `scipy`, `nltk` versions in the same
  cell are all still needed (WER work needs `jiwer`; confirm `nltk` is actually used
  somewhere, or drop it).

### 0.3 Git tracking cleanup
- [ ] Run `git ls-files | grep vendor` — if `blockchain/chaincode/misinformation/go/vendor/`
  is tracked despite being in `.gitignore`, remove it: `git rm -r --cached <path>` then
  commit. A committed vendor tree (hundreds of files, all third-party) bloats the repo
  and looks like an oversight to an examiner browsing GitHub.
- [ ] Same check for `blockchain/chaincode/misinformation/go/misinformation` (the compiled
  binary — also gitignored, confirm it's not tracked).
- [ ] Confirm `data/` (1.8GB, gitignored) isn't accidentally tracked from before the
  ignore rule existed: `git ls-files | grep "^apps/ai_service/v1-0-0/app/data/"`.

### 0.4 Dead code cleanup
- [ ] `data_prep.py`: the batch-translation build/submit/compile functions
  (`build_batch_jsonl`, `submit_batch_job`, `check_job_status`,
  `compile_results_to_dataframe`, `get_dataset`, `clean_text`, `split_text_by_chars`) are
  all commented out. Decide: are these historical (translation already done, keep as
  reference) or should they be restored to a runnable state for isiZulu (Phase 6 needs
  them)? **This decision blocks Phase 6 planning — resolve it here.**
- [ ] If keeping as reference, move to a clearly-labeled `scripts/archive/` or add a
  top-of-file comment explaining why it's commented out and what replaced it.

**DoD for Phase 0:** `git clone` the repo fresh, run every `.py` file's `--help` or
top-level import with no errors, and every notebook's first two cells execute cleanly in
a fresh kernel.

---

## PHASE 1 — Exploratory Data Analysis (EDA)
**Track:** A (AI/NLP) | **Depends on:** Phase 0 | **Effort:** 1–2 sessions

This is one of your four stated proposal deliverables and currently has **zero**
representation in the repo. Low technical risk, straightforward to produce, and directly
strengthens your methodology chapter.

### 1.1 Dataset-level EDA (English source)
- [ ] Class balance (misinformation vs. reliable) — bar chart + exact counts.
- [ ] Text length distribution (chars and tokens) — histogram, flag outliers (very short
  tweets, very long threads).
- [ ] Source distribution if `source` column is populated (currently often "unspecified"
  per `data_prep.py`'s dead `clean_text` function — confirm what fraction).
- [ ] Duplicate/near-duplicate row check (exact text duplicates at minimum).

### 1.2 Translation-quality EDA (Sepedi)
- [ ] Sample 20–30 rows at random, human-read (or use your own Sepedi/isiZulu speaker
  contacts if available) the `text` (English) → `translated` (Sepedi) →
  `back_translated` (English) triples. Flag obviously broken translations.
- [ ] Character length ratio: Sepedi text length vs. English source length (sanity check
  for truncation/garbling).
- [ ] Missing-value audit: rows where `translated` or `back_translated` is null/empty —
  quantify, decide whether to drop or re-translate.

### 1.3 Anonymization audit (ties into Phase 2.1 below but belongs in EDA output)
- [ ] Count remaining `@\w+` patterns, raw URLs, and photo-credit patterns in the
  `translated`/`back_translated` columns specifically (your notes flag this as
  unconfirmed) — produce a before/after count table.

### 1.4 Deliverable
- [ ] One notebook: `apps/ai_service/v1-0-0/app/src/eda.ipynb` (or `eda_sepedi.ipynb` +
  `eda_english.ipynb` if you split them), with saved output figures in
  `apps/ai_service/v1-0-0/app/data/processed/eda_figures/` (gitignored is fine — figures
  can be regenerated, or export the key ones as PNGs directly into a `docs/figures/`
  folder that *is* committed for direct dissertation embedding).

**DoD:** notebook runs top-to-bottom on a fresh kernel against the current CSVs and
produces at least: class balance, length distributions, missing-value counts,
anonymization leak counts — each with a one-line written interpretation.

---

## PHASE 2 — Data Quality Fixes (blocking Sepedi re-run)
**Track:** A | **Depends on:** Phase 1.3 | **Effort:** 1 session

### 2.1 Anonymization completeness
- [ ] If Phase 1.3 finds leaks in `translated`/`back_translated`, re-run
  `anonymize_text()` (from `data_prep.py`) against those columns specifically — currently
  the `__main__` block applies it to *all* columns of whatever CSV is passed, so confirm
  the Sepedi CSV was actually passed through `data_prep.py --input_file <sepedi_csv>` at
  some point, not just the English raw file.

### 2.2 Mojibake / encoding artifacts
- [ ] Write a small detection pass: scan for common mojibake signatures (`Ã©`, `â€™`,
  replacement character `�`, etc.) across `text`/`translated`/`back_translated`.
- [ ] Fix at the source if possible (re-read with correct encoding) or add a cleanup
  regex step to `anonymize_text()` / a new `clean_encoding()` function.

### 2.3 URL pattern coverage gaps
- [ ] Extend `CLEAN_PATTERN` / `anonymize_text()` regex to catch URL shorteners
  (`bit.ly`, `t.co`) and non-`https` prefixed bare domains if present in the data —
  quantify the gap first (how many rows still contain a recognisable URL after current
  anonymization) before deciding it's worth the regex work.

**DoD:** re-run the Phase 1.3 leak-count check — target near-zero remaining leaks in the
anonymized columns. Document the before/after numbers in the EDA notebook or a short
`data_quality_notes.md`.

---

## PHASE 3 — Sepedi Fine-Tuning Re-Run (the first real reportable non-English number)
**Track:** A | **Depends on:** Phase 2 | **Effort:** 1 session (mostly GPU wait time)

- [ ] Confirm `SEPEDI_CSV_PATH` in `fine_tuning_training.ipynb` points at the
  Phase-2-cleaned CSV.
- [ ] Run `training.ipynb` end to end on Kaggle T4: sanity check (1 epoch, small subset)
  first, then the full 15-epoch run with early stopping.
- [ ] Upload the resulting `afroxlmr_sepedi_final` folder as a Kaggle Model.
- [ ] Edit `fine_tuning_test.ipynb`'s `model_path` to the uploaded model, run **once**.
- [ ] Save `sepedi_final_test_results.json` — this is your first clean, reportable Sepedi
  number for the dissertation.
- [ ] Record: accuracy, F1, recall, precision, and briefly interpret against the (already
  known to be leaky/non-reportable) English baseline — is Sepedi harder, as expected for
  a lower-resource language? Write 2–3 sentences now while it's fresh, for later
  dissertation reuse.

**DoD:** `sepedi_final_test_results.json` exists, was produced by exactly one
`trainer.evaluate()` call on a held-out test set never touched during training, and the
numbers are written down somewhere durable (this roadmap doc, a results table, or
directly into a dissertation draft section).

**Risk:** Kaggle T4 session limits (typically ~9–12h/week free tier) — budget the 15-epoch
run doesn't get interrupted mid-training; `training.ipynb` already has checkpoint-resume
logic built in (`resume_checkpoint` block), so an interruption is recoverable, not fatal.

---

## PHASE 4 — Stratified WER Comparison (NLLB vs GPT-4o-mini)
**Track:** A | **Depends on:** Phase 0 (independent of 1–3, can run in parallel)
**Effort:** 1 session

You were mid-derivation on this last session (bucket-boundary math, ~4 chars/token
conversion from NLLB's 512-token limit). Picking that back up:

- [ ] Finish the char-length bucket boundaries: short / medium / long, derived from
  NLLB's 512-token ceiling × ~4 chars/token ≈ 2048 char practical ceiling — define your
  three buckets explicitly (e.g., short: <100 chars, medium: 100–500, long: 500+) and
  write down the reasoning, not just the numbers.
- [ ] Run NLLB translation on a stratified sample (proportional across buckets) of the
  same rows GPT-4o-mini already translated — reuse existing back-translation pairs where
  possible to save compute.
- [ ] Compute WER (`jiwer`, already in your dependency list) for both NLLB and
  GPT-4o-mini against a shared reference — decide what the reference is (likely the
  back-translated-then-compared-to-original approach you're already using elsewhere, or
  a small human-translated gold set if you have one).
- [ ] Produce one comparison table: WER by (model × bucket).
- [ ] Interpret: does GPT-4o-mini's advantage (if any) hold across all three length
  buckets, or does it degrade on long text where NLLB's dedicated MT architecture might
  do better? This is a genuinely interesting, self-contained result for the dissertation's
  translation-methodology section — worth doing properly rather than rushing.

**DoD:** one notebook or script producing the WER-by-bucket table, saved as a CSV/JSON,
plus 3–5 sentences of interpretation.

---

## PHASE 5 — isiZulu Pipeline (repeat the Sepedi process for the second language)
**Track:** A | **Depends on:** Phase 2 (reuses the fixed anonymization/cleaning logic)
**Effort:** 2–3 sessions (translation batch + fine-tuning)

### 5.1 Translation
- [ ] Restore/adapt the batch translation logic (per the Phase 0.4 decision) for isiZulu
  (`zul`) — same GPT-4o-mini forward + back-translation pattern used for Sepedi.
- [ ] Batch submit via OpenAI Batch API (JSON-per-row, not numbered lists — matches your
  existing principle), respecting the same timeout/retry pattern (`httpx.Timeout` +
  `max_retries=0`) that fixed the Windows SSL hangs for Sepedi.
- [ ] Parse results via `data_parser.py` (should work unmodified — it's schema-driven, not
  language-specific, but confirm column names match after the rename step).

### 5.2 Data quality (repeat Phase 1–2 for isiZulu)
- [ ] EDA pass specific to isiZulu (length distributions, missing values, translation
  spot-checks).
- [ ] Anonymization + mojibake + URL pattern checks on the isiZulu columns.

### 5.3 Fine-tuning
- [ ] Copy `fine_tuning_training.ipynb` → an isiZulu variant (or parameterize the
  existing one with a `LANGUAGE` variable at the top — cleaner long-term, worth the
  refactor now rather than maintaining two near-duplicate notebooks).
- [ ] Same train/val/test-safe split methodology, same 15-epoch + early-stopping config
  (or re-tune if isiZulu's data volume differs meaningfully from Sepedi's).
- [ ] Run `final_evaluation.ipynb` (parameterized) once, save `isizulu_final_test_results.json`.

**DoD:** `isizulu_final_test_results.json` exists via the same leak-free methodology as
Sepedi; you now have three comparable numbers (English baseline — flagged non-reportable
— Sepedi, isiZulu) for a cross-language comparison table in the dissertation.

**Risk:** isiZulu data volume/quality from GPT-4o-mini may differ from Sepedi — if the
model struggles more with isiZulu (a Nguni language, structurally quite different from
Sepedi's Sotho-Tswana family), don't be surprised by lower translation quality; this is
itself a valid dissertation finding, not a failure. Document it either way.

---

## PHASE 6 — Blockchain Depth & Query Richness (polish on an already-solid core)
**Track:** B | **Depends on:** Phase 0 | **Effort:** 1–2 sessions
**Can run fully in parallel with Phases 1–5** — different skillset, no shared dependency.

The B1–B6/C1–C3 core is done per your own tracker. This phase is about making the ledger
more *useful*, not building more foundation.

### 6.1 Filtered queries
- [ ] Add `QueryReportsByLanguage(language string)` — `GetStateByPartialCompositeKey("pred",
  []string{language})` (the composite key already supports this prefix filter, so this is
  a small addition mirroring `QueryVotes`'s existing pattern).
- [ ] Add `QueryReportsByStatus(status string)` — will need either a secondary index
  (composite key `status:{status}:{language}:{row_id}`, maintained alongside the primary
  record) or an in-chaincode filter over `QueryAllReports` results (simpler, fine at PoC
  scale, document the scaling tradeoff if you mention this in the dissertation).
- [ ] Add corresponding `FabricBridge` methods (`query_by_language`, `query_by_status`) and
  CLI wrappers.
- [ ] Unit tests for both, following the existing `mockstub_test.go` pattern.

### 6.2 Org metadata (makes the demo feel like real fact-checking orgs, not `Org1MSP`)
- [ ] Extend `RegisteredOrg` struct: add `OrgName string` and `OrgType string` (e.g.,
  "fact-checker", "media-monitor", "ngo") fields, passed as args to `RegisterOrg`.
- [ ] Update `RegisterOrg` signature, `misinformation_test.go`, `smoke-test.sh`, and
  `blockchain.py`'s `register_org()` to match.
- [ ] Update `DATA_MODEL.md` to reflect the schema change (keep the decision-record habit
  going — it's one of the stronger parts of your existing documentation).

### 6.3 Optional: 4th-org quorum-scaling demo
- [ ] Only if time allows after Phase 6.1–6.2 and the usability layer (Phase 7) — extend
  `onboard-org3.sh`'s pattern to a 4th org, demonstrating quorum scaling from 2/3 (2 of 3)
  to 3/4 (per your `quorumFor` formula: `ceil(2N/3)`). This is cheap given the existing
  script as a template but is genuinely optional — it doesn't change the argument, just
  adds a data point.

**DoD:** new chaincode functions pass unit tests (`go test ./...`), `smoke-test.sh`
extended to exercise at least one filtered query, `DATA_MODEL.md` updated.

---

## PHASE 7 — Usability Layer ("a system fact-checkers can use")
**Track:** C (new) | **Depends on:** Phase 6.2 (org metadata should exist before building
UI around orgs) | **Effort:** 2–3 sessions

This is the phase that closes the gap between "technically correct consortium ledger" and
your actual stated purpose. Pick the smallest thing that honestly satisfies the claim —
don't over-scope given everything else on this roadmap.

### 7.1 Minimum viable: proper CLI tool
- [ ] Build `apps/ai_service/v1-0-0/app/src/fabric_cli.py` using `argparse`, wrapping
  `FabricBridge` with real subcommands:
  - `fabric_cli.py register --org org1`
  - `fabric_cli.py submit --org org1 --row-id 42 --language nso --text-file row42.txt
    --label 1 --confidence 0.95 --model-version afroxlmr-large-nso-v1.0`
    (hashes the text file locally, never sends raw text — matches existing
    `Prediction.hash_content` behaviour)
  - `fabric_cli.py vote --org org2 --language nso --row-id 42 --verdict accept`
  - `fabric_cli.py finalize --org org1 --language nso --row-id 42`
  - `fabric_cli.py query --language nso --row-id 42`
  - `fabric_cli.py list-orgs`
- [ ] Sensible `--help` text throughout, and clear error messages when e.g. an org tries
  to vote twice (surface the chaincode error message cleanly, don't just dump a stack
  trace).
- [ ] A short `USAGE.md` walkthrough — written for a non-developer fact-checker, not a
  dev — showing the register → submit → vote → finalize → query flow with real example
  output.

### 7.2 Better if time allows: minimal web demo
- [ ] Thin FastAPI (or Flask) wrapper exposing the same six operations as HTTP endpoints,
  reusing `FabricBridge` — this is a small layer on top of 7.1, not a rebuild.
- [ ] A single-page HTML/JS frontend (no framework needed at this scale) — form for
  submit, a table for pending reports awaiting votes, a vote button per registered org.
  This becomes your dissertation's primary demo screenshot/recording.
- [ ] **Explicitly scope this down**: no auth, no production hardening, no styling
  polish — it exists to prove the interaction model works for a non-CLI user, not to be a
  shippable product. Say so directly in the dissertation so it reads as an intentional
  scoping decision, not an oversight.

**DoD:** a person who has never seen the `peer` CLI can, following only `USAGE.md` (or
using the web form if 7.2 is built), successfully submit a report, have it voted on by two
simulated orgs, and see it finalize — without touching Fabric internals directly.

---

## PHASE 8 — Full End-to-End Integration
**Track:** D | **Depends on:** Phase 3, 5, 7 | **Effort:** 1 session

Right now the AI pipeline and blockchain layer are each proven independently
(`fine_tuning_test.ipynb` produces model metrics; `smoke-test.sh`/`explanation`'s demo
uses hand-written example rows). They are not yet connected in one real run.

- [ ] In `final_evaluation.ipynb` (or a new `pipeline_to_ledger.ipynb`), after computing
  test-set predictions, take a sample of actual model outputs (row_id, predicted label,
  confidence — real numbers from your trained Sepedi/isiZulu models, not the demo rows in
  `explanation`) and call `submit_pipeline_output(rows, language=..., model_version=...,
  bridge=...)`.
- [ ] Register 2–3 simulated stakeholder orgs, have them vote on the real submitted
  predictions, finalize, and query the results back.
- [ ] Verify the content-hash round-trip: `Prediction.hash_content(original_text) ==
  bridge.query_report(...)['content_hash']` for a few sampled rows — this is your
  integrity proof and should be explicitly demonstrated, not just asserted.
- [ ] Save a transcript or screen recording of this full run — **this is your primary
  dissertation demo artifact**, and should show a real model prediction flowing all the
  way to an immutable, voted-on ledger record.

**DoD:** one artifact (notebook output, recorded transcript, or video) showing a real
trained-model prediction going from raw text → hash → PENDING report → votes → FINAL →
query, with the hash verified to match.

---

## PHASE 9A — System-Level Validation (Functional Robustness)
**Track:** D | **Depends on:** Phase 8 | **Effort:** 1 session

Before writing this up as "working," stress it a little — examiners will ask "what
happens if...".

- [ ] What happens if a 4th, unregistered org tries to vote? (Should fail cleanly —
  already covered by `TestUnregisteredCannotSubmit`-style logic, but confirm it extends
  to `CastVote` too, not just `SubmitReport`.)
- [ ] What happens if the network drops mid-invoke? (Document the failure mode, even if
  the answer is "the CLI wrapper surfaces the peer error and the transaction simply
  doesn't commit" — Fabric's own atomicity guarantees this, but say so explicitly.)
- [ ] What happens with a genuinely adversarial submission (nonsense confidence value,
  malformed hash)? Confirm `validateReportInput` rejects it — you already have tests for
  this (`TestInvalidInputsRejected`), just confirm coverage is complete against the
  usability-layer (Phase 7) entry points too, not only direct CLI invokes.
- [ ] Basic timing note: how long does register → submit → vote×2 → finalize → query take
  end-to-end on your machine? A rough number ("~8 seconds per full cycle") is useful
  context for the dissertation's discussion of practicality at scale.

**DoD:** a short `VALIDATION_NOTES.md` documenting each check and its outcome — doesn't
need to be exhaustive, needs to show you tested boundary conditions deliberately.

---

## PHASE 9B — Performance, Latency & Throughput Testing (Multi-VM Stakeholder Simulation)
**Track:** E (new — Performance/Infra) | **Depends on:** Phase 6 (blockchain depth),
Phase 8 (integration) | **Effort:** 2–3 sessions

This is the phase that turns "the consortium workflow works" into "here's evidence it
holds up when multiple real, physically-separate stakeholders hit it concurrently." It's
also one of the more dissertation-friendly phases — quantitative results, plots,
genuinely interesting failure modes to discuss.

### 9B.1 — Test environment setup (choose one, don't try to do both)

**Option A — remote clients, centralized network (recommended, lower effort):**
The Fabric network (orderer + all peer containers) stays on your main host machine, as it
does now. Each additional VM/device acts purely as a *stakeholder client* — it runs only
the `peer` CLI and `FabricBridge`, configured to reach the host over the LAN instead of
`localhost`.
- [ ] Expose the relevant Docker container ports on the host beyond `127.0.0.1` (bind to
  the host's LAN IP or `0.0.0.0` in the compose port mappings — check
  `test-network/compose/` files).
- [ ] Securely copy each org's MSP + TLS crypto material
  (`organizations/peerOrganizations/orgN.example.com/...`) to the corresponding client
  VM — this is sensitive material, keep it off any VM representing an org that shouldn't
  have it, to also implicitly test that org-scoped access boundaries hold.
- [ ] On each client VM, set `CORE_PEER_ADDRESS` / `CORE_PEER_TLS_ROOTCERT_FILE` in
  `FabricBridge._build_env()` (or via env vars) to point at the host's real LAN IP, not
  `localhost`.
- [ ] Open host firewall for the peer/orderer ports (7051, 9051, 11051, 7050 by default)
  to the LAN subnet only — don't expose to the open internet for a student project demo.
- [ ] Realistic mapping: 3–4 VMs/devices = 3–4 simulated fact-checking orgs, each only
  able to act as its own org (proves org-identity separation isn't just a config toggle,
  it's enforced by which crypto material physically exists on which machine).

**Option B — fully distributed peers (stretch goal, closer to real deployment):**
Each org's peer container runs on its own separate VM, communicating over the real
network rather than the Docker bridge network. Requires editing
`test-network/compose/*.yaml` networking, hostname resolution across machines (hosts
file entries or a shared DNS), and re-distributing certs per peer. Only attempt this if
9A/9B Option A are done comfortably early — it's a meaningfully bigger lift and the
marginal realism gain over Option A is smaller than it sounds for a PoC-scale demo.

### 9B.2 — Metrics to capture
- [ ] **Per-operation latency**, wall-clock from invoke call to commit confirmation
  (`--waitForEvent` already gives you a natural commit boundary): `RegisterOrg`,
  `SubmitReport`, `CastVote`, `FinalizeReport`, and each query type separately (queries
  should be much faster — no ordering/consensus round-trip).
- [ ] Latency **percentiles** (p50 / p95 / p99), not just mean — blockchain latency is
  rarely normally distributed, tails matter more than averages for a real deployment
  story.
- [ ] **Throughput (TPS)** under increasing concurrent submitters: 1, 2, 5, 10 concurrent
  clients issuing `SubmitReport`/`CastVote` simultaneously.
- [ ] **Simulated network latency/jitter** between VMs using `tc netem` (Linux) — add
  +50ms, +150ms artificial delay to represent geographically distributed stakeholders
  (a fact-checker on a slower connection vs. one on the same LAN as the network host) and
  re-measure the same metrics against the zero-added-latency baseline.

### 9B.3 — Load-testing harness
- [ ] Build `apps/ai_service/v1-0-0/app/src/load_test.py`: one `FabricBridge` instance
  per simulated org/VM, driven concurrently via `concurrent.futures.ThreadPoolExecutor`
  (the bridge is subprocess-based via `peer` CLI, so threads are sufficient — no need for
  `asyncio`).
- [ ] Log every operation's start/end timestamp, org, operation type, and success/failure
  to a CSV — this raw log is what the plots and tables in 9B.5 are built from.
- [ ] Parameterize concurrency level and target operation via CLI args so you can sweep
  concurrency 1→10 in a simple loop without rewriting the script each time.

### 9B.4 — Concurrency correctness / stress testing (the interesting part)
- [ ] Specifically test **multiple orgs calling `CastVote` on the same report at
  effectively the same instant**. Fabric uses optimistic concurrency control (MVCC) at
  commit time — and `CastVote` currently does a read-modify-write on the *shared*
  `ReportRecord` key (`record.Votes = append(...)` then `PutState` on the same
  `pred:{language}:{row_id}` key that every voting org touches). Under genuine concurrent
  writes to that key, expect `MVCC_READ_CONFLICT` failures on some transactions — this is
  expected Fabric behaviour, not a bug in your chaincode logic, but it's a real
  consideration for "multiple stakeholders voting concurrently."
- [ ] Quantify the conflict rate at each concurrency level (2, 3, 5+ orgs voting on the
  same report near-simultaneously, repeated N times).
- [ ] If conflicts occur at a rate worth addressing: implement a retry-with-exponential-
  backoff wrapper around `FabricBridge.invoke()` (or a new `invoke_with_retry(function,
  args, max_retries=3)` method) that catches the MVCC error and retries the read-modify-
  write cycle. Re-measure conflict rate and latency with retry enabled vs. disabled.
- [ ] This — "identified a real concurrency limitation in the naive design, measured it,
  and implemented + validated a fix" — is a strong, concrete engineering narrative for
  the dissertation's evaluation chapter. Don't skip documenting the *before* numbers even
  once you've fixed it; the delta is the interesting result.

### 9B.5 — Reporting
- [ ] One results table: operation type × concurrency level × (mean latency, p95
  latency, throughput, error/conflict rate).
- [ ] Plots: latency vs. concurrency, throughput vs. concurrency, conflict rate vs.
  concurrency (with/without retry logic) — simple `matplotlib`, doesn't need to be
  fancy.
- [ ] Explicit comparison: single-host baseline (everything on `localhost`, current
  setup) vs. multi-VM LAN setup vs. multi-VM + artificial network latency.
- [ ] Written interpretation (aim for half a page): what does this imply for a real
  multi-institution fact-checking consortium — is quorum voting latency acceptable for
  the expected report volume? Does concurrent voting on trending misinformation stories
  (many orgs voting on the same hot report at once) become a bottleneck? This directly
  strengthens the "practicality" argument the dissertation needs to make.

**DoD:** `load_test.py` exists and runs against at least a 2-VM (ideally 3–4 VM) setup;
one CSV of raw latency/throughput/conflict measurements covering both the single-host
baseline and the multi-VM scenario; plots + a written interpretation section; if MVCC
conflicts were found, a documented before/after comparison with the retry mitigation.

**Risk:** VM/device availability and network setup (port exposure, cert distribution,
firewall rules) is fiddly and easy to lose a session to without result — timebox the
environment setup (9B.1) to one session and fall back to Option A only if Option B stalls.

---

## PHASE 10 — Dissertation Alignment Pass
**Track:** D | **Depends on:** all of the above being at least drafted | **Effort:**
1–2 sessions, but genuinely important

- [ ] Write the explicit paragraph addressing proposal-vs-implementation scope: your
  proposal frames the blockchain layer as "deliberately simple, modular,
  platform-agnostic," but the implementation is full Hyperledger Fabric with Raft
  consensus and multi-org endorsement policies. State this directly, explain why (team
  decision, learning value, more realistic consortium demonstration), and don't let it
  look unaddressed — this is the single biggest "a marker will ask about this" risk
  identified across the whole review.
- [ ] Cross-language results table: English (baseline, flagged non-reportable due to the
  known leak — include this transparently, it demonstrates methodological maturity, not
  weakness), Sepedi (Phase 3), isiZulu (Phase 5).
- [ ] WER comparison table + interpretation (Phase 4) in the translation-methodology
  section.
- [ ] EDA figures (Phase 1) in the data/methodology chapter.
- [ ] Blockchain data model decision record (`DATA_MODEL.md`) largely drops in as-is for
  the implementation chapter — it's already written at dissertation quality.
- [ ] Usability-layer walkthrough (Phase 7) + end-to-end demo artifact (Phase 8) as the
  implementation chapter's central demonstration.
- [ ] Validation notes (Phase 9A) as a short "robustness" subsection.
- [ ] Performance/latency/throughput results and the concurrency-conflict finding
  (Phase 9B) as a dedicated "evaluation" or "performance" subsection — this is a strong,
  quantitative addition that most PoC-level blockchain dissertations skip, so lean into
  it rather than burying it as a footnote.
- [ ] Limitations section: REST layer deferred by design (document as scope decision, not
  gap), quorum formula behaviour at very small/large N, translation quality ceiling from
  Phase 4's WER findings, single-machine deployment (not distributed across real
  institutions).

---

## PHASE 11 — Final Submission Prep
**Track:** Housekeeping | **Depends on:** everything above | **Effort:** 1 session

- [ ] Fresh clone on a clean machine (or clean VM/container) — run through Phase 0's DoD
  checklist one more time now that everything else has been added.
- [ ] Confirm `README.md` (currently one line) actually describes the project, points to
  `updates`, `blockchain/explanation`, and the usability-layer `USAGE.md`.
- [ ] Confirm all `(...)` placeholder paths in `fine_tuning_training.ipynb` /
  `fine_tuning_test.ipynb` (`SEPEDI_CSV_PATH`, `model_path`) are either filled in with
  real final paths or clearly marked as "edit before running" with a comment, consistent
  with how `fine_tuning_test.ipynb` already does this well (`# EDIT: replace with your
  actual uploaded Sepedi model path`).
- [ ] Tag a release/commit that represents the submitted state, so you have a clean
  reference point independent of any post-submission tinkering.

---

## Suggested Execution Order & Parallelization

Two people (you + teammates), roughly independent tracks:

| Order | Phase | Track | Can parallelize with |
|---|---|---|---|
| 1 | 0 — Repo hygiene | Housekeeping | — (do first, blocks everything) |
| 2 | 1 — EDA | A | Phase 6 (blockchain) |
| 2 | 6 — Blockchain depth | B | Phase 1 (EDA) |
| 3 | 2 — Data quality fixes | A | Phase 6 continued |
| 3 | 4 — WER comparison | A | Phase 2, 6 |
| 4 | 3 — Sepedi re-run | A | Phase 7 (usability layer) |
| 4 | 7 — Usability layer | C | Phase 3 (GPU wait time is dead time otherwise) |
| 5 | 5 — isiZulu pipeline | A | — |
| 6 | 8 — Full integration | D | — (needs 3, 5, 7 done) |
| 7 | 9A — Functional validation | D | Phase 9B environment setup can start in parallel |
| 7 | 9B — Multi-VM latency/throughput/stress testing | E | Phase 9A |
| 8 | 10 — Dissertation alignment | D | can start drafting earlier, finalize last |
| 9 | 11 — Submission prep | Housekeeping | — |

**Key insight for parallelization:** Phase 3 (Sepedi re-run) has significant GPU-idle wait
time during the 15-epoch training run. That's the ideal window to build Phase 7's CLI
tool — no shared dependency, and it turns dead time into progress. Similarly, Phase 9B's
VM/network setup (9B.1) doesn't depend on Phase 9A's outcome — you can start sourcing and
provisioning devices/VMs while functional validation is still being written up.

---

## Risk Register (things likely to bite you)

| Risk | Phase | Mitigation |
|---|---|---|
| Kaggle T4 weekly quota runs out mid-training | 3, 5 | Checkpoint-resume already implemented in `training.ipynb` — use it deliberately, don't fight it |
| isiZulu translation quality much lower than Sepedi (different language family) | 5 | Expect and document this as a finding, not a bug — budget extra EDA time to characterize it |
| Proposal-vs-implementation scope question from examiners | 6, 10 | Address head-on in Phase 10, don't leave implicit |
| Vendor tree / large binary accidentally in git history even after `rm --cached` | 0 | If already pushed, history rewrite (`git filter-repo`) may be needed — check repo size on GitHub before assuming a simple removal is enough |
| Usability layer scope creep (building a "real product" instead of a demo) | 7 | Explicitly timebox to 2–3 sessions; the dissertation needs *evidence of usability*, not a shippable app |
| Time pressure causes Phase 9A (validation) or Phase 10 (write-up alignment) to get skipped | 9A, 10 | These are cheap relative to their payoff in examiner confidence — protect this time even if Phase 5 (isiZulu) runs over |
| VM/device network setup (port exposure, cert distribution, firewall) eats a whole session with no measurable result | 9B | Timebox environment setup to one session; fall back to Option A (centralized network, remote clients) if Option B (fully distributed peers) stalls |
| Concurrent `CastVote` on the same report produces `MVCC_READ_CONFLICT` failures under load | 9B | Expected Fabric behaviour, not a chaincode bug — treat as a finding to measure and (optionally) mitigate with retry-with-backoff, not an emergency |
| Artificial network latency (`tc netem`) requires root/admin on each VM and platform-specific setup (Linux-only tooling) | 9B | Confirm VM OS choice supports `netem` before committing test design around it; Windows/macOS VMs need a different approach or a Linux VM layer |
