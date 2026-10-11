# Toast federation interoperability plan handoff

## Current findings

- Primary architecture is `plans/moo-federation.md`; the requested artifact is a
  separate Toast interoperability execution plan, not a replacement protocol.
- Barn issue #170 is open and actively implemented by PR #197,
  `agent/issue-170-waif-swiss-identity`; exact PR head inspected:
  `c70741bdf7352895baf32fd8f87f1a939d1b9209`.
- Current #170 contract is 128-bit `crypto/rand` WAIF identity plus a
  database-hash-bound `.waifids` sidecar, keeping the Toast-compatible `c N` /
  `r N` body unchanged. Wire encoding is deliberately excluded from #170.
- Toast source inspected at WSL commit
  `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
- Toast has useful native seams: descriptor registration in `src/network.cc`,
  single-task main loop in `src/server.cc` / `src/tasks.cc`, normal serializable
  suspended tasks, stateful built-in read/write callbacks, explicit-programmer
  program execution, OpenSSL/TLS, and concentrated WAIF allocation/checkpoint
  code in `src/waif.cc`.
- Toast traps: player connection handles and `open_network_connection()` are the
  wrong transport; generic external queues are interrupted at checkpoint;
  socket callbacks must not run MOO/database work; stock `run_server_task()`
  installs verb-owner permissions; current `Waif *` identity is process-local.

## Current state

- Added `plans/toast-federation-interop.md` as the requested secondary plan.
- Removed three incorrect scratch artifacts created before the correction:
  `docs/reports/toast-federation-interop-source-survey.md`,
  `reports/spec-discovery-toast-federation-interop.md`, and
  `prompts/spec-draft-toast-federation-interop.md`.
- The repository was already heavily dirty/untracked; no pre-existing user file
  has been edited.
- This note must remain uncommitted and unstaged.

## Blockers

- None for finishing the planning document.
- Implementation must take the final merged #170 sidecar/identity contract, not
  assume the current open PR is immutable.
- The spec-updating protocol's `ward` role helper is not installed in this shell,
  so its ward-bound review dispatch cannot be used. Use local verification as
  fallback unless the environment changes.

## Next action

- Planning work is complete. If implementation is authorized later, begin with
  Phase 0 in `plans/toast-federation-interop.md`: freeze the interoperable wire
  profile and shared vectors, while consuming the final merged #170 contract.
- Do not stage, commit, push, patch Toast, or touch PR #197 without a new user
  request.

## Verification completed

- Confirmed the plan contains 21 unique headings and one valid relative link.
- Confirmed `git diff --no-index --check` reports no whitespace errors.
- Confirmed the three incorrect scratch artifacts no longer exist.
- Confirmed the only task-specific untracked files are the requested plan and
  this handoff note; neither is staged.
- Confirmed the plan explicitly calls #170 active work and refers to PR #197,
  the `.waifids` contract, Toast task persistence, `E_REMOTE`, and every phased
  interoperability gate.

## What worked

- Reading the exact PR #197 head after fetching `refs/pull/197/head` established
  the current implementation contract without guessing from #157.
- Reading Toast directly in WSL established the native networking, suspension,
  checkpoint, built-in continuation, execution-permissions, TLS, and WAIF seams.
- Keeping the requested output as one companion plan avoided modifying the
  existing untracked federation proposal and research reports.

## What did not work

- The initial pass confused completed #157 with active #170 and produced three
  incorrect scratch files. Those files were deleted before the final plan was
  written.
- The `ward` role-binding executable required by the selected protocol skill is
  not installed in this PowerShell environment. The plan therefore received a
  local structural/source verification pass rather than ward-bound review.

## Current blocker

- None for the planning request. Implementation remains intentionally
  unauthorized in this task.

## Feasibility conclusion

- The Toast federation interoperability work is doable. Source inspection found
  no architectural blocker.
- Toast already supplies the required foundations: a single-threaded main loop,
  native descriptor registration, TLS/OpenSSL, serializable suspended VMs,
  checkpointable built-in continuation state, an explicit-programmer execution
  seam, and concentrated WAIF allocation/checkpoint code.
- The hard work is engineering rather than feasibility: freeze exact wire bytes,
  preserve principal/facet authority, make redelivery and restart idempotent,
  port the final merged #170 identity contract to Toast, and prove every mixed
  engine direction in the paired harness.
- Next action remains Phase 0 only after new implementation authorization:
  specify the Grange v0 wire profile and shared vectors before server code.
