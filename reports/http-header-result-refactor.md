# Private HTTP header result (#364)

This slice replaces `parseHTTPHeaders`' six positional return values with an
unexported `httpHeaderParseResult`: headers, body start, content length, chunked,
and one `httpHeaderParseState`. The parser remains in builtins. Complete,
incomplete, and invalid input have distinct states; the zero state is incomplete.

| Surface | Disposition |
| --- | --- |
| Six-value header-parser signature and boolean state flags | Delete |
| Header parsing loop, accepted tokens/folding, length sentinel | Keep existing behavior under the named result |
| Request and response tuple unpacking | Rewrite to named fields and explicit state switches |
| Chunk parser, HTTP waiters, authorization, buffer lifetime | Keep with existing owners; outside this slice |
| Parser regression coverage | Extend with each return path and caller byte consumption |

No tuple adapter, alternate parser, public framework, or syntax change is allowed.

## Iteration 1

New regression tables passed against the old parser (`HEADER_BASELINE_EXIT=0`).
They cover empty/offset input, folded/binary headers, length/chunked metadata,
incomplete CRLF, invalid folds/tokens, and both callers' errors and consumption.
The initial test accessor and three hand-counted offsets were corrected before
recording that baseline; production behavior was not changed to satisfy them.

Deleting the old parser exposed exactly its required callers:

```text
network.go:325: undefined: parseHTTPHeaders
network.go:382: undefined: parseHTTPHeaders
network_headers_test.go:40: undefined: parseHTTPHeaders
HEADER_DELETION_GATE_EXIT=1
```

## Iteration 2

The same parsing loop returns named fields. Both callers switch on incomplete
and invalid states, retaining their previous values, offsets, body handling, and
error code. Complete results retain content length `-1` when absent; incomplete
results retain zero metadata and consume nothing; invalid results retain the
position after the rejected line. The temporary test tuple decoder is gone.

```text
ok github.com/MongooseMoo/barn/builtins
HEADER_TYPED_EXIT=0
```

The focused HTTP race suite, `go vet ./...`, `staticcheck ./...`, formatting,
and diff checks finished successfully:

```text
HEADER_RACE_AND_STATIC_OK
```

The repository-wide Go search for the six-value signature, `badHeader`, and old
header tuple unpacking reported `POSITIONAL_HEADER_SURFACES=0`. Inspection of
the final diff confirms that parsing and body handling retain their existing
operations and only the private representation changed. This bounded slice has
reached its fixed point: builtins owns the named result and parser, each caller
handles the state directly, and no compatibility path remains. The kept commit
is recorded below after committing. Full repository CI remains required before
normal merge; the next iteration selects another issue.
