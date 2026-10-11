# Conformance parser crash-safety checkpoint

- Task: add a neutral parser nesting resource-limit interoperability case in sibling `moo-conformance-tests`.
- Isolated worktree: `C:\Users\Q\code\.worktrees\moo-conformance-parser-crash-safety`.
- Branch/base: `agent/parser-crash-safety-conformance` from `origin/main` at `21add5a254843de871d1daf8ec929b26f5e3e55f`.
- Dirty sibling main checkout remains untouched; no conformance source edits exist yet.
- Current schema already supports multi-step tests, step expectations, and always-run cleanup; no new profile or schema feature appears necessary.
- Managed oracle is Barn `scripts/wsl_oracle.sh` with `MOO=/root/src/toaststunt/build-release/moo`.
- Oracle observation: emergency player is `#2`, `typeof({})` returns `4`, and `#2` has no `scratch` verb. Direct `set_verb_code(player, "scratch", ...)` returns `*Aborted*`; the following `;1 + 1` returns `2`, proving the process remains alive but the target verb setup is invalid.
- Earlier nesting sweep results are not usable contract evidence because they targeted the absent `scratch` verb; a Windows argv limit also blocked a 32K argument at depth 16,384.
- Blocker: establish a disposable existing verb target in the oracle invocation before measuring deep parser behavior.
- Next action: use managed oracle commands to create a uniquely named disposable verb on `#2`, program it with generated deeply nested source, assert `typeof(set_verb_code(...)) == 4`, send a follow-up responsiveness command, then delete the verb in the same invocation.
- No files are staged, committed, or pushed.

## Depth-2560 implementation checkpoint

- Improved oracle shape: a shallow helper verb builds `source = "1"`, wraps it in parentheses 2,560 times, and calls `set_verb_code` on a disposable target verb. This avoids emergency input-length bias.
- Exact canonical managed Toast outputs: target add `35`, target baseline code `{}`, helper add `36`, helper code `{}`, helper invocation `{}`, follow-up eval `2`, helper delete `0`, target delete `0`, final follow-up eval `2`. Toast accepts the generated source and remains responsive.
- Conformance source added only in isolated sibling worktree: `src/moo_conformance/_tests/language/parser_resource_limits.yaml`.
- Test contract is neutral: helper invocation must return type LIST, then `1 + 1` must return 2. It does not assert Barn's depth 256 or require Toast rejection. Both disposable verbs are removed by always-run cleanup.
- Schema tests: 87 passed. Selected suite collection: admission plus one conformance case.
- Pre-fix Barn `b3d730e8bd3579517124fd011126f0d514547ce2` managed selected run: 2 passed. This is expected because the portable invariant exercises dangerous recursion without asserting a Barn-only outcome.
- Documented managed Toast wrapper selected run: 2 passed.
- Repaired Barn `55978fdecfd8f3354cf5ecc1f58d0dff5e198198` managed selected run: 2 passed.
- No blocker. Next actions: duplicate lint, Ruff, focused Python suite, diff review, scoped commit, and final status report. No push or PR.
