# Persistent map deletion

Goal: close #1256 by removing insertion-chain copying from indexed deletion.

Decisions: sequence numbers live on indexed entries. Overwrites retain their sequence and reinsertion uses the next sequence. Raw insertion consumers sort entries on demand. The lazy lookup tree still rebuilds in insertion order, preserving its topology and existing comparator edge cases. Neither the flat entry layout nor the checkpoint format changes.

State: `go test ./types -run '^TestMapDeletePreservesSnapshotInsertionOrder$' -bench '^BenchmarkMapDeleteOldest$' -benchmem -benchtime=250ms -count=1` returned PASS. Indexed oldest-key deletion at 2,000 entries measured 1349 ns/op, 576 B/op and 4 allocs/op. A differential Barn/Toast run is unverified.

Next step: publish for CI.
