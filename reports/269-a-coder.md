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
connection state and serializes inside the barrier. Running checkpoints publish
after the barrier returns; terminal shutdown/panic checkpoints retain the barrier
through publication so pending background activations cannot run during that
publication window. Hooks still surround the complete synchronous operation and
run outside the barrier; request coalescing and panic output selection retain
their existing control flow. An ordinary dump attempt still marks output eligible
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

## Terminal publication review repair

Review found that releasing admission during a final shutdown or panic dump lets
previously dispatched background work execute after the captured terminal state.
Input.Stop closes only input admission. Explicit checkpointRunning and
checkpointTerminal modes now keep the terminal Serialize/Publish stages under
one barrier; checkpoint_finished still runs after that barrier is released.
Narrow factory injection on the actual shutdown/panic lifecycle paths preserves
the production ordinary and emergency entrypoints and lets tests hold Publish.

The terminal regression registers a MOO fork before capture, pre-dispatches its
scheduler worker inside the paused serialization phase and observes it queued at
admission, then holds real staged publication.
Shutdown, signal shutdown and panic each require it to wait in admission with
no VM probe effect or property commit. The ordinary held-publication test still
requires a runtime property commit before release. Output namespace and ordinary
dump attempt state are checked on terminal paths.

Before the mode repair, `.tmp/269-a-terminal-red.log` recorded all three paths:

```text
background activation produced an effect during terminal publication
background activation was not held at admission: queued=0
background activation committed during terminal publication: value=42 code=E_NONE
terminal_red_exit=1
```

The initial green run, `.tmp/269-a-terminal-green.log`, recorded:

```text
--- PASS: TestCheckpointPublicationAllowsRuntimeCommit
--- PASS: TestCheckpointPublicationFailureRunsFailureHookAfterResume
--- PASS: TestTerminalCheckpointPublicationHoldsBackgroundActivation
ok github.com/MongooseMoo/barn/server 3.143s
terminal_green_exit=0
```

Final lifecycle race and full focused suites after the production repair:

```text
go test -race ./server -run 'Checkpoint|Panic|Shutdown' -count=1 -timeout=240s
ok github.com/MongooseMoo/barn/server 5.743s
terminal_race_exit=0
go test ./server ./db/format ./engine -count=1 -timeout=240s
ok github.com/MongooseMoo/barn/server 9.866s
ok github.com/MongooseMoo/barn/db/format 5.574s
ok github.com/MongooseMoo/barn/engine 24.662s
terminal_suites_exit=0
```

The final pre-dispatched worker regression and terminal failure-hook/resource
release regression also passed under race, after the last test edits:

```text
go test -race ./server -run 'TestTerminalCheckpoint|TestCheckpointPublication' -count=1 -v -timeout=60s
--- PASS: TestTerminalCheckpointPublicationHoldsBackgroundActivation
--- PASS: TestTerminalCheckpointPublicationFailureReleasesBarrierBeforeFailureHook
ok github.com/MongooseMoo/barn/server 4.141s
terminal_predispatch_race_exit=0
```

Raw outputs are `.tmp/269-a-terminal-race.log`,
`.tmp/269-a-terminal-suites.log` and
`.tmp/269-a-terminal-predispatch-race.log`. The failure-hook test verifies native
publication error propagation, failure hook execution after releasing the
terminal barrier, aborted-stage removal and a successful fresh checkpoint that
reacquires both runtime admission and the output path.
