# Activation frame list formatting consolidation

## Goal

Make the existing ActivationFrame.ToList owner construct the complete requested
frame list, including optional line and variable fields, and delete a one-call
error-name forwarding function.

## State

The six-file mechanical consolidation is implemented. Read the language scout,
ActivationFrame/RuntimeVariableMap/NewList owners and all seven former ToList
calls before editing. The callers() true/false branches now share one call, so
six production calls remain. The six source edits and this report now live on
the independent refactor/activation-frame-lists branch from master. Their exact
campaign hunks were removed; other in-progress source changes are preserved.

Source diff: 30 additions and 62 deletions across types/activation_frame.go,
builtins/tasks.go, vm/traceback.go, engine/lag_callback.go,
engine/task_runtime.go and compiler/literal_values.go.

Raw checks:

```text
gofmt_exit=0
diff_check_exit=0
removed_wrapper_search_exit=1
builtins/tasks.go:331: frame.ToList(includeLineNumbers, false)
builtins/tasks.go:484: frame.ToList(includeLineNumbers, includeVariables)
vm/traceback.go:38: f.ToList(true, includeVariables)
engine/lag_callback.go:61: stack[i].ToList(true, s.session.IncludeRTVars(t.Context))
engine/task_runtime.go:915: stack[i].ToList(true, s.session.IncludeRTVars(t.Context))
engine/task_runtime.go:1239: stack[i].ToList(true, s.session.IncludeRTVars(t.Context))
```

The wrapper search was `rg -n 'errorNameToCode' . --glob '*.go'`; exit one
means no remaining Go reference. All ToList callers now supply both booleans.

## Decisions and why

Use the existing method with explicit line/variable booleans and one exact-size
final slice. Preserve caller-side filtering, authority redaction, anonymous
invalidation, frame order and lazy variable snapshots. Do not add a wrapper,
parallel DTO or options layer. Use types.ErrorFromString directly at its caller.

ToList allocates a slice of exactly 5 plus the requested optional fields,
constructs the fixed fields once, places line number before variables and calls
RuntimeVariableMap only when requested. The existing fixed-six-field paths still
construct one six-element slice and one MOO list rather than an intermediate
list/copy. This is source structure, not a measured allocation/performance claim.

Caller-side anonymous handling, permission checks, ordering, eval/frame filtering
and snapshot capture remain untouched. Removed each call site's trim/copy/append
formatting branch. Compiler literal conversion calls types.ErrorFromString
directly, retaining its unknown-code error path and deleting the forwarding
function. No compatibility wrapper or new owner was added.

No tests or builds ran locally because this task writes no test file. No bespoke
mirror tests were added. Existing task_builtins flag combinations, lazy runtime
variables, anonymous traceback, effective authority, eval callers and traceback
integration coverage remain the PR CI verification source. No speedup or measured
non-regression is claimed.

## Next step

Return the isolated six-file diff for coordinator review and an independent PR
against the default branch. The move created the worktree/branch but performed
no commit, publication, review posting or merge.
