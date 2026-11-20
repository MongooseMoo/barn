# PR 46 admission review checkpoint

- Task: independently review moo-conformance-tests PR #46 at exact head `34b0d30f895a7bb9c67d51068d174603491fdca0` for issue #43.
- Scope: current code and live GitHub only; no historical project Markdown; read-only review except this required untracked checkpoint note.
- Review focus: fail-closed staged admission, canonical probe inventory/order/status, blocked semantics, execution gating, phase-aware expected inputs, exact set validation, JUnit/inventory completeness, workflow quoting, unconditional Toast profiles and exact union, schema/provenance, direct CLI and filtered-run regressions.
- Current findings:
  - Exact head and live PR CI are green; run `31004774857` exercised admission plus all packaged cases on all three Toast profiles and enforced the exact union.
  - Canonical admission JSON has the required ordered four-probe inventory and strict per-status fields; controller unit tests reject malformed, duplicate, unknown, phase-incompatible, missing, extra, and inexact surfaces.
  - Major fail-closed gap: `-m conformance` and `-k 'not capability_admission'` each collect all 11,587 packaged cases while deselecting the sole admission item. No plugin hook requires prior admission evidence for those filtered runs. The paired workflow intentionally uses this bypass after a separate admission process, but there is no evidence token/input binding the packaged invocation to that successful admission. Direct filtered CLI/pytest runs can therefore execute packaged YAML without admission, contrary to issue #43's universal execution gate and regressing the former autouse behavior.
  - The reusable Toast workflow and profile union remain unconditional. Live artifact evidence: each profile admission JUnit has one aggregate test and admission JSON has four ordered probes; packaged counts are 11,516/71, 11,501/86, and 11,497/90 pass/skip; union 11,546 of 11,587 with 41 reviewed skips.
  - Minor cleanup defect: paired workflow's oracle profile literal repeats the `implementation` key; same value, so no runtime semantic change.
- Verification: 65 focused tests passed; all 305 harness tests passed; unfiltered collection 11,588 with admission first; marker and name filtering each proved admission deselection with 11,587 packaged cases retained.
- Blocker: exact head is NO-MERGE until packaged-only execution requires and validates trusted successful admission evidence, with regression tests for marker/name-filtered direct runs and the staged workflow binding.
- Next action: report the exact-head NO-MERGE finding and evidence to the parent agent. Do not modify PR code.

## Corrected head re-review

- Exact head: `a8756eef330ba686f898db3eb7ddd283492c7c3a`.
- Prior bypass is materially repaired: after final marker/keyword selection, collection rejects packaged items unless a selected admission item remains or both a schema-v2 evidence file and exact expected context are supplied. External evidence is fully parsed, context-matched, and rejected if any probe is failed, error, blocked, skipped, malformed, missing, or unknown before fixtures execute.
- Managed Toast and paired workflows derive context from immutable candidate/oracle/profile coordinates, pass the same context to the admission producer and packaged consumer, and the trusted verdict controller revalidates it. The duplicate workflow dictionary key was removed.
- Focused corrected-head verification: 76 admission/controller/workflow tests passed.
- Remaining review question: same-session gating currently accepts any selected test carrying the `admission` marker, rather than proving the selected item is the one canonical `test_capability_admission` identity. Determine whether that is an exploitable architectural bypass or merely a plugin-extension concern before final verdict.
- Live CI `31006666628` is fully green at the exact head, including all three Toast profiles and exact union.
- Next action: run full tests and direct adversarial collection, inspect same-session canonicality and artifact workflow evidence, then issue exact-head MERGE/NO-MERGE.

## Adversarial trust re-review

- Exact head: `691db2a93f6337c981a9b857dda7175e527ada34`.
- Runtime admission repair succeeds: exact canonical item identity is required; fake marker and post-collection removal regressions pass; authorization becomes valid only after setup, call, and teardown pass; packaged setup performs a final try-first authorization check before fixtures.
- Controller trust improvements are present: paired commands execute `github.workflow_sha` controller code, candidate checkout supplies suite/DB paths, trusted inventory is a required subset of a recomputed candidate inventory, admission JSON/JUnit/context/exit state reconcile, schema-v4 result records inventory identity digest and admission JUnit state, checkout credentials are disabled, and Toast remains a three-profile unconditional gate.
- Merge blocker: candidate suite and database roots are not anchored to the candidate checkout after symlink resolution. `--moo-suite-root` is simply `Path(...).resolve()` and accepted; `validate_candidate_inventory()` likewise resolves the supplied root without verifying it remains below candidate-data. A candidate can delete every YAML and commit `_tests` as a relative symlink to `../../../controller/src/moo_conformance/_tests`; trusted inventory and packaged execution then traverse the trusted controller suite and report no deletion.
- Exact local exploit: a declared candidate `_tests` junction containing no candidate YAML but targeting the trusted controller `_tests` was accepted by `validate_candidate_inventory()`, producing 11,587 trusted IDs, 11,587 candidate IDs, and zero additions. Python resolved the candidate path directly to the trusted controller tree. GitHub Linux checkouts preserve repository symlinks, so the equivalent relative Git symlink is viable.
- The same root-escape class applies to candidate `Test.db` and an entire `_db/startup` symlink: workflow paths and the runner's confinement are relative to the already-resolved candidate-controlled root, not an independently trusted candidate checkout boundary.
- Verification: 10 hostile admission tests passed; 79 controller/inventory/workflow/server tests passed; all 322 harness tests passed; Ruff and mypy passed; CI `31010545689` is fully green but has no root-symlink adversarial regression.
- Blocker: NO-MERGE until controller/workflow rejects suite root, DB root, primary DB, and nested link targets escaping the immutable candidate checkout, with the deletion-via-root-symlink exploit covered.
- Next action: report the exact exploit and NO-MERGE verdict to the parent agent; do not modify PR code.
## Exact-head re-review: 055b993e206bedf887780e64d84dfa7fcadb2f75

- State: PR #46 exact head is clean locally; live CI run 31012613429 reports all five jobs green.
- Current observation: the confinement repair changes 13 files and adds `path_confinement.py`; no code verdict yet because the new implementation and hostile-path tests still require direct inspection and reproduction.
- Prior blocker under review: candidate `_tests` and database roots could be symlinked/junctioned into the trusted controller checkout, making external evidence appear in-root and defeating inventory/deletion trust.
- Next action: inspect root anchoring and per-target resolution, reproduce suite-root/primary-DB/database-root/nested-link attacks (including Windows junctions), then run focused and full verification.

### Verification checkpoint

- Prior bypasses are closed when the independently supplied checkout root is the fixed real checkout directory: `_tests` junction to the controller and whole DB fixture-root junction both raise escape errors.
- Windows hostile-path focused surface passed: 67 tests across execution ledger, CLI selection, and managed server.
- Full local gates passed: 330 harness tests; 11,588 collected package items (one admission plus 11,587 packaged); Ruff; mypy; `git diff --check`.
- Root-boundary observation: `resolve_candidate_anchor` intentionally resolves and accepts a supplied root junction. This is not candidate-controlled in the paired workflow because the trusted workflow fixes `${GITHUB_WORKSPACE}/candidate-data` and `actions/checkout` owns that directory entry; assess whether any caller permits attacker control before treating it as an exploit.
- Next action: verify exact workflow/runtime/provenance binding and determine whether any in-scope remaining exploit exists before issuing the exact-head verdict.
## Final exact-head re-review: 1e2c0994ee78b0faf5537e9ea57872350e85f4f7

- State: local and live PR head match exactly; PR is clean/mergeable and CI run 31015132189 is fully green across quality, all three Toast profiles, and the exact-union ledger.
- Topology observation: conformance candidate and pinned Toast oracle are credentialless sibling checkouts (`candidate-data`, `toast-oracle`); candidate anchor is exactly `${GITHUB_WORKSPACE}/candidate-data`.
- Generated outputs inspected so far: uv environments, Toast build, reports/JUnit/admission evidence, downloaded artifacts, and final ledger are under `${RUNNER_TEMP}`; commands use explicit project/working-directory paths.
- Prior identical-byte primary DB sibling-link attack is covered and rejected, as are generated sibling suite/DB links.
- Intermediate graph failure came from job-level runner context usage; final head initializes the absolute uv environment paths through runtime `$GITHUB_ENV` steps.
- Next action: run focused/full local gates, recheck live exact head, and finish the canonical-runtime/controller/inventory/nonce/confinement regression audit.
