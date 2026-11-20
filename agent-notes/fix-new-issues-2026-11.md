# Fix new issues #909-#951 (2026-11)

## Goal

Every open issue #909-#951 that still fails on origin/master gets a fix on
its own branch, ready for the lead to push, PR and queue. #935 and #951 are
out of scope (their PRs merged).

## State

Fuzz issues, each input run on origin/master and at the merges around the
fix (`go test ./vm -run 'FuzzCheckpointResumeEquivalence/<input>$'`):

| Issue | Input | Verdict |
|---|---|---|
| #909, #910, #912, #913, #915, #920, #921, #925, #926 | see issue | pass on master; fail before the #922 merge, pass after it |
| #941, #942 | 007dfca672765d97, 0329b7ec28a39328 | pass on master; fixed by #919 (inputs already in the corpus) |
| #943 | 2a07eca62ef43fe3 | fixed on branch `fix/checkpoint-resume-fuzz` |
| #949 | 95c78688fa989aab | fixed on branch `fix/checkpoint-resume-fuzz` |
| #946 | n/a | branch `fix/anonymous-identity-namespace` |

### #943, #949: lines are counted as Toast counts them

Both are a queued fork whose body begins with an empty statement `;`. Barn
numbered lines by source position (`compiler/lower_emit.go` `trackLine`), so
the `;` took a line in the uninterrupted run, but the checkpoint header's
first line comes from the body's first instruction, which is on the next
line, so the restored fork was numbered one too high per checkpoint.

Toast numbers a traceback's lines by counting statements in the decompiled
listing from `first_lineno` (`decompile.cc` `find_hot_node`,
`find_line_number`). An empty statement compiles to nothing and is not
listed; source layout plays no part. `compiler/toast_lines.go`
(`numberStatements`) gives every statement its Toast line, and `trackLine`
uses it. The Toast oracle confirms the rule:
moo-conformance-tests branch `test/traceback-line-numbering`,
`language/traceback_line_numbering.yaml`, 8/8 pass on WSL Toast; Barn master
fails `empty_statements_take_no_line` (got 4, Toast 2); the other seven
already passed because `set_verb_code` stores formatted code.

## Decisions and why

- PR #944 (closed) made Barn self-consistent by numbering a restored fork
  from its first statement's source line. Toast disagrees with that, so this
  fix changes the numbering to Toast's instead; with it both runs agree on
  these inputs because Toast's listing has no `;` line either.
- Three Barn unit tests asserted source-position lines for raw source with
  one-line loops or a leading blank line; their expected lines were changed
  to the Toast count (`engine/contention_diagnostics_test.go`,
  `vm/interp_dispatch_prop_read_test.go`).

## Surprises

- Toast's restored fork whose body begins with try/except numbers from the
  except line (pc 0 is the first arm's codes), so its lines move after a
  restart (PR #944's oracle run: `[5, 9, 14, 15]` before, `[7, 11, 17, 18]`
  after). Barn does that only for computed codes; with literal codes or ANY
  it numbers from the try line. Matching Toast here would make
  FuzzCheckpointResumeEquivalence fail as Toast would. Not changed.
- Toast's second restart of a queued fork writes first line 1: a program read
  back has `cached_lineno = 1` for `(MAIN_VECTOR, pc 0)` (`program.cc:34-36`),
  and `write_forked_task` asks exactly that (`tasks.cc:1906`). Barn keeps the
  real line. Not replicated.

## Next step

The lead pushes `fix/checkpoint-resume-fuzz`, then the conformance branch
`test/traceback-line-numbering` after it merges (Toast-green, Barn-red until
the Barn fix lands).
