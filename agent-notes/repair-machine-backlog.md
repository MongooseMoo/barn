# Barn repair backlog and self-healing

## Goal

Explain why open reports are accumulating without repair PRs, fix verified
automation failures and make recoverable interruptions self-healing.

## State

- GitHub returned 90 open issues labeled `claude-fix`. Of those, 23 also had
  `repair-needs-attention`, 24 had an agent or maintainer comment without that
  label, and 43 had neither. The `codex-repair`, `codex-running` and
  `codex-conformance` queries each returned zero.
- The WSL worker process was live. On #1124, run 45017989465 returned `success`
  after `Route the repair`; the worker recorded `repair #1124: policy-block`.
  #1124 carries `repair-needs-attention` and has no repair PR. Automatic provider
  selection uses even issue numbers for Codex when enabled. Policy blocks are
  terminal. PRs #1114 and #1116 were created and merged on November 22 UTC.
- Claude repair #1081 ended `No open pull request from claude/issue-1081;
  nothing to queue.` The issue comment identifies it as a duplicate of #1059,
  which merged. #1099 points to still-open #981 and #1033. The older failure for
  #1027 logged `Claude execution failed: result is_error:true`. The provider
  reason is unknown.
- In cycle 45033596493, two lanes logged `retained corpus and provenance exceed
  256 MiB; preserve artifacts before resizing`. Acceptance logged `Artifact
  not found for name: fuzz-failure-ToastReadsBarnDatabase`. The next cycle,
  45050112430, had four jobs in progress on the four dedicated fuzz runners.
- Both size failures are at `scripts/fuzz_retention.py:170`, after capture
  has written input files and before it commits the manifest or emits the
  execution census. `ToastDifferential` logged `The operation was canceled`
  at its configured 45-minute job limit. The colocated world comparison and
  upload at `.github/workflows/fuzz.yml:409–431` were skipped. In all six
  completed cycles checked, from 44963046975 through 45033596493, the API
  reports `cancelled` for `ToastDifferential`, `failure` for acceptance and
  `success` for successor.
- The local workflow now runs the world comparison independently and gives
  each comparison time to upload its own evidence. Acceptance requires both
  results and applies the existing result checks. The differential stops
  beginning new batches after 40 minutes and retries once from completed
  batches, with the original transcripts preserved.
- Local retention writes census and logs before capture. It archives a full
  oversized generation, keeps a bounded replay cache, and only saves that
  cache after its archive uploads. Later jobs read the full generation for
  that cycle. The reporter requests a repair run receipt and retries
  rate-limit rejections with GitHub's backoff. Unknown responses and provider
  policy blocks remain visible for review.
- Focused local runs returned `Ran 36 tests ... OK` for the reporter,
  `Ran 16 tests ... OK` for retained evidence, `Ran 39 tests ... OK` for the
  differential, and `Ran 62 tests ... OK` for acceptance, dashboard and
  provider tests together. A fixture executed the actual workflow shell
  step and printed `deadline=124, resumed=true, final=1, results=true, first
  transcript preserved`. Actionlint returned zero for the fuzz workflow.

## Decisions and why

- Preserve provider policy holds. For availability incidents, retain the work
  on a durable queue and make the actual outcome visible to the operator.
- Preserve failed evidence and continue applying the existing acceptance
  criteria. Investigate artifact production and retention at their owners.

## Surprises

- The GitHub API returned `display_title: Repair` for run 45017989465 even
  though the workflow declares a numbered run title. The dashboard parser
  requires a numbered title and cannot associate that run with #1124. Both
  repair and alarm run names were unquoted, so the YAML parser treated the
  hash and everything after it as a comment. Both are now quoted locally.
- `scripts/fuzz_report.py:285` comments on an existing signature and skips
  dispatch. At lines 301–308, issue creation and explicit workflow dispatch
  are separate API calls. A failed dispatch after creation has no retry through
  that path. This gap is verified in source. Its incidence is unverified.

## Next step

Q authorized the pull request. After merge, verify the first live cycle's
artifact paths, any timed retry evidence, and its acceptance verdict. Verify
the first new repair run has a numbered title and that the live backlog joins
it to its issue.
