# PR 49 GC isolation checkpoint

- Scope: `src/moo_conformance/_tests/builtins/gc.yaml` in the `moo-conformance-toast-contract-cleanup` worktree.
- Finding: all 24 repaired Toast GC fixtures had success-path cleanup but lacked failure-path isolation.
- Current state: all 24 now initialize cleanup handles to invalid object values and use MOO `try`/`finally` with guarded anonymous-object-first, prototype-last recycling.
- Semantic safeguard: temporary references for root-value, chain, and two-object cycle fixtures are cleared before the original happy-path collection point.
- Proof so far: diff audit reports 24 added `finally` blocks matching 24 removed skips; `git diff --check` passes.
- Blocker: none currently known.
- Next action: run non-runtime YAML/schema or collection checks, audit the final diff, and report without committing or pushing.
