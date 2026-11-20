# Frame observer measurement

## Goal and state

Measure the real language-visible callers(1) observer on an eight-frame task,
comparing the existing PR implementation with its default-branch base. Preserve
ten raw samples per side, allocation measurements and benchstat output. The
parent owns publication and the separate task_stack samples.

Only engine/task_builtins_test.go benchmarks, this report and the phase note
are owned by this worker. No broad local tests, publication or merge is allowed.
The six production files may be temporarily restored to the base only through
apply_patch and must be restored exactly to their current committed contents.

## Evidence

Initial worktree status has a modified phase note and benchmark test file,
untracked analyst report and parent-owned task-stack sample capture. No
production file is dirty. Measurement is in progress.

The added BenchmarkCallersFormatting dispatches the real builtin through the
registry with eight actual task.State activation frames. callers excludes its
current frame, so the measured result has seven frame lists. It measures both
callers(0) and callers(1). No executable VM fixture is manually installed.

Initial candidate-then-base samples use Linux/amd64 Go 1.27.1, Ryzen 5950X,
GOMAXPROCS=1, ten samples of 300 ms per case. Raw samples are saved as
system-layering-callers-original.txt and system-layering-callers-refactored.txt.
The only production diff temporarily removed was the six-file PR diff. After
restoration, git diff --exit-code over those six files returned zero.

Initial benchstat:

```text
callers(0) time: 4.270us +/-5% -> 3.642us +/-5%, p=0.000 n=10
callers(1) time: 3.336us +/-9% -> 3.393us +/-11%, p=0.436 n=10
callers(0): 7360 -> 6016 B/op; 39 -> 25 allocs/op
callers(1): 6128 B/op and 25 allocs/op on both sides
```

These initial timing intervals are too wide for a strong common-path conclusion.
No significant callers(1) timing difference was detected, but this is not proof
of all-input non-regression. A CPU-affinity/interleaved comparison is complete
below.
WSL exposes CPU affinity 0-31 but no cpufreq control directory; no governor or
turbo claim is made. A first build probe stopped before go test because .tmp did
not exist. The actual benchmark binary build uses an explicitly absent /tmp
target instead; that probe generated no source change.

## Affinity-pinned interleaved comparison

Built two engine test binaries from the identical edited benchmark file. Each
was built with `GOMAXPROCS=1 go test ./engine -c -o <target>`. The original
binary uses the base versions of exactly the six changed production files;
the refactored binary uses the committed PR versions. No other production
file was edited. Both are retained under /tmp as
barn-frame-observer-{original,refactored}-20261107.test in Debian.

After restoring the six production files, repeated ten original/refactored
pairs. Each process used `GOMAXPROCS=1 taskset -c 2 <binary>` and:

```text
-test.run=^$ -test.bench=^Benchmark(Callers|TaskStack)Formatting$
-test.benchmem -test.benchtime=300ms -test.count=1
```

Raw outputs are system-layering-frame-pinned-original.txt and
system-layering-frame-pinned-refactored.txt: each has ten samples for each
of four cases. system-layering-frame-benchstat.txt retains the exact comparison.
The parent-supplied task_stack fixture uses a suspended registered task with
eight frames, line numbers and an optional preexisting runtime-variable map.
The benchmark includes task lookup, snapshot and list construction. No VM is
manually installed and no broad local tests ran.

```text
case                    original       refactored      p-value
callers(0)              4.053us +/-7%   3.069us +/-5%    0.000
callers(1)              3.131us +/-5%   3.223us +/-6%    0.363
task_stack lines        4.747us +/-6%   3.403us +/-7%    0.000
task_stack lines/vars   4.827us +/-23%  3.642us +/-5%    0.000

case                    bytes/op old -> new   allocs/op old -> new
callers(0)              7360 -> 6016           39 -> 25
callers(1)              6128 -> 6128           25 -> 25
task_stack lines        8176 -> 6384           44 -> 28
task_stack lines/vars   8176 -> 6640           44 -> 28
```

The common six-field callers(1) path has identical allocation counts/bytes and
no detected timing difference in either comparison. Its interleaved timing
point estimate is 2.9% higher; this evidence does not establish exact CPU
equivalence or exclude a small regression. The other three cases consistently
reduce heap allocation, with lower observed timings in these samples. The
wide original task_stack/vars interval, WSL host interference, unavailable
frequency control, fixed original-first order inside pairs, one frame depth,
and one Linux Go/CPU combination limit timing conclusions. This is no
application-wide speedup or all-input non-regression claim. No performance
change or speculative special case was added in response to unresolved noise.

## Final state

Both benchmark binaries compiled successfully and all measured invocations
reported PASS. Explicit `git diff --exit-code --` over all six production
files returned zero after restoration. Only the benchmark test file, reports
and phase note are changed by this measurement work. Parent owns existing
analyst/task-stack report edits and publication. No commit, push or merge ran.

Final direct probe output:

```text
source_diff_exit=0
diff_check_exit=0
primary temporary measurement note exists: False
```

The checkpoint-required temporary primary-checkout task note was consolidated
into the existing phase note and removed through apply_patch. No extra living
task note remains from this worker.
