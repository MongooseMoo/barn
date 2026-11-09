# Commit applier (Wave 3b): allocation counts

Branch refactor/commit-applier, rebased on origin/master c305e0bd (#506, B14).
Allocation counts do not depend on machine load; ns/op does and is not compared here.

Command, run on each tree:

```
go test ./db/store -run '^$' -bench 'BenchmarkTxnPropertyWriteCommit|BenchmarkCommitScratch|BenchmarkTxnReadOnlyCommit' -benchmem -count=5
```

All five runs of each benchmark gave the same allocs/op; B/op varied by under 10 bytes.

| Benchmark | allocs/op before (c305e0bd) | allocs/op after | B/op before | B/op after |
|---|---|---|---|---|
| CommitScratch/objects1 | 34 | 36 | 4656 | 5072 |
| CommitScratch/objects8 | 142 | 148 | 21792 | 23720 |
| CommitScratch/objects32 | 546 | 567 | 99313 | 102288 |
| TxnPropertyWriteCommit | 33 | 35 | 4703-4711 | 5125-5132 |
| TxnReadOnlyCommit | 17 | 17 | 2800 | 2800 |

The same before numbers were measured on 491d7462 (before #502, #505, #506).

The design predicted fewer allocations. The commit allocates more. A memory
profile of TxnPropertyWriteCommit (`-memprofile -memprofilerate 1`, `pprof -list
writesByObject`) puts 4 allocations per op in writesByObject:

```
10098      10098     65:	writes := make(map[types.ObjID]*objectWrites)
10098      10098     94:		w.props = append(w.props, key)
10098      10098     69:			w = &objectWrites{}
10098      10098     70:			writes[id] = w
```

(10098 allocations over ~10100 ops: one per op per line.) The same profile of
the removed commitDecentralized on c305e0bd shows its only allocation sites in
the function body:

```
10098      10098    441:		propWritesByObj[key.objID] = append(propWritesByObj[key.objID], w)
 5067       5067    360:			writeIDs = append(writeIDs, id)
```

The new gatherer allocates a heap map and one objectWrites per written object,
which the old path did not; the multi-object rows grow by about one allocation
per extra object.

## Real-workload gate

Not run. Load at 2026-10-25T19:19 MDT, three 2 s samples of
`\Processor(_Total)\% Processor Time`: 18.7, 27.2, 25.6. The machine was not idle.
