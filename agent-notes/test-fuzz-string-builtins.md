# String builtin fuzzing

## Goal
Implement plan item C3: one registry-driven string builtin fuzz target, source-backed round trips, seeds, and nightly matrix entry. Publish an independent PR; do not merge.

## State
FuzzStringBuiltins (builtins/fuzz_test.go) and its fuzz.yml matrix line are done and verified locally on final source:
- `go test ./builtins -run '^(Test|FuzzStringBuiltins)' -count=1`: `ok ... 15.302s`.
- gofmt clean; go vet, staticcheck and golangci-lint on ./builtins clean (`0 issues.`).
- `timeout --kill-after=5s 300s go test ./builtins -run '^$' -fuzz '^FuzzStringBuiltins$' -fuzztime=4m -parallel=2`: PASS, 899,302 execs, 638 corpus entries, no failure.

### Zero execs/sec stall in the earlier campaign: harness defect, now fixed
- Per-exec cost was `newTestExecution()` (builtins/execution_test.go:5-8, a new `NewRegistry()` and `NewSession` each time): measured with temporary println probes (removed) at 10.7 to 17.7 ms per exec. The whole exec took about the same, so the builtin calls cost almost nothing. Over a 90 s instrumented campaign no builtin call took more than 200 ms.
- At about 12 ms per exec, minimizing each new interesting input needs thousands of execs. Go adds execs to the displayed count only when a worker returns a batch or finishes minimizing (Go 1.27 `internal/fuzz/fuzz.go:722-726`, `worker.go:725`), so the display read `0/sec` for long stretches.
- Second, smaller cause: two seeds were built to backtrack catastrophically (`^(a+)+$` on 40 a's plus `!`, as pcre_match and as pcre_replace). Timed at 2.09 s and 2.06 s, which is Barn's 2 s regexp2 timeout (builtins/regexcache.go:40,83). Every other seed took 0.02 s or less.
- Fixes: build the Execution once per fuzz process, outside `f.Fuzz`. That is safe because a worker runs execs one at a time and the fuzzed builtins only read the session; the one write, crypto.go:87, sets the same MaxStringConcat every time. Also removed the two timeout seeds.
- Result: 4-minute throughput rose from 15,373 execs (seeds removed only) to 899,302 execs. 34 of 83 status lines still show `0/sec`. A 60 s run with `-fuzzminimizetime=0s` showed 1 of 23, so the remaining zero lines come from Go minimizing new inputs, not from the target.
- No builtin hang was found, so there is no testdata/fuzz input and no production change.

### Toast citations re-checked (WSL /root/src/toaststunt/src), all supported, none dropped
json.cc:457-458 (int), 490-504 (list, bool), 203-229 (bool and integer parse), 351-376 (array start/end, order kept); list.cc:1485-1489 (decode_binary fully), 1549-1552 (int bytes 0..255), 1579-1583 (encode_binary); utils.cc:672-682 (escape every non-graph byte and `~`), 700-732 (decode `~XX`). Comment ranges corrected to these exact lines.

## Decisions and why
- Use newTestExecution and admitTestBuiltin, matching existing tests. Build the Execution once per process; building it per exec made the target about 60x slower.
- Each call has a ten-second watchdog that panics in a stuck fuzz worker.
- Bound each string to 8192 bytes. Preserve invalid UTF-8.
- Assert only the binary byte-list round trip and the JSON integer/boolean/list round trip, with a Toast source citation beside each assertion. All other cases assert only no panic and termination.
- No seed is built to hit the regex timeout. Each such exec costs 2 s, and coverage guidance clusters mutations around it. No `builtins` test exercises that timeout path either (searching `*_test.go` for `(a+)+`, `pcreMatchTimeout` and `catastroph` finds only this target).

## Surprises
- The coordinator decided to keep Barn's existing regex timeout as is; this task does not change engine behavior.
- Toast pcre_replace delegates to dependencies/pcrs.c rather than the pcre_match context.
- Most of the target's time went to building a registry for each exec, not to the builtins under test.

## Known related gaps
Toast bounds pcre_match with PCRE2 match/depth counters plus JIT (10,000,000 match steps and depth 10,000, `/root/src/toaststunt/src/pcre_moo.cc:51,53`; `pcre2_jit_compile` at :113), while Barn uses a two-second regexp2 timeout, so results can differ near the limit. Engine parity is separate work.

## Next step
PR #615 is open against master. CI passed on both earlier pushes, including the MOO conformance suite.

2026-11-06: rebased onto origin/master after #613 and #616-#624 merged. One conflict, in the `.github/workflows/fuzz.yml` matrix: kept master's new `./server FuzzTCPReadInput` line and added this PR's `./builtins FuzzStringBuiltins` line after it. Every other master entry, including `./db/format FuzzReadDatabase`, is unchanged. After the rebase: `go test ./builtins -run Fuzz -count=1` gives `ok ... 1.242s`, `gofmt -l builtins/fuzz_test.go` is empty, and `go vet ./builtins` is clean. Committed locally; the coordinator pushes. Left unmerged.
