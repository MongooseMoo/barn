# Current Barn/conformance campaign inventory checkpoint

## Scope and method

- Read-only inventory requested across `MongooseMoo/barn` and `MongooseMoo/moo-conformance-tests`.
- Used live GitHub issue/PR metadata, PR files/commits/checks, and failed Actions logs only.
- Did not read repository Markdown, plans, reports, historical notes, fixture contents, secrets, or sensitive data.
- Existing Barn checkout is heavily dirty; no existing files were changed and nothing was staged, committed, or pushed.

## Current repository heads

- Barn default `master`: `aa82abf05a6a23783ba69105d0c393f926304a13`.
- Conformance default `main`: `1e61c1dedb515387874fac5f961a7df54bca4786`.

## Open PR state

- Barn: #108 closes #107, head `34d2ff589bf6b37d0c77b1c7829abc1fa457e04d`, base PR #106 head `08a2de3498c8b888e0d0244cc12700bce3bd0830`; draft, build green, conformance red.
- Barn: #106 closes #72, head `08a2de3498c8b888e0d0244cc12700bce3bd0830`; draft, build green, conformance red.
- Barn: #105 closes #102, head `c8861f611eceabfaffa6d574573ac38f92b9265e`; draft, build green, conformance red.
- Barn: #104 closes #103, head `771cbb39f06b698d89c897235031a35910c48995`; draft, build green, conformance red.
- Barn: #101 closes #100, head `df0f8ac8b77ae9ab8458d78341f71218766e13c8`; draft, build green, conformance red.
- Barn: #99 closes #91, head `2719014d16e777c97979f71a6e68638d2469c288`; non-draft, build green, conformance red.
- Barn: #98 closes #97, head `9d22061dac4bd6adcb4ba11d6c8e8fc410c38e68`; non-draft, build green, conformance red.
- Barn: #96 closes #95, head `0450cf22c84d0c3ebdb96a9172b6072db1a7f56a`; draft, build green, conformance red.
- Barn: #94 closes #93, head `d3096fc967c3fa6415c6eec05ed78f1ed53a88b0`; draft, build green, conformance red.
- Barn: #81 closes #4, head `e2fc3653322a8188506208d8a08055ad5267906e`; non-draft, build green, conformance red.
- Conformance: #45 closes #44, head `d03abea108555cc7f01cabbec69011e2c3e3b221`; draft, behind main, all five checks red.
- Conformance: #22 closes #12, head `be763f95c7b4cea60bc5f69d1383f6f76c6d3a26`; draft, behind main, all five checks green.

All Barn PRs except #108 target default `master` at the SHA above. #108 is explicitly stacked on #106. All have literal `Closes #N` phrases and are mergeable at the GitHub graph level; CI state is `UNSTABLE` because conformance is red.

## Current blockers and overlap

- Barn PRs #94, #96, #99, #101, #104, #105, #106, and #108 all fail at the suite-wide `OUTBOUND_NETWORK` capability probe because their bases do not contain #98/#97; representative fan-out is 11,545 or 11,566 errors.
- Barn #98 clears that probe but then fails suite-wide at `ephemeral_listen` with 11,553 errors, directly blocked by #101/#100.
- Barn #81 predates the capability-probe fan-out and has one unrelated conformance failure: `index_call_shapes::index_third_arg_str`; it does not indicate a reseed_random regression.
- Conformance #43 exists to replace this autouse capability fan-out with exact first-class admission evidence. It overlaps the diagnosis surface but does not repair Barn #97 or #100.
- Barn #108 is genuinely dependent on #106 and should remain stacked until #106 lands.
- Conformance #22 is already implementation-complete for issue #12 by diff/closure/check evidence but remains draft and behind main.
- Conformance #45 implements #44 but is currently not deliverable: draft, behind main, and every check fails; failed-log diagnosis remains next.

## Next action

- Inspect current #45 failed logs and compact open-issue bodies/source references to cluster all remaining issues by common cause.
- Confirm whether current default-branch source already addresses any still-open issue, without reading Markdown or fixture content.
- Produce a concise complete inventory and recommended order to the parent agent.

## Follow-up audit: conformance issue 43 at origin/main

- Exact audited commit: `09056a9daea918b0b851d81f66c7025be38e7985` via immutable `git show origin/main:<path>` blobs because the primary checkout is dirty and no worktree is pinned to that exact SHA.
- The live open issue #43 acceptance surface is already materially implemented at this commit.
- `src/moo_conformance/admission.py` owns a finite four-identity ordered inventory, explicit prerequisites, passed/failed/error/blocked outcomes, schema-version 2 evidence, strict exact-key/status/inventory/dependency validation, context binding, and stable JSON IO.
- `src/moo_conformance/test_conformance.py` exposes one canonical `@pytest.mark.admission` testcase; it records context, executes the complete inventory, writes evidence, and fails once with exact failed/error and blocked identities.
- `src/moo_conformance/plugin.py` distinguishes the exact canonical admission item from packaged items, runs admission first, rejects packaged-only runs without validated successful context-bound evidence, and prevents fake/removed/failed/skipped admission from authorizing packaged runtime. The old capability probe autouse fan-out is absent; the remaining autouse fixture is only the profile metadata gate.
- `src/moo_conformance/paired_result.py` validates exactly one admission or packaged phase, exact declared/observed bad identity sets, complete packaged JUnit identity surfaces, admission JSON/JUnit/context agreement, expected-success packaged-only semantics, and schema-version 5 result output.
- `.github/workflows/paired-candidates.yml` validates immutable SHAs, explicit phase and exact identity arrays; runs Barn admission first; gates packaged execution on admission exit zero; and has a trusted default-branch verdict produce schema-v5 provenance.
- `.github/workflows/toast-conformance.yml` runs three fixed Toast profiles, each with canonical admission before packaged cases, then requires all matrix jobs and an exact three-profile packaged union.
- Tests explicitly cover admission success/failure/error/blocking, malformed/inexact/context-mismatched evidence, authorization bypass attempts, packaged exact-set preservation, phase mismatch, unknown/duplicate/malformed declarations, expected-success rules, staged workflow gating, three-profile Toast admission, exact union, and provenance.
- Remaining audit question: run or inspect exact current CI/test evidence at `09056a9d` and determine whether the issue is simply stale-open versus any criterion not exercised at runtime.
