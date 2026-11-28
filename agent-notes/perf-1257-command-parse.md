# Command parsing allocations

Goal: Reduce parser time and allocations for #1257 while keeping the same parsed command fields.

State: An ordered first-word alias index replaces per-command alias copies and splits. ParseCommand reuses CommandWordList's tokens and gives Args its own slice. Plain words refer to spans of the input, and quoting and escaping still use the same byte rules.

Decisions: Keep strings.ToLower to preserve alias matching for non-ASCII input. Preserve the table's existing order, including matching "off" before "off of". Leave Argstr's byte-based boundary calculation in place.

Verification: Run `go test ./command -run '^TestParseCommandWordAndPrepositionOrder$' -bench '^BenchmarkParseCommand$' -benchmem -count=2`. The local Windows baseline for no_prep_8words was 25,206 ns/op, 9,408 B/op, and 374 allocs/op. The updated benchmark uses 480 B/op and 4 allocs/op. Repository checks run on the PR.
