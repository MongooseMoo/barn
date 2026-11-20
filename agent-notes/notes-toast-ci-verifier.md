# Toast CI verifier checkpoint

- Scope: read-only final merge gate for `MongooseMoo/moo-conformance-tests` PR #21.
- Current finding: the configured gauntlet path `C:\Users\Q\.Codex\AGENTS.md.d\gauntlet.md` is absent; the installed `protocols:gauntlet` skill was read successfully.
- State: no repository, GitHub, PR, or merge mutations performed.
- Blocker: none yet; live/current verification has not started.
- Next action: locate any applicable gauntlet role instructions, then verify PR scope, commits, issue acceptance, CI, branch protection, and mergeability without mutation.

## Current issues 16-18 audit

- Scope: current source and live GitHub only; historical Markdown, docs, plans, reports, notes, and git history excluded.
- Issue #16 remains open: packaged `src/moo_conformance/test_conformance.py` and ordinary `tests/test_conformance.py` still duplicate execution semantics and differ on feature, builtin, option, config, profile, and managed-restart handling.
- Issue #17 remains open: the sole required `Full suite` workflow covers packaged Toast execution, strict skips, fixture hashes, exact totals, and artifacts, but does not run harness units, duplicate lint, Ruff, or mypy.
- Issue #18 remains open: Barn CI hard-codes conformance `main`, does not accept or record explicit paired refs, uses mutable action tags, has no explicit least-privilege permissions while repository defaults are write, and Barn `master` is unprotected.
- Current live state: conformance main `a031527261632d262e1432344e0a5113119635de` has a green required Full suite run; Barn master `aa82abf05a6a23783ba69105d0c393f926304a13` last ran CI on 2026-07-30 and has not been revalidated by Barn CI against current conformance main.
- Blocker: none for the read-only audit.
- Next action: finish command/symbol ownership mapping and dependency order, then report whether #17 requires a focused PR.

## Issue 12 Toast workflow audit checkpoint

- Scope: read-only audit of the active issue-12 worktree and live GitHub workflow; no source, workflow, PR, or GitHub mutations.
- Current findings: active worktree `agent/issue-12-enforce-expectations` is clean at `9d27313f45ba4c62cf9fcbed87281d7aee5975f2`. The workflow invokes packaged pytest tests against pinned Toast without `--moo-suite-path`, so the packaged YAML tree is recursively collected with no selector omission.
- Execution gap: the live required `Full suite` run collected 11,569 cases but executed only 11,431; 138 were skipped. The workflow deliberately accepts `skipped: 138`, and `--fail-on-unexpected-skip` permits declared skips, so it does not prove every conformance case ran.
- Collection gap: the workflow asserts aggregate JUnit totals, not the collected test identities, so a remove-one/add-one swap can preserve the count.
- State: live PR #22 head matches the audited worktree SHA; the current `Full suite` check is green. No edits were made to the issue-12 worktree.
- Blocker: literal all-tests-executed compliance is blocked by the accepted skip population and by profile-dependent cases that cannot all apply to one Toast invocation.
- Next action: issue a strict NO-MERGE verdict and specify the minimal fail-closed workflow change (`skipped: 0`) plus collection-ID verification and live CI commands.
