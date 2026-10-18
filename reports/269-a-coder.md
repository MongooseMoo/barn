# Issue 269: publication after runtime admission resumes

Slice A keeps one synchronous checkpoint lifecycle and the existing snapshot
implementation. `format.BeginCheckpoint` / `BeginPanicCheckpoint` acquire the
output path and recover interrupted publication before creating staging files.
`Checkpoint.Serialize` consumes the protected store/task/connection state and
flushes the portable writer. `Checkpoint.Publish` performs file sync/close,
identity sidecar/hash work, recovery journaling and publication. `Close` releases
the path and removes aborted staging, while preserving recovery evidence after
an attempted pair publication. Close errors are joined into caller errors.

`Server.dumpCheckpoint` begins staging before the barrier, captures task and
connection state and serializes inside the barrier, then publishes after the
barrier returns. Hooks still surround the complete synchronous operation;
shutdown, request coalescing and panic output selection retain their existing
control flow. An ordinary dump attempt still marks ordinary output eligible
even when staging or publication fails.

## Regression and verification evidence

The new runtime commit regression failed before the implementation. Raw evidence
is retained in `.tmp/269-a-red.log`:

```text
=== RUN   TestCheckpointPublicationAllowsRuntimeCommit
input_admission_test.go:163: runtime command could not commit during checkpoint publication
--- FAIL: TestCheckpointPublicationAllowsRuntimeCommit (1.01s)
test_exit=1
```

Its final form holds Publish on a real staged checkpoint, executes a VM property
write through runtime admission and verifies the committed value while Publish
is still held. Reload verifies the earlier captured value. A second test proves
publication failure after runtime resume invokes checkpoint_finished(false).
The preexisting task/store-capture regression now holds Serialize and still
requires the VM to queue. New format tests cover aborts after begin, invalid
store, writer failure, successful serialization, sync failure and sidecar
creation failure; stage ordering/idempotent Close; and preserved saved recovery
pairs when publication and rollback both fail.

Full targeted package suites, `.tmp/269-a-suites.log`:

```text
go test ./db/format ./server ./engine -count=1 -timeout=240s
ok github.com/MongooseMoo/barn/db/format 6.848s
ok github.com/MongooseMoo/barn/server 9.980s
ok github.com/MongooseMoo/barn/engine 27.361s
test_exit=0
```

Final checkpoint/panic race tests after all test edits,
`.tmp/269-a-race-final.log`:

```text
go test -race ./server ./db/format -run 'Checkpoint|Panic' -count=1 -timeout=240s
ok github.com/MongooseMoo/barn/server 5.025s
ok github.com/MongooseMoo/barn/db/format 4.879s
race_exit=0
diff_check_exit=0
```

This slice does not move the numbered-object walk or serialization outside the
barrier. It establishes no measured latency/memory, managed conformance or Elle
isolation claim. Slice B must introduce coordinated store capture before moving
those stages, using this same publication lifecycle.
