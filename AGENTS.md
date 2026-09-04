# Barn Agent Notes

## Retry Reservation Publication Invariant

- Retry-reservation admission and commit publication must share synchronization through the complete validation/publication interval. A reservation check that unlocks before publication permits a later reservation to overtake an admitted writer; cover that ordering with a deterministic regression before measuring a reservation policy.

## Conformance Discipline Rules

- Keep Barn GitHub Actions runner selection behind the `CI_RUNNER_LABELS` repository variable with `ubuntu-latest` as the workflow fallback; while hosted minutes are exhausted, set the variable to `["self-hosted","Linux","X64","barn"]` before starting CI.
- Before reading a skill entrypoint, resolve its advertised skill root with `rg --files <root> --glob SKILL.md`; never construct an assumed path by inserting catalog labels or runtime directory names.
- In PowerShell, run a native command and capture `$LASTEXITCODE` in separate statements before using that result in an expression; never embed both statements inside a cast, subexpression, or parenthesized expression.
- For a multi-stage command passed through `wsl ... bash -lc`, begin the shell body with `set -euo pipefail`; never report the outer exit code as proof that an earlier probe succeeded when a later command or pipeline can mask its failure.
- Under `set -e`, when a text match may legitimately be absent, place `grep` in an explicit `if` branch that prints distinct match/no-match evidence; never use a bare expected-absence `grep` as the probe.
- Under `set -o pipefail`, do not truncate a command with `head` when the producer can receive SIGPIPE and turn a successful probe into exit 141; capture the output or use a non-pipeline probe.
- When a Bash body passed from PowerShell contains `$name` or `$()` expansion, invoke it as `wsl -d <verified-distribution> --exec bash -lc '<body>'` and single-quote the complete Bash body at the PowerShell layer; omitting `--exec` can add an outer WSL shell that consumes Bash variables, and backslash does not escape PowerShell interpolation.
- When passing a Windows path directly as an argument to `wsl --exec`, double every backslash in the PowerShell single-quoted literal (for example, `'C:\\Users\\Q\\code\\barn'`) and require a successful `wslpath` probe before using the translated path.
- Never reuse one `uv` project environment across Windows and WSL; when the same checkout is invoked from both platforms, set a platform-specific `UV_PROJECT_ENVIRONMENT` outside the checkout before the second platform runs.
- For issue-delivery ledger transitions into `WORKER_RUNNING`, supply both the canonical live `--agent` handle and the exact `--worktree` path; if the transition is rejected, interrupt the dispatched agent before retrying.
- Before invoking `gofmt`, verify every argument is a Go source file and pass only literal `.go` paths; never include Markdown or other changed files in the formatter command.
- In PowerShell, never pipe directly from the closing brace of a `foreach` statement; assign the loop output to a variable (or wrap the statement in `@(...)`) and pipe that value afterward.
- Before submitting a JavaScript tool wrapper, balance every template interpolation and function-call delimiter; never place a Bash body containing `${...}` in a JavaScript template literal—use a normal quoted JavaScript string or escape the interpolation marker; for exit reporting use the delimiter-light `text(r.exit_code ?? "RUNNING")` form instead of an interpolated template. A wrapper parse failure proves that its nested shell command did not run and must be corrected before continuing.
- Before any `apply_patch`, verify each context anchor against the live file; for a multi-file patch, when one anchor differs, split the edit into per-file patches using the verified anchors so one stale context cannot reject the entire repair.
- Before staging an investigation record, audit state-dependent sentences such as "no implementation files changed" against the final diff and update any statement made stale by later work.
- After adding an untracked file, inspect that file directly (or with an explicit no-index diff); ordinary `git diff` does not display untracked content and cannot satisfy the immediate post-edit inspection requirement.
- Before any explicit file read or search, resolve the target from an existing directory or `rg --files`; pass only verified existing directories or literal files as path arguments, express filename selection with `--glob`, and never put a wildcard in an `rg` path argument on Windows.
- In PowerShell, never type `*` in an `rg` path position: use the literal-directory form `rg PATTERN server --glob 'connection*.go'`, never `rg PATTERN server\\connection*.go`.
- In an `rg` command, use at most one `--` path separator, place every option such as `--glob` before it, and pass only verified literal paths after it; once `--` has appeared, never add a second `--` before a path variable.
- Before a multi-path `rg` invocation, run `Test-Path -LiteralPath` for every candidate, assign only existing targets to a filtered variable such as `$rgTargets`, and pass that variable after `--`; never hand-type a literal path list into the final `rg` invocation. Remembered filenames are not evidence that a path exists.
- Before building a Barn executable, resolve and verify the main package with `go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn`; never assume the repository root is a buildable command.
- Before a managed WSL oracle command names a distribution with `-d`, verify that exact name in `wsl --list --quiet`; use the installed canonical oracle distribution and never assume `Ubuntu` exists.
- For conformance work, use only the documented managed conformance command unless the user explicitly approves a different path.
- Do not invoke `pytest` directly as a YAML or collection probe during conformance work; validate collection and behavior through the documented managed conformance command.
- A focused packaged conformance selector must include the canonical `capability_admission` test in the same managed session, unless validated admission evidence and its exact context are supplied explicitly.
- When the requested conformance deliverable is a full-suite run or complete failure inventory, test failure markers are results to collect, not a reason to interrupt the run; continue to the terminal summary unless the harness itself fails, hangs, or the user cancels.
- Do not manually launch Barn or Toast for conformance work unless the user explicitly asks for a manual repro.
- Do not manually run against a tracked database file. If a manual repro is explicitly required, use a disposable copy outside tracked fixtures.
- Do not create ad hoc debug executables with arbitrary names for conformance work. Use the managed server flow, or one clearly temporary disposable binary under `.tmp` only when the user explicitly approves a manual repro.
- If the managed harness command fails to start or connect, report the exact command, working directory, expected result, and actual failure before improvising.
- Do not guess at startup failures, transport failures, IPv4/IPv6 issues, or harness behavior. Prove them or report them as unknown.

## Conformance Verification Rule

- Describe behavior shared across MOO servers as MOO behavior or MOO conformance; use Toast only for statements specifically about the ToastStunt oracle implementation, executable, or a verified Toast-specific divergence.
- Before debugging, implementing, or changing Barn for a conformance-test behavior that is uncertain, surprising, order-dependent, or poorly understood, first verify the exact behavior against Toast.
- When a conformance assertion depends on `caller`, invoke the behavior through an explicit test-owned driver verb and assert that receiver; do not infer the harness eval receiver.
- For Barn Toast-oracle and conformance work, Toast means the WSL oracle documented in `plans/barn-toast-mongoose-convergence-workstreams.md`: `/root/src/toaststunt/build-release/moo` (`~/src/toaststunt/build-release/moo` inside WSL). Do not use Windows Toast binaries for this workflow unless the user explicitly asks for a Windows Toast repro.
- Do not spend time on Barn-side root-cause analysis until the expected Toast behavior has been confirmed.
- If Toast verification is blocked, stop and say so plainly before proceeding further.

## Verification Claim Discipline

- No verification or status claim without adjacent raw output in the same message. This applies equally to intermediate commentary and final responses; without the output, say the result is unknown.
