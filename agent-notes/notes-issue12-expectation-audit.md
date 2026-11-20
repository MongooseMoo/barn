# Issue 12 expectation audit checkpoint

## Current findings

- Live issue `MongooseMoo/moo-conformance-tests#12` requires consistent enforcement of every supported expectation field while leaving unknown-field/cardinality, timeout, skip, and isolation changes out of scope.
- `Expectation` parses `value`, `error`, `type`, `match`, `contains`, `range`, `satisfies`, `notifications`, and `output`; nested output parses `exact`, `match`, and `contains`.
- Single-execution verification enforces `value`, `error`, `type`, `match`, `contains`, `range`, and `notifications`, but never `satisfies` or top-level `output`.
- Step result verification enforces only `value`, `error`, `type`, and `match`; it drops `contains`, `range`, `satisfies`, and `notifications`.
- Raw output verification enforces nested output `exact`, `match`, and `contains` for command/send/send_bytes/read steps.
- Tracked declarations found so far include two step-level `notifications` cases in `builtins/task_local.yaml`, eleven step-level `range` cases in SQLite suites, many raw-output declarations, and no tracked `satisfies` declarations.

## State and blocker

- Audit is read-only apart from this mandatory uncommitted checkpoint note.
- No Barn or Toast process has been launched.
- No blocker is currently known.

## Next action

- Finish the tracked-declaration inventory with exact test names and line evidence, then design focused red unit tests separating pure harness behavior from managed-oracle validation.
