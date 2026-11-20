# Bootstrap scheduling guard analysis

Scope: scripts/bench_players.py and scripts/test_bench_players.py local diff,
including actual offline-profiler/gameplay callers and current reader contract.
Read-only analysis; no implementation changes or publication.

## State

Completed parser/caller/diff review and focused probes. No implementation files
changed. No servers launched.
The advertised role protocol ~/.Codex/AGENTS.md.d/gauntlet.md is absent.

## Findings

### P2: New reader dependency has no runnable setup for existing entrypoints

scripts/bench_players.py:1026-1030 imports the reader in the current interpreter
and requires Reader.read_v17_header. Its new header says a current package is
required, but the benchmark's Usage still invokes plain python. Existing real
consumers use plain python3 as well: profile_offline_mongoose.py:5, :350;
run_mongoose_gameplay.py:450; docs/offline-mongoose-cpu-profiling.md:17;
docs/mongoose-gameplay-runner.md:27 and :112. None selects/provisions a compatible
reader environment. The installed Windows reader on this host is incompatible.

Observed ordinary-interpreter probe against the retained native fixture:

```text
RuntimeError: offline bootstrap requires current lambdamoo-db-py (Reader.read_v17_header)
```

The current reader project environment works, but that invocation/setup is not
integrated into the three entrypoints. Update the existing setup/invocations so
the required reader is available before reporting the guard usable there. Do
not weaken the scheduling check. New tests mock checkpoint_schedule at
scripts/test_bench_players.py:94 and therefore cannot detect this integration
failure or parser-contract drift.

### P2: Expected bootstrap rejection bypasses the benchmark failure record

scripts/bench_players.py:1158 calls bootstrap_control_account before constructing
record at :1166 and entering the error-reporting try at :1183. A changed schedule
now takes this expected failure path, but no run.json records source hash,
configuration or the error. bootstrap-schedule.json contains task rows only.
The other two callers place bootstrap inside their manifest-protected try and
retain failure provenance; the direct benchmark does not.

Observed probe drives main with a real temporary source/copy and mocked
bootstrap rejection, without starting any server:

```text
RuntimeError offline bootstrap changed saved task scheduling
run.json exists: False
original copy retained: True
```

Retain the same normal failure record for bootstrap errors, before any server
is constructed, while preserving both rejected and original checkpoint files.

## Verification

Only the explicitly changed test file was run:

```text
uv run --project C:/Users/Q/src/lambdamoo-db-py python scripts/test_bench_players.py
Ran 16 tests in 0.087s
OK
```

This passing output does not cover the two reproduced gaps above. The native
adapter's local Path.replace operation is compatible with its Linux paths;
no platform-specific replace failure was found. The actual reader fields used
for queued deadline, suspended deadline/readiness and interrupted status match
the current reader source. The function intentionally reads only the header,
so it is not a full checkpoint validity test.

## Scoped fix review

Reviewed only the two reported fixes. The documented Windows uv invocation and
separate Linux uv sync before network namespace / --no-sync execution address
the missing setup path. Moving bootstrap and server construction inside the
existing record/try now preserves ordinary RuntimeError failures, and the new
regression confirms server construction is skipped and run.json/source hash
are retained.

Changed-file verification:

```text
Ran 17 tests in 0.103s
OK
```

### Remaining P2: interrupted bootstrap deletes its failed input

The new cleanup condition in scripts/bench_players.py:1309 relies on exit_code
being nonzero for every failed execution. Only except Exception at :1292 sets
that state. KeyboardInterrupt and SystemExit bypass it. Bootstrap now runs
inside this finally scope, so interrupting bootstrap deletes run.db and writes
a run record with no error, whereas it previously exited before this cleanup.
This is a defect in the newly added failure-retention fix.

Exact producer probe calls main on a temporary source and mocks the running
bootstrap to raise KeyboardInterrupt, then inspects the actual written record
and retained files (no server launched):

```text
KeyboardInterrupt
run.db retained: False
recorded error: None
```

Keep interruption semantics, including propagation, but make successful input
deletion depend on actual completed success rather than the initial zero exit
status. No implementation changes made by analysis.
