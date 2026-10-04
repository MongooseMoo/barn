# Barn Execution Tracing Guide

`--trace` writes execution metadata to stderr. Calls report argument counts;
returns report MOO type names. Notifications report their recipient, and
connection events report connection/player IDs. Argument values, return values,
notification text, input lines, and remote addresses are omitted.

```bash
./bin/barn --db game.db --port 7777 --trace 2> trace.log
```

Tracing is disabled by default. There is no raw-payload trace mode.
`--trace-filter`, debug log levels, and configuration defaults cannot enable
payload capture.

## Verb filters

`--trace-filter` accepts comma-separated Go `filepath.Match` glob patterns.
An empty filter traces every verb. Surrounding whitespace and empty entries
are ignored. Nonempty patterns select call, return, and exception events only;
notifications and connection lifecycle events remain visible.

```bash
./bin/barn --db game.db --trace --trace-filter "look,@describe"
./bin/barn --db game.db --trace --trace-filter "do_*,user_*,get_*"
```

`*` matches a sequence, `?` matches one character, and `[a-g]` matches a range.
Filters change event selection without exposing arguments or results.

## Output format

```text
[TRACE] CONN NEW conn=2 player=#-2
[TRACE] CALL #0:"do_login_command" argc=3 player=#-2 caller=#-2
[TRACE] RETURN #0:"do_login_command" type=OBJ
[TRACE] CONN LOGIN conn=2 player=#8
[TRACE] CALL #8:"look" argc=0 player=#8 caller=#8
[TRACE]   NOTIFY #8
[TRACE] RETURN #8:"look" type=INT
[TRACE] EXCEPTION #8:"look" E_VERBNF
[TRACE] CONN DISCONNECT conn=2 player=#8
```

Verb identities are quoted so embedded newlines cannot forge records. Return
types are fixed names such as `INT`, `OBJ`, `STR`, `LIST`, `MAP`, and `WAIF`.
Exceptions contain a bounded error-code name, with `E_UNKNOWN` for an unknown
code. Notification messages still reach their recipients normally.

The login example shows an argument count only. A password supplied to the
login verb is never included in these trace records. No special verb-name
redaction rule is needed: the same metadata-only contract applies to every verb.

## Reading traces

```bash
grep 'EXCEPTION' trace.log
grep 'CONN' trace.log
grep 'CALL' trace.log
```

Object, verb, player, caller, and connection identities remain useful for
following execution. Call records identify the callee and caller; connection
IDs link lifecycle events. Records have no new duration or task-correlation
fields because those were not part of the existing tracer API.

## Cost and log handling

Disabled tracing performs a boolean check and no formatting or I/O. Enabled
tracing formats metadata under a mutex and writes one record per event. The
formatter does not traverse argument containers or render return payloads.
Its allocation cost does not grow per MOO argument.

Trace output can be verbose. Select relevant verbs and keep trace files under
the operator's usual access and retention policy. Stderr can also contain
ordinary server logs; the trace contract does not redefine their fields or
MOO-visible tracebacks.
