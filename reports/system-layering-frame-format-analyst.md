# Activation frame formatting analyst review

## Scope and state

Reviewing published PR #640 against its actual base and worktree source. The
published diff contains six Go source files, the phase note, and two existing
reports. Initial `git status --short` produced no output. This review makes no
source changes, runs no local builds or tests, and publishes nothing.

Read the published diff and gauntlet analyst instructions, list ownership,
optional field construction, all six real observers and existing regression
coverage. Source review is complete. No correctness, security, race or error
handling defect was identified in the scoped changes. This is an analyst result,
not merge approval or a measured performance conclusion.

## Findings

### Performance evidence remains incomplete

types/list.go:261 returns listValue(newSliceList(elements)); line 125 retains
the slice in a sliceList, and line 138 returns that header through Value.ref.
NewList neither copies nor truncates its input. The old six-field literal and
the new types/activation_frame.go:74 make both supply the backing storage of
the returned list. There is no additional source-level temporary backing array
or constructor copy in the new common six-field path.

This does not establish identical allocation counts, inlining or CPU cost.
The method now has dynamic sizing, indexing and optional-map branches. No
compiler diagnostics or benchmark data were collected, so common-path runtime
non-regression is unknown. If that is a delivery requirement, compare real
callers(1) and traceback observers against the base, including allocations;
do not infer non-regression from fewer source lines or CI correctness results.

### Field construction and observation boundaries

types/activation_frame.go:61 preserves ThisValue fallback and the five fixed
fields. Flags false/false, true/false, false/true and true/true produce lengths
5, 6, 6 and 7. Variables occupy size-1, so without line numbers the map is field
6 rather than overwriting player or leaving an empty slot. RuntimeVariableMap
is called only within the requested-variable branch and does not mutate the
frame or snapshot (types/runtime_variables.go:22).

builtins/tasks.go:331,484 retain observer-side authority and anonymous handling,
eval boundaries and reverse iteration. vm/traceback.go:38 still formats only
after filtering/redaction. engine/lag_callback.go:61 and task_runtime.go:915,1239
retain their existing stack source, reverse order and per-frame option lookup.
Moving IncludeRTVars evaluation before fixed-field construction has no observed
side effect: construction reads fields and creates values, not shared state.
The compiler change calls exactly the former wrapper target and retains its
unknown-error return (compiler/literal_values.go:23).

### Coverage inspected, not rerun locally

engine/task_builtins_test.go:221 tests the optional-map/no-line field-6 case,
line/no-map case and full seven-field case. vm/lazy_runtime_variables_test.go:61
checks option-dependent six/seven-field traceback shape and bound locals; its
snapshot regression checks captured locals survive resume. Existing anonymous
traceback and callers authority tests inspect redaction; eval_callers_test.go:14
asserts five-field synthetic eval frame order. Real exception, failing-handler
and timeout-handler integration tests remain present. No new mirrored tests
were needed for the mechanical formatter move.

## Exact PR checks observed

```text
Build & unit tests pass 1m58s
Record pass pass 8s
Serializable histories and negative control pass 4m42s
MOO conformance suite pending 0
Check for an earlier pass pass 7s
Check for an earlier pass pass 7s
```

Source: `gh pr checks 640 --repo MongooseMoo/barn`. Conformance completion and
any measurement are left to the coordinator; no local builds/tests ran.
