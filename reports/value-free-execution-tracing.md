# Value-free execution tracing (#209)

Ordinary `--trace` previously rendered every argument and return with
`types.Value.String()`, printed notification text, and included the remote
address in connection records. The pre-authentication input path passes the
login words as verb arguments, including a conventional password.

## Contract and implementation

The trace API now takes an argument count and result `TypeCode`, rather than
MOO argument/result values. Notification and connection APIs no longer have
message or detail parameters. Engine, VM, builtin, and server callers pass only
metadata. Error records use fixed `ErrorCode.String()` names, including bounded
`E_UNKNOWN`; unknown result types become `UNKNOWN`.

Object, verb, player, caller, and connection identities remain. Verb names are
quoted to keep newlines inside one record. No duration or task-correlation
service was introduced. No raw-payload mode was retained.

Filters select call/return/exception events; they cannot enable payload
capture. Startup removes blank entries and surrounding whitespace. This also
repairs the empty CLI filter, which previously became a single empty pattern
and hid all verb events. Notification/connection selection is unchanged.
The guide and flag help now describe the metadata-only contract.

## Regression evidence

The unchanged tracer failed the new server integration regression by exposing
the password, returned string, notification, connection address, and exception
argument sentinels. The final test exercises both unfiltered and filtered
tracing, direct and scheduled pre-authentication dispatch, a nested verb call,
list/map/waif arguments and results, custom exception message/value, and actual
connection lifecycle processing. It checks notification delivery and returned
values alongside absence of every sentinel from trace and captured debug-level
JSON output. No MOO evaluation, argument, or transport payload is changed.

Focused trace tests also cover exact structural records, bounded unknown
codes, exclusion filters, concurrent record integrity, quoted verb newlines,
writer failures without retry output, and a disabled path with zero allocations
and a writer that panics if called. An allocation assertion rejects growth per
argument.

The touched production call sites add no structured JSON fields and do not
pass payloads into tracing. Existing ordinary traceback/source logging has its
own contract; this change does not redesign that schema or MOO-visible
tracebacks. The integration test captures existing exception/connection JSON
records to check dynamic argument/error/remote-address sentinels as well.

## Verification

Debian WSL, Go 1.24.6:

```text
go test ./... -count=1
go test -race ./trace ./server ./engine ./vm ./builtins
go vet ./...
staticcheck ./...
go test ./trace -run '^$' -bench BenchmarkTraceMetadata -benchmem -count=3
TRACE_FULL_RACE_STATIC_BENCH_OK
```

Race runs: trace 1.029s, server 14.350s, engine 206.895s, VM 66.118s,
builtins 66.767s. Three benchmark repetitions report 16 B/1 allocation for call
counts 0, 1, and 128; 24 B/2 allocations for count 4096 (integer boxing); and
32 B/2 allocations for STR/LIST/MAP/WAIF return metadata. This verifies bounded
formatting allocation, not a timing improvement claim.

The documented managed command, with `--trace` enabled, selected canonical
`capability_admission`, `notify_call_shapes`, and
`audit_listener_handler_do_login_command`: **57 passed, 23 deselected in
12.61s**. It used the packaged disposable Test.db session, strict markers,
unexpected-skip rejection, and the stock WSL oracle manifest. The source
Test.db SHA-256 remained
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
No fresh baseline or manual server/oracle was run for this host-only change.
