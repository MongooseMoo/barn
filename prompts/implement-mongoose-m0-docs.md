# Milestone 0 implementation: durable managed WSL oracle instructions

Work in `C:/Users/Q/code/barn` on the current `master` checkout.

Read these authoritative inputs first:

- `plans/barn-toast-mongoose-convergence-workstreams.md`
- `docs/reports/mongoose-phase0-barn.md`
- `docs/reports/mongoose-phase0-conformance.md`
- `docs/reports/mongoose-phase0-oracle.md`
- `AGENTS.md`

Implement only the Milestone 0 documentation repair:

1. Rewrite the conflicting conformance sections of tracked `CLAUDE.md` so they point to the managed WSL workflow and no longer direct ordinary Windows Toast, attached-port, manual lifecycle, read-only-conformance, or Barn-fix-only behavior.
2. Rewrite the existing untracked `reports/toast-oracle-wsl.md` into the canonical tracked managed procedure described by the audits. It must document both pinned WSL binary paths and SHAs, `scripts/run_toast_wsl.sh`, disposable `{db}` and `{db}.new` behavior, stock and Mongoose command shapes, `TOAST_MOO`, the uncommitted `MONGOOSE_LOGIN_SCRIPT` environment-variable mechanism, required run-record fields, and the requirement to freshly verify WSL identities and fixture SHA-256 before each authoritative run.

Do not launch Barn or Toast. Do not inspect or change credentials. Do not change `AGENTS.md`, scripts, profiles, source, tests, the convergence plan, or any other file. Do not stage or commit. Preserve unrelated worktree contents. Run read-only fixed-point searches and `git diff --check` when finished, then report exact paths and findings.
