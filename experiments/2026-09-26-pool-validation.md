# Combined pool validation

Production source is exactly the three independently reviewed pool changes. Rejected commit scratch and conversion variants are not active code. The benchmark and byte-contract harness for the rejected conversion variants is retained for reproduction.

## Windows full suite

At combined source `dc6a477`, `go test ./... -count=1` exited 1. Every package except builtins passed; the one reported test failure was:

```text
--- FAIL: TestDivergenceRuntimeIntrospection (0.01s)
    divergence_edges_test.go:109: memory_usage() = flow 4 error E_QUOTA
```

The saved unchanged-baseline `unique-base.exe` was compiled from source `3c0f510` with only the unique benchmark harness added. Running it with `-test.run=^TestDivergenceRuntimeIntrospection$ -test.v -test.count=1` produced:

```text
memory_usage() = flow 4 error E_QUOTA
unchanged baseline exit=1
```

The same focused candidate command also exited 1 with the identical error. This establishes a Windows baseline limitation, not a passing full suite. No semantic fix was attempted in this optimization PR. Full stdout is preserved in `2026-09-26-pool-review-evidence/windows-full-tests.txt`.

```text
go vet ./...: vet exit=0
go test -c -o .tmp/engine-candidate.exe ./engine: candidate test binary exit=0
```

Independent reviewer ownership/concurrency and race tests, plus ten-pair holdouts, are recorded in `2026-09-26-pool-independent-review.md`. Their scope does not replace full repository or conformance checks.

## Final narrowed-source checks

After the narrowed finalization source `ea251f7` was integrated, final build,
vet, and source whitespace checks completed:

```text
go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn
github.com/MongooseMoo/barn/cmd/barn main
go build ./...: build exit=0
go vet ./...: vet exit=0
git diff --check origin/master -- builtins vm db/store: source diff check exit=0
```

Focused normal/race tests and the full VM test run for the narrowed source are
recorded in `2026-09-26-pool-refinement-review.md` and the refinement record.
The full Windows-suite baseline limitation above remains; Linux CI is separate.

## Application environment observations

The fixed five-pair application experiment runs one workload process at a time; no campaign tests, builds, or profiles overlap it. During the run, Windows reported total CPU utilization 99%, later 77%. One process-counter snapshot showed Everything at 1122% (the process counter sums logical-core use), WSL at 191%, System at 117%, and several Python processes near 100% each. Free physical memory was 33,624,148 KiB of 134,125,572 KiB. These observations establish competing work, not causation for any particular baseline/candidate difference. All originally planned pairs remain in the result; none are excluded or replaced.
