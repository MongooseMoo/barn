# Current campaign checkpoint 40

- Task: fix CI so every packaged conformance identity executes against pinned Toast across the admitted profile union.
- Conformance PR 48 branch: `agent/all-toast-zero-skips`; first commit `f512248` removed the reviewed never-executed baseline and made the execution ledger require exact collected/passed identity equality.
- Red proof: GitHub run `31017076556` failed the full-suite ledger with exactly 41 identities never executed while the three original Toast profiles and quality checks passed.
- Current second wave adds a fourth 64-bit example-extension Toast profile for `read_stdin`, fail-closed exact one-line source-toggle verification, and an uploaded source-config patch as provenance.
- The other 36 identities were made executable without renaming/removal: 24 corrected GC fixtures, 3 corrected anonymous stack fixtures, 3 corrected wildcard verb setup cases, 3 strict numeric cases, absent `file_version` registry behavior, 64-bit-only minint modulus, and corrected `tostr` tick budget.
- Managed WSL Toast proof: admission 1/1; corrected GC group 42/42; corrected non-GC group 15/15. Stock WSL cannot prove the example-enabled profile; GitHub's managed pinned-source build must do so.
- Local gates green after all current edits: 332 unit tests, 11,920 total collected tests, focused workflow/ledger/schema tests, exact duplicate baseline, Ruff, mypy, and `git diff --check`.
- An accidental raw full pytest invocation attempted the default external socket and failed connection before any packaged case executed. It was not used as proof; runtime verification remains managed only.
- Current files are uncommitted second-wave changes in the PR 48 worktree. Next: review final diff, commit explicit paths, push, inspect all four managed Toast jobs and require the exact 11,587/11,587 ledger union before review/merge.
- PR 45 exact head `a9af53c` is being close/reopened solely to retrigger missing exact-head CI; it remains draft and unmerged.
