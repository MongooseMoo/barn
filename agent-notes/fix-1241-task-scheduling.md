# Use the unfinished task index for live scans

## Goal

Fix #1241 by bounding reader lookup, queued-task enumeration and three GC root catalog scans by the unsettled task population.

## State

The five scans copy the unfinished index before inspecting task state. Catalog read locks still protect the reader and queue queries from concurrent removal. Runtime and execution-lease checks still protect the GC walks.

## Decisions and why

The unfinished index already includes every task that can appear in the reader or queue output. All three GC scans previously skipped terminal catalog rows. They still capture parked roots separately and fail closed for executing siblings where required.

## Surprises

Unfinished snapshots must release their leaf mutex before any operation that may take a task lock.

## Next step

Publish for review and CI.

## Benchmark

The same benchmark ran three times baseline and three times fixed on Windows amd64. With 10 readers and 10,000 completed tasks, reader lookup went from 362,468–431,443 to 348.2–363.8 ns/op. Queue enumeration went from 458,698–482,728 to 573.1–605.0 ns/op. The copy adds one 80-byte allocation per scan in this fixture.

`go test ./engine -run '^$' -bench '^BenchmarkTaskManagerScans$' -benchmem -benchtime=200ms -count=3`
