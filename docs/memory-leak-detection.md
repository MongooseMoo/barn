# Memory and resource leak checks

The existing discovery jobs now check resource lifetimes as well as results.
Native Go 1.27 `goroutineleak` profiles detect permanently blocked goroutines;
they do not detect every goroutine leak. Heap growth is a separate candidate,
and RSS alone is never a leak verdict.

## Checks and evidence

- CI engine tests capture before/after the process and repeat eval, suspension
  and automatic anonymous cleanup on one live runtime. `engine-memory-*` keeps
  raw samples, heap profiles and full stacks.
  Test subprocess captures use separate `process-<pid>` directories.
- Elle's existing driver captures each process and each stopped model runtime.
  The latter retains the existing task/history/read-floor/pending-GC census.
  Histories are still checked even when memory checks fail. A fault case's
  expected nonzero exit cannot conceal a nonempty leak profile.
- Jepsen's `--leak-check` captures the owned PID and start-time identity at
  admission, resume, before termination and final connection drain. Stopped
  checkpoint processes get `/proc` evidence without HTTP requests or SIGCONT.
  Reanalysis checks native profiles independently. Memory leaks use verdict
  `memory-leak`/exit 7; correctness violations keep precedence. Negative controls
  cannot turn an unrelated memory failure into a passing result.
- The two source-fuzz lanes replay up to eight real cache inputs, rotating the
  selection each cycle. A separate test executable runs each input's existing
  execution helper 98 times in one process, with normal per-input cleanup. Two
  warmup cycles precede six windows of sixteen cycles. This does not put a
  collection or profile capture in each fuzz mutation.

Replay is bounded to 60 seconds per 30-minute source lane, including compilation.
This is a wall-time budget, not a measured CPU overhead percentage. Every initial
finding needs a second matching replay in another process before reporting.
Insufficient confirmation time, timeout, missing records, build failure and
inconclusive results remain incomplete, and cycle acceptance rejects them.

Each replay keeps its original Go corpus record, decoded source, commands and
toolchain, sample series, before/after heap profiles, full stacks and native leak
profile. Findings use the existing fuzz reporter/repair path and artifact
retention. Reproducible retained growth is reported as retained growth, not as a
proven retaining reference. Shrinking a multi-operation trace is future work;
the implemented replay preserves the entire fuzz input and its generated source.

## Heap interpretation

The bounded replay predicate looks for more than 32 KiB growth in every window
and more than 1 MiB total growth after warmup. It is a discovery heuristic, not a
proof of no leaks: smaller, intermittent or slower leaks may escape it. Exact
descriptor-return and native goroutine checks have separate reasons. Controls
cover intentional heap retention, a permanently blocked goroutine, an unclosed
descriptor, proper cleanup and a bounded cache.

Live heap comes from `/gc/heap/live:bytes`. Other samples include heap objects,
stack memory, collections, goroutines, RSS, descriptors and threads. Shared
cgroup `memory.events`/`memory.stat` and tmpfs usage are runner context, not
attributable bytes from one input. Unsupported platform measurements are marked
unavailable. The descriptor oracle is Linux-only.

Go GC never substitutes for Barn anonymous/WAIF cleanup. Replay does not call
`run_gc` to conceal an automatic-cleanup defect. Append histories, recorder
buffers, intentionally suspended tasks and a growing world cannot be subjected
to a constant-size heap assertion; captures retain their context instead.

## Replay commands

Run managed campaigns through their existing workflows. For a retained public
source/structured corpus record, ordinary bounded in-process investigation is:

```sh
BARN_LEAK_INPUT=/absolute/path/to/record \
BARN_LEAK_OUTPUT=/tmp/new-memory-evidence \
  go test ./vm -run '^TestMemorySourceReplay$' -count=1 -v -timeout=30s

go tool pprof -inuse_space -base /tmp/new-memory-evidence/baseline/heap.pprof \
  /tmp/new-memory-evidence/final/heap.pprof
```

`BARN_LEAK_OUTPUT` also opts engine test programs into process captures. The
managed Elle driver supplies it per case. Normal fuzz callbacks remain unchanged.
The general mechanism can be applied to other existing workload cleanup
boundaries using `internal/leakcheck`; no second server harness is introduced.
