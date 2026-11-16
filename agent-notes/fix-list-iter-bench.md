# BenchmarkVM/list_iter_1M compiles again

## Goal
`go test ./vm -run='^$' -bench=BenchmarkVM` failed at list_iter_1M with a
compile error, reported by the toast-2.8-minint agent on 2026-11-16.

## State
- Cause: the workload used `{1..1000000}`; 42346278 (2026-11-06) made the
  parser reject ranges in list literals, as Toast does.
- Fix: build a 1000-element list with `{@l, i}` and iterate it 1000 times
  (1M iterations, as before). Runs: 47 ms/op, 1041 allocs/op.

## Next step
- Merge.
