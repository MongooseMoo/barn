# Fix #540: legacy queued-task activations

## Goal and state

Parent authorized #540 on base 720df1583b133633616f4458d0bdea470c4924c6.
The repair conditions the three modern activation fields at the native
version boundaries 11, 13 and 16 (five net production lines). v4/v5 numeric rows
follow the dummy value directly; v17 behavior is unchanged. The existing modern
writer/reader test now explicitly sets Version:17 instead of leaving it zero.
No new versions, parser framework or typed-identity semantics.

Production change and portable regression completed. Temporary full-fixture
experiment passed and was removed before commit. No server runs, full CI,
fixture copies or publication during this implementation.

## Source and input evidence

Fixture: C:/Users/Q/src/lambdamoo-format-oracle/LambdaCore-latest.db, commit
15bdc121a7c4bfbaa45e1a5cc7d7ed92b427e345, raw no-filters blob
261a7a73486c0fa83e503fed18e4ff026152d286, 2314940 bytes, SHA256
e453a0d4f837db9969e2fa00250c3a58aafcf0d843d96495c1dd81924ebf4747.
The original CRLF working file was different; only this committed LF input is
the fixture evidence. No external whole database or absolute-path test is committed.

Managed Toast source aecc51e9449c6e7c95272f0f044b5ba38948459e imported this input:
`test_capability_admission PASSED`, `historical_import_completes PASSED`,
`2 passed in 37.60s`. Full native evidence/provenance correction remains in
main worktree agent-notes/db-format-historical-next.md.

Native execute.cc:3838–3878 gates modern fields by DBV_This/DBV_Anon/DBV_Threaded
(declarations src/include/version.h:73–84). Both admitted legacy versions
predate those fields. Barn reader_task.go:122–139 previously read them
unconditionally; its numeric-row assignments remain unchanged.

## Focused verification

Commands ran in C:/Users/Q/code/barn-format-legacy, only actively written tests.

Before fix:
```text
go test ./db/format -run '^TestHistoricalActivationExperiment$' -count=1 -v
historical_activation_experiment_test.go:9: LoadDatabaseReadOnly pinned LF v4 fixture: read queued tasks: read queued task 0: read activation this value: parse int: strconv.Atoi: parsing "2 -7 -8 2 -9 2 88 -10 8": invalid syntax
--- FAIL: TestHistoricalActivationExperiment (0.09s)
FAIL github.com/MongooseMoo/barn/db/format 1.843s

go test ./db/format -run '^TestReadQueuedTaskActivationV4$' -count=1 -v
reader_legacy_activation_test.go:18: read activation this value: parse int: strconv.Atoi: parsing "2 -7 -8 2 -9 2 88 -10 8": invalid syntax
--- FAIL: TestReadQueuedTaskActivationV4 (0.00s)
FAIL github.com/MongooseMoo/barn/db/format 1.226s
```

After fix:
```text
go test ./db/format -run '^(TestReadQueuedTaskActivationV4|TestHistoricalActivationExperiment)$' -count=1 -v
historical_activation_experiment_test.go:11: version=4 queued=1 suspended=0
--- PASS: TestHistoricalActivationExperiment (0.05s)
=== RUN   TestReadQueuedTaskActivationV4
--- PASS: TestReadQueuedTaskActivationV4 (0.00s)
PASS
ok github.com/MongooseMoo/barn/db/format 1.155s
```

Exit 0. Full load emitted existing startup relationship-repair warnings including
`WARN #0 not in it's parent's (#1) children src=startup_repair`.
Successful loading is not evidence of graph-order preservation or task execution.
No real v5 saved-task fixture was run. Broader v17 coverage is left to PR CI.

Parent requested the existing modern writer test explicitly identify its layout.
After applying exact native version guards and setting that fixture to Version17:

```text
go test ./db/format -run '^(TestReadQueuedTaskActivationV4|TestQueuedTaskWriterReaderPreservesToastHeaderFields)$' -count=1 -v
--- PASS: TestReadQueuedTaskActivationV4 (0.00s)
--- PASS: TestQueuedTaskWriterReaderPreservesToastHeaderFields (0.00s)
PASS
ok github.com/MongooseMoo/barn/db/format 1.105s
```

Read all direct queued-reader call sites: version-specific loaders, the modern
writer/reader fixture and this new regression. The full v4 after-check above
preceded this refinement; its v4 field sequence is identical in the final guards.

The portable regression checks This2, Player2, Programmer2, VerbLoc88,
Verbdisfunc and the untouched following environment header using the exact short
producer record. Temporary experiment with the local absolute path was deleted.

## Next step

Parent reviews local commit and publishes; this agent does not push or publish.
Preserve other agents' edits.

## PR draft

Title: Fix legacy queued-task activation field alignment

Barn treated the historical activation's numeric row as a typed value and
rejected the public v4 LambdaCore dump. Skip the three modern activation fields
for supported v4/v5 layouts while retaining the existing modern reader.

The portable v4 producer-record regression checks decoded fields and the next
record boundary. The pinned LF dump now loads with one queued task through
the temporary focused experiment; its absolute-path test was removed.
Broader tests and existing v17 coverage belong to PR CI.

Closes #540.
