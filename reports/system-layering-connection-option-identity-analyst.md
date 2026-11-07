# Connection option identity analyst

## Scope and state

Read-only source analysis of the current physical-ID option-store change against
origin/master. No implementation changes, tests, builds or publication. This is
source-only analysis during the parent's serialized measurement windows.

## Findings

### Historical P2: an admitted setter could recreate a closed physical connection's bag

This finding describes the first reviewed identity implementation. The selected
lifetime correction and its RED/GREEN evidence are reviewed in the delta section
below; the original interleaving is retained here as the defect record.

builtinSetConnectionOption resolves a real Connection at network.go:1300 and
then validates its option/value, including iterating an intrinsic-commands list,
before Session.setConnectionOption at :1347. Resolution releases manager.mu
(server/connection_manager.go:519-541). The setter then lazily creates a missing
byConnection entry while holding only the Session option mutex (:214-225).

Meanwhile processDisconnect removes the socket from both manager maps under
manager.mu, unlocks, and calls ForgetConnectionOptions
(server/input_processor.go:539-563). There is no shared lifetime check between
Forget and lazy creation. This interleaving is permitted:

1. A wizard task on a different connection, or Eval, resolves target connection C.
2. That task is preempted or validating its intrinsic-command list.
3. Target C's disconnect lane removes C and deletes its physical-ID option bag.
4. The admitted setter resumes and recreates byConnection[C.ID].

The recreated bag has no future disconnect event to delete it. Connection IDs
increase for every new physical socket (connection_manager.go:454-455), so
repeating the interleaving across reconnects retains new unreachable bags rather
than one reusable player-keyed entry. This is a cleanup/resource defect; this
analysis does not assert a particular MOO return value for the race.

The per-connection input lanes do not serialize this case: cross-connection
events run concurrently (input_processor.go:290-345). Disconnect dispatch calls
processDisconnect directly (:366-368), and its cleanup precedes any user-hook
runtime call. set_connection_option's irreversible gate serializes VM effects,
not that Go manager/Session cleanup. No runtime witness has been run here.

Smallest deterministic real-producer RED: wrap the existing real manager by
embedding/delegating it, override GetConnection to capture the actual returned
Connection and pause before returning it once. Invoke the actual admitted
set_connection_option for that target in a goroutine. After capture, process the
real target disconnect through InputProcessor, release the wrapper, join the
setter, then assert the closed ID is absent from Session storage. Register all
release/join cleanup first. Do not copy manager resolution or production setter
logic into the test. This scheduling seam models a preemption after resolution.
Use the existing server fixture and suppress this target's disconnect hook before
starting: the setter is paused after its irreversible admission, so an unrelated
user-hook runtime call must not be allowed to wait for that task's gate and hide
the cleanup observation. Set binary to one and require the already-admitted
setter's normal zero return after release; its positive option read after target
removal currently exposes recreation. Register idempotent wrapper release and
the setter join before launching it. Closing/removing a target on its own input
lane while a wizard task owns another lane is the actual production schedule.

A cause-level correction must coordinate lazy mutation with physical lifetime.
An existing lifetime signal is Connection.ctx.Done. Marking that signal closed
before Forget and checking it under the existing Session option mutex during
mutation can order both races without tombstones, another cache or another
mutex: a mutation preceding cancellation is subsequently deleted; a mutation
following cancellation does not recreate the bag. Exposing this signal only to
mutation keeps hot option reads ID-only and out of manager/output locks. This is
a design direction for parent selection, not an implementation or an earned
runtime verdict. Preserve admitted setter return semantics until evidence earns
a different response. Session.Close/post-close host concurrency is a separate
existing lifecycle question; do not infer it solved by per-socket cleanup.

The required ordering is explicit: capture immutable ID/lifetime channel; acquire
the existing option mutex; check the lifetime signal; create/update only if it
is still open. Physical disconnect closes that signal before taking the option
mutex to Forget. If the setter's open check wins, it holds the mutex through
mutation and Forget deletes afterward. If cancellation/Forget win, the setter
observes the closed channel under the mutex and performs no creation. A default
select arm cannot bypass an already-ready closed channel. Cancellation between
the check and mutation is covered by the subsequent locked delete. No second
lock, tombstone or retained closed-ID registry is needed. Connection.Close and
closeForShutdown already cancel before transport closure (connection.go:197-208);
natural input disconnect needs its cancellation ordered before Forget too.

### Actual Session.Close producers

The production Session is constructed by engine.NewRuntimeWithRegistry from a
Host initially containing TaskManager, without ConnManager (runtime.go:105,194).
The server wires the real ConnManager at server.go:203-206. Normal server
shutdown closes listeners and connections before stopping input, takes its
checkpoint, stops the runtime, joins background work, then closes Session
(server.go:572-595). CloseConnections cancels every physical connection through
closeForShutdown and joins writers/handlers (connection_manager.go:198-249).
The proposed per-socket lifetime check therefore also prevents a captured setter
for those closed sockets from recreating options after that normal cleanup.

Listener-reservation failure stops input/runtime and joins background work before
Session.Close (server.go:319-331); accepted client commands have not started at
that point. Restored tasks can be scheduled by input.Start, but scheduler.Stop
joins its workers (scheduler.go:71-84) before Session.Close. This is not proof
of complete startup transport cleanup, and that separate owner question is not
changed here.

The other production direct caller is EvalSession.Close: Runtime.Stop then
Session.Close (internal/dbtool/dbtool.go:225-227). NewEvalSession constructs the
runtime without installing a ConnManager (:214-222), so real connection-option
admission has no socket source there. EvalExpression, EvalFile and cmd/moo defer
that wrapper's Close, rather than creating another live socket owner.

Session.Close itself resets the option map under the same mutex (:130-133); it
does not mark a Session globally closed. This source audit earns no new guarantee
for arbitrary embedded hosts invoking Session.Close concurrently while leaving
their sockets live or submitting more executions. Do not introduce a Session
closed flag or broaden lifetime semantics from this per-socket finding.

### Inspected identity/data flow and coverage

The stable-ID reads are source-consistent: setter, both option getters, notify,
read admission and read_http use their already-resolved actual Connection.
InputProcessor uses input.ConnID, the current reader's conn.ID, and ForceInput's
single real manager lookup. No production cast from player to physical ID was
introduced. The existing synthetic negative ForceInput queue-lane fallback is
after option reads and is not used as physical option identity.

Held command and HTTP maps stay keyed by player. HandleHeldInput,
collectHTTPWakeupsLocked and prepareHTTPRead accept the existing producer ID;
only cold cancellation resolves the current manager mapping. New tests use
player 7 with physical ID 91, and cancellation uses replacement ID 92 before any
new producer runs. Existing raw store/parser fixtures sometimes deliberately
use equal numeric ID/player values through explicit casts; they are not proof
of producer identity. The distinct-ID tests and real server switch/reconnect
tests supply that distinction. Engine fake connections have explicit IDs;
benchmark connections get unique per-manager indices.

Stale physical disconnect deletes its own ID and preserves replacement options.
Existing replacementActive logic still protects player-owned HTTP state. No
option-bag copy or held-buffer migration was introduced. Same-task deferred
boot/switch visibility and deferred notification recipient selection remain
unchanged and outside this fix.

### Lock and error audit

Option getters copy values/maps under the same existing RWMutex. The setter
releases that mutex before waking input/draining held commands. Disconnect
releases manager.mu before Forget and before closing held HTTP state.
Ordinary hot input/HTTP paths have no manager lookup while holding HTTP state;
cold cancellation retains HTTP -> manager -> option-read ordering. Manager
replacement publishes/releases manager.mu before enqueueing callbacks, and no
inverse Session call under manager.mu was found in the inspected owners.
ConnectionID reads the immutable ID without metadata/output locking. Getter
permission and nil/argument error order are preserved in the inspected delta.

## Known evidence limits

The coder records focused correctness output and controlled residual costs:
default input 39.83 to 41.55 ns and stored notify 130.0 to 135.2 ns. Allocations
are unchanged. This analysis does not clear performance neutrality or PR CI.

## Lifetime correction delta review

No new actionable correctness defect found in this delta. The original P2 is
addressed at its cause: mutation and physical deletion now share the existing
option mutex and the real connection's monotonic cancellation signal.

The new Connection.Done method returns its existing ctx.Done channel; it adds
no independent lifetime, registry, lock or task state. The sole production
setConnectionOption caller passes conn.ConnectionID and conn.Done from the same
admitted actual Connection. The setter checks the channel while holding the
existing option mutex and returns false before creating or modifying a bag
when cancelled. Natural processDisconnect invokes conn.cancel after manager
unlock and before Forget; ordinary Close and shutdown already cancel first.
The option getter/input/HTTP hot paths do not query Done or acquire new locks.

Both orderings are covered by the source: a mutation winning the open check
holds the mutex through its write, so the subsequent Forget deletes it. If
cancel/Forget win first, the next locked check sees closed and cannot recreate
the bag. Cancellation after an open check but before assignment is also deleted
by Forget after that mutex releases. Connection cancellation itself takes no
manager/output/option lock, so this ordering adds no lock cycle.

The builtin returns normal integer zero when the already-admitted mutation is
rejected. That early return also prevents binary WakeInputReader and held-command
draining/ForceInput for the rejected write. Validation and target/permission
admission retain their prior order. A mutation accepted before cancellation still
uses the existing ancillary-action placement outside the option mutex; this
delta does not claim those actions form a new atomic operation with disconnect,
replacement or player-owned task queues.

The new server regression delegates its lookup to the real manager, captures the
actual Connection, pauses after resolution, processes a real disconnect, then
releases and joins the admitted runtime Eval. Its target's disconnect hook is
suppressed to avoid a runtime-gate dependency hiding the cleanup. Idempotent
release/join cleanup is registered before launch. Outcome reads follow the
finished channel, and captured identity is asserted. The recorded RED and GREEN
are for the actual setter producer, without copied resolution or mutation code.

Inspected raw artifacts:

```text
system-layering-option-identity-stale-setter-red.txt:
admitted setter recreated the disconnected physical connection's binary option bag
FAIL github.com/MongooseMoo/barn/server 1.062s

system-layering-option-identity-lifetime-server-green.txt:
TestAdmittedOptionSetterCannotRecreateDisconnectedBag PASS
ok github.com/MongooseMoo/barn/server 1.553s

system-layering-option-identity-lifetime-builtins-green.txt:
TestConnectionOptionLifetimeOrdersMutationAndForget PASS
TestClosedOptionMutationSkipsWakeAndHeldDrain PASS
ok github.com/MongooseMoo/barn/builtins 1.161s

system-layering-option-identity-lifetime-engine-green.txt:
TestRunTaskReleasesSnapshotBeforeBlockedCommittedOutput PASS
ok github.com/MongooseMoo/barn/engine 1.425s
```

The direct lifetime test covers a live mutation preceding cancellation/deletion,
rejection without changing an existing closed bag, and rejection after Forget.
The closed-builtin test asserts normal zero returns, no bag recreation, no input
wake, no forced input, and preservation of the already-held command. Raw fixtures
and recording fakes explicitly use nil lifetime channels to model permanently
live fixture stores; the real production caller supplies the real Done signal.

This was source and artifact review only: no local Go command, race detector,
build or timing run was performed by this analyst. The earlier controlled timing
pair predates this cold setter/lifetime correction. The exact final binary has
not been measured; the old figures cannot establish final performance neutrality.

## Remaining

The original cleanup finding is source-corrected with inspected real-producer
RED/GREEN evidence. Required PR checks and verifier review remain unperformed by
this analyst. No merge verdict or performance-neutral claim is issued here.
