# Spec catch-up for behaviour merged 2026-09-19 to 2026-11-19

## Goal

Q 2026-11-19: "find all the shit merged in the last couple of months here and
in ../moo-conformance-tests that wasn't captured in the spec, and ... update
the spec".

## Scope

- Barn PRs merged since 2026-09-19 (453) and moo-conformance-tests PRs merged
  since 2026-09-19 (133). Lists: `gh pr list --state merged --search
  "merged:>=2026-09-19"` in each repo.
- The spec documents Toast (CLAUDE.md). Every added claim is backed by a merged
  Toast-passing conformance test or Toast source file:line.
- Out of scope: perf, CI, fuzz, Elle/Jepsen, harness tooling; builtins only the
  Mongoose ToastStunt fork has (validate_string_lines, waif_owner_count, AST
  builtins, valid(obj, incarnation)), because stock Toast lacks them.

## Ownership (one agent per file set, worktree C:/Users/Q/code/barn-spec-catchup)

- A: spec/grammar.md, statements.md, operators.md, errors.md, types.md
- B: spec/objects.md, spec/builtins/objects.md, properties.md, verbs.md
- C: spec/tasks.md, spec/builtins/tasks.md
- D: spec/database.md, spec/vm.md
- E: every other spec/builtins/*.md, spec/server.md, spec/login.md

## State

- A done (grammar, statements, operators, errors, types); sent back to fix
  operators.md 2.2 (map indexed assignment on a missing key).
- C done (tasks.md, builtins/tasks.md); sent back to drop the "Barn provides it
  by default" line in builtins §2.4 and verify ticks_left/seconds_left in eval.
- All five agents finished, follow-ups included: B (server_options is an
  object; TYPE_FLAG on isa/locate_by_name/occupants/locations; depth-first
  lookup), C (builtins/tasks.md §7 now points to builtins/server.md; tasks.md
  §10.3 rewritten from the 17 mct#162/#163 callback suites), D (re-saved
  restored fork loses its first line), E (renumber and anonymous objects).
- 24 spec files changed; committed on spec/catch-up-2026-11 and PR opened.
- Left for later, pre-existing and outside the merged-PR scope:
  builtins/tasks.md §8 and §9 duplicate server.md, network.md and time.md;
  output_delimiters, switch_player, connection_name_lookup and raise exist only
  there and would have to move to their owner files first. types.md 2.3 says
  strings are bytes (Toast) while Barn strings are Unicode by design.
- Possible new conformance test: verb/property lookup order in a diamond
  (Toast is depth-first; no test tells it from breadth-first).
- Checked by me against origin/master: OP_INDEXSET mapinsert (A), bf_seconds_left
  INT and task perms (C), db2_add_parents depth-first (B), first_lineno only set
  by read_task_queue (D). Diff scan: no AST / fork-only builtins added.
- Not documented, not stock Toast: Unicode char builtins (#334), tz_offset
  (#428), mapmerge, PROMOTE_NUMBERS float promotion (#825).
- No conformance test distinguishes depth-first from breadth-first lookup;
  objects.md §3.3 rests on Toast source.

## Decisions and why

- builtins/tasks.md §4.4 threaded-call text kept on Toast source alone: the
  conformance test for it (mct#110) was withdrawn by mct#114 and never restored,
  and the brief admits Toast source file:line as evidence.
- types.md 2.3 ("strings are bytes") left: Barn strings are Unicode by design.

## Surprises

- `C:/Users/Q/src/toaststunt` working tree is on `fix/json-duplicate-keys`,
  older than the 2.8 oracle (numbers.cc). Cite Toast via
  `git show origin/master:src/<file>`. All agents told; memory updated.

## Next step

Watch the PR's checks; fix anything red. Remove the worktree and branch after merge.
