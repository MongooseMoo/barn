# Scalar map keys

Goal: close Barn #1258 by eliminating string formatting for anonymous object, error, and boolean map keys.

Decisions: use the inline payload for all three lookup representations. Out-of-range error codes are possible in checkpoint input and previously shared the E_UNKNOWN lookup spelling, so keep their common key. Keep comparison and presence tests across flat, bulk, and updated indexed maps.

State: `go test ./types -run '^TestMapScalarKeys$' -bench '^BenchmarkMapScalarKeys$' -benchmem -benchtime=200ms -count=1` returned PASS. Anonymous flat lookup was 24.66 ns/op, 0 B/op and 0 allocs/op. Anonymous indexed lookup was 43.47 ns/op, 0 B/op and 0 allocs/op. Error and boolean lookups also allocated zero bytes in both representations.

Next step: publish for CI.
