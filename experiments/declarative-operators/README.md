# Declarative operator parsing measurements

PR #599, branch `refactor/declarative-operators`, 2026-11-04.

The candidate removes repeated binary-operator recognition and metadata
lookups from expression parsing. The baseline is the parent of the task's
implementation commit with the same new parser benchmark file supplied;
both baseline test binaries were compiled before editing production code.
Candidate binaries were compiled after that edit. Subsequent changes only
recorded these results and the grammar evaluation.

## Method

- Go 1.27.1, windows/amd64, AMD Ryzen 9 5950X, 32 logical processors.
- `-test.cpu=1`, one second per benchmark, ten independent samples per side.
- Alternated before/after pairs, reversing their order on every other pair.
- Parser and compiler measurements ran sequentially, after PR CI completed.
- No in-progress Barn Actions runs at the start or end of the comparison.
- This is a shared Windows workstation, without a pinned CPU clock or affinity.
  Whole-host CPU load samples before/after were 14%/11%; timing results are
  limited to this experiment, not proof of production throughput.
- `TestDB` parses every programmed verb, including empty/rejected source, in
  sorted source order: 444 programs per operation. Dump loading, source
  collection and sorting happen outside the benchmark timer. `Test.db` SHA256:
  `532FEF8F9AE4A0804D3638B8F65B3C2CEFD5F1B0C2FBD661EC248A694B40D04E`.
- `Nested` and `Malformed` exercise 128 levels of parentheses, with a binary
  expression or an incomplete operand. `CompileCacheCold/Single` is the existing
  benchmark that creates a fresh compiler for 256 increment statements.
- Startup-repair warnings in the raw logs are from untimed loading of tracked
  Test.db and appear on both sides. No database file was written.

From the respective package working directories, each variant was invoked as:

```powershell
& ../parser-before.test.exe '-test.run=^$' '-test.bench=^BenchmarkParseProgram$' '-test.benchtime=1s' '-test.count=1' '-test.cpu=1' '-test.benchmem'
& ../compiler-before.test.exe '-test.run=^$' '-test.bench=^BenchmarkCompileCacheCold$/^Single$' '-test.benchtime=1s' '-test.count=1' '-test.cpu=1' '-test.benchmem'
```

The candidate uses the corresponding `*-after.test.exe`. Binaries were built
with `go test -c -o <binary> ./parser` or `./compiler`, without custom build flags.
The initial invocation omitted quotes around some dotted flags and was rejected
before benchmarks; it contributes no measurement. All recorded invocations
exited successfully. Raw combined output is in `before.txt` and `after.txt`.

```text
benchstat experiments/declarative-operators/before.txt experiments/declarative-operators/after.txt
```

## Result

| Workload | Baseline median | Candidate median | Comparison |
| --- | ---: | ---: | --- |
| TestDB, full corpus | 1.976 ms | 1.943 ms | No significant difference, p=0.143 |
| Nested | 30.30 us | 28.79 us | -4.97%, p=0.005 |
| Malformed | 12.51 us | 12.20 us | No significant difference, p=0.247 |
| Uncached compilation | 1.158 ms | 1.112 ms | -3.98%, p=0.000 as printed by benchstat |

Allocation counts are identical for every workload. No significant allocated-byte
change was observed. Full statistical output is in `benchstat.txt`.

This experiment found no statistically significant timing regression in the
measured workloads. Retain the smaller metadata cleanup and the existing lookup
representation. The result does not establish a need for indexed lookup tables,
a full parser rewrite, or a general server-performance improvement.
