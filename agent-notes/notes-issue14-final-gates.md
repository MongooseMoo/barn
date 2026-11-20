# Issue 14 correction checkpoint

- Worktree: `C:\Users\Q\code\.worktrees\moo-conformance-issue-14`.
- No commit, push, or staging has been performed.
- The ordinary pytest source example now invokes pytest from the repository root; the packaged conformance module remains the sole execution owner.
- Option probing accepts only the pinned Toast success values observed through the managed harness: `ON` and `#-1`. All other successful values, including `OFF`, fail closed. Both option-path `E_INVARG` results also fail closed.
- Capability probes explicitly select wizard identity. Connectable and ephemeral probes no longer catch arbitrary MOO errors as absence.
- Managed pre-fix Toast evidence showed connectable and ephemeral cases skipping after programmer cases while fresh wizard cases passed.
- Managed post-fix Toast evidence: connectable 2 passed; ephemeral 3 passed; width 1 passed and the incompatible 32-bit case skipped on pinned 64-bit Toast.
- Unit suite: 167 passed.
- Diff check: green.
- Ruff found two import-order issues; both were auto-fixed on the two affected files and need a clean rerun.
- Project environment does not include mypy; next action is an ephemeral mypy run plus Ruff, parity, corpus-identity, status, and final diff verification.
- The WSL managed run recreated a Linux `.venv`; Windows verification uses `%TEMP%\moo-conformance-issue14-win-venv` and does not delete or mutate the shared `.venv`.
