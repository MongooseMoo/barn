# fix/server-version-default-options

## Goal
Make `server_version()`'s `options` group declare Barn's own compiled limit
defaults under ToastStunt's names, so MOO code that reads them (for example
`server_version("options/DEFAULT_MAX_MAP_VALUE_BYTES")`) works as on stock
Toast. Issue #887.

## State
Done. `builtins/system.go` reports 12 more options from Barn's existing
constants in `builtins/limits.go`. New candidate conformance file
`tests/mongoose-conformance/builtins/server_version_default_options.yaml`
passes on stock Toast and on this branch, and failed on master.

## Decisions and why
- Only options Barn actually implements, under Toast's names, value shapes and
  order. Copying Toast's whole 54-entry list would claim compile options Barn
  doesn't have.
- `DEFAULT_FG_SECONDS`/`DEFAULT_BG_SECONDS` are integers because Toast emits
  them with `_DINT1`.
- Values come from Barn's own constants, never copied from Toast, so the report
  can't drift from what Barn enforces. The test checks names, types and
  default >= MIN_*, not Toast's numbers.

## Surprises
None.

## Next step
Toast also declares `DEFAULT_CONNECT_TIMEOUT` (300), `MAX_LINE_BYTES`
(5242880) and `DEFAULT_LAG_THRESHOLD` (5.0). Barn has matching values, but as
literals in `server/` and `engine/`, which `builtins` can't import. Reporting
them means moving those values into a package both sides can use. Left out of
this PR.
