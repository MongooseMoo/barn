# Barn support for MCT callback coverage

## Goal

Q requested an isolated Barn worktree, production fixes sufficient to pass all
coverage in moo-conformance-tests PRs #162 and #163, a Barn PR, then normal
merges of the MCT PRs. Q subsequently authorized handling #163's controller
refresh as well: "do all the work".

## State

Barn worktree: C:/Users/Q/code/barn-mct-callbacks, branch fix/mct-callbacks.
MCT integration worktree: C:/Users/Q/code/mct-barn-callbacks, branch
integration/barn-callbacks, containing current main and both test PRs unchanged.
The integration branch is published for the existing paired-candidate workflow.
Baseline paired run 40444307193 completed: 114 failures, with raw XML and
admission/inventory evidence under .tmp/mct-baseline-40444307193. Its three
Toast profiles succeeded. Barn PR #552 contains system hook context, startup,
lag, error and input callback changes. Paired run 40445777195 tests the earlier
callback changes; it completed with 91 failures. The subsequent paired run
40448717670 tests the context, initializer, setter and builtin-marker fixes.
Login, eval-player and deferred-finalizer fixes are pushed; their unit and Elle
jobs passed. Explicit recycle-exit changes are pushed; staticcheck caught an
unused initial assignment, now removed. Plain-eval frames retained
player #-1 while their execution context had the task owner; initializing the
frame player from that context repaired fork ownership. Compiler-local
reservation was replaced by frame state without changing source variable limits.
Its first unit CI failed because TestCheckpointedConnectionsDisconnectBeforeServerStarted
expected player #0 instead of the source-confirmed #-1. Corrected that existing
expectation without changing its ordering assertion.

MCT #161 merged. #163's previous controller validation rejected restart
arguments; GitHub accepted an update-branch request to include current main.
Its original worktree contains someone else's uncommitted note edits, preserved.
#162's previously reported Full suite check passed. MCT merges remain dependent
on Barn support and successful required checks.

## Decisions and why

Use the existing paired-candidate CI workflow to run the combined candidate
suite, preserve raw evidence, and avoid duplicating CI locally. Do not change or
skip the new tests to hide Barn failures. Reuse existing server task dispatch,
VM callbacks and checkpoint machinery for implementation.

Deferred-finalizer slice (about 70 added lines, deleting the separate WAIF
runner): extend the existing sweep-owned standalone verb runner to retain its
VM on suspension in a registered task. Use the existing sweep barrier for its
first execution and ordinary scheduler ownership thereafter. Accept a typed
receiver so WAIF and anonymous hooks share this path, with player/caller #-1.
Toast server.cc:617-686 runs these hooks as root server tasks. Retain the current
anonymous cleanup and frozen-candidate ordering after the initial hook slice.
Claim each WAIF finalizer once on its existing identity: the suspended callback
retains this and later releases it back into the GC queue. This mirrors Toast's
destroyed_waifs boolean and avoids retaining an ever-growing runtime identity map.
The native WAIF checkpoint format does not save that boolean (waif.cc:858-920);
keep its existing portable format.

Explicit recycle exit slice (about 100 lines): move one current content or the
object itself to nothing, then push exitfunc on the existing recycle continuation.
Re-read contents/location after each return, as Toast objects.cc:856-873 does,
so a callback that moves objects is respected. Preserve the continuation through
the existing snapshot metadata; retain the existing initial-hook error handling.

Smallest-change plan is being established from the test inventory and source;
line estimates must follow the actual failures, not an invented feature design.
Initial source search finds no start-line/start-script or handle_lagging_task
implementation. Read Toast implementations before designing these additions.
No new compatibility protocol, feature switches or replacement checkpoint
storage are planned.

First implementation slice: fix three system-hook player arguments to #-1,
remove local-only lookup gates on those hooks, and use existing callable-verb
resolution for server task dispatch (roughly 20 changed lines). Toast
server.cc:794 and tasks.cc:1861 establish the contract; Barn server.go:619,
:628 and :637 currently supply #0, and task_factory.go:99 uses FindVerb.
The new MCT callback tests are the regression coverage. Further changes follow
the initial paired CI failure inventory; normal local CI gates are not run.

Startup support plan (about 80 production lines): parse the two requested
CLI options, keep the last value of each and their Toast ordering, read files
as newline-delimited lines, preserve blank lines, clean nonprintable bytes,
then dispatch do_start_script through existing server tasks before startup
notifications. Toast server.cc:1273-1334, :2019-2032, :2318-2328 and
utils.cc:637-648 are the source. No evaluation of the supplied text by Barn.

Lag callback plan (about 90 lines): retain only entry verb metadata, reuse the
existing measured VM-attempt duration, read the live float task_lag_threshold
(default 5 seconds, disabled below 0.1), and dispatch the existing server task
after successful publication and before error/suspend handling. Completed
payloads use Toast's partial six-field frame; aborted/suspended payloads use
the existing captured activation stack. Source: execute.cc:3108-3147,
server.cc:530-541 and :1432-1440; options.h:133. No timing cache or new queue.

Error/context repair plan (roughly 70 changed production lines plus signature
updates): root server callbacks report caller_perms() as #-1; retain custom
raise messages through the existing formatter/output owner; aborted error
handlers print their own traceback then the original, and must not count as
truthy success. Timeout dispatch uses the existing resumable server-task path
and the captured exception stack. Toast execute.cc:535-565, :580-601,
:3162-3186 and :3721-3731 establish these semantics. Existing callers are
updated directly; no compatibility wrappers or alternate error pipeline.

Input callback slice (about 25 changed lines): route out-of-band and trusted
blank hooks through existing resumable server-task dispatch and use callable
lookup for login tasks. Toast tasks.cc:895-915 and :970-974 use the same server
task entrypoint for these hooks. Preserve missing-hook fallthrough; let the
runtime own exception output instead of printing it twice.

Nested context slice: copy the caller's current values at each verb call.
The initial compiler-local representation failed CI's variable-name and slot
boundary tests. Remove that representation: keep values absent from source
locals in six frame fields, carry them through existing snapshot metadata and
fork environments, and include them in existing GC traversal (about 120 lines).
This preserves all source-variable limits and assertions. Preserve wizard-only object-valued player
override and strip the WAIF prefix from the local verb name while retaining it
in activation metadata. Source: Toast execute.cc:754-778. Initialize the command
locals at root VM entry; eval still starts with blank command context.

Open scope decision: MCT checkpoint_fault hardcodes Test.db.new.#1# and its
YAML requires Toast diagnostics, whereas Barn checkpoint_stage.go:32 stages
Test.db.tmp. Asked Q whether to make the fixture portable without removing
failure/context/recovery assertions. Do not change checkpoint code or those
MCT assertions while that question is pending; independent callbacks continue.

Initializer slice (about 40 lines): use the existing FlowBuiltinPush path,
retain the created object on the caller's operand stack and discard only the
initializer's normal result. Native errors propagate; missing/non-executable
initializers return the created object. This keeps initialization on the caller
VM across suspension without introducing continuation state. Source: Toast
objects.cc:430-457 and :520-540. Builtin caller-stack markers remain a separate
missing part of the callback context contract.

WAIF setter slice (about 55 lines): share class-owner/callable validation with
reads, map absent setters to E_TYPE, and save the destination local on the
callback frame so normal return replaces the indexed container. Keep the RHS
expression result already on the operand stack. Persist the destination through
the existing VM metadata for suspension/checkpoint. Source: execute.cc:1183-1213.

Builtin stack slice (about 90 lines): record the invoking builtin name and
continuation PC on callback activations. Expand those into observation-only
frames for callers(), task_stack() and exception payloads; permission lookup
continues to use actual verb activations. Preserve the metadata on checkpoint.
Toast execute.cc:452-516 and objects.cc call-pack PCs establish the shape.
Expand the same markers when lag, timeout or uncaught handlers use the live
task stack fallback (three call sites); VM exception snapshots already include
them. This preserves builtin callback frames during suspension and exhaustion.

Initializer CI exposed an obsolete fixture assumption: the inline-threaded-sort
test used create() to enter a throwaway VM. Initialization now correctly shares
the resumable caller. The fixture instead enters through Runtime.CallVerb,
the actual synchronous host path; its return-flow, valid-object and sorted-value
assertions are unchanged. Ordinary initializer suspension is covered by MCT.

Paired run 40445777195 completed with 91 failures; evidence is retained under
.tmp/mct-candidate-40445777195. Native range-error output regressed because
ErrorMessage exposed VM diagnostics. Removing all detail then caused four
main-suite failures for source-defined type, property and variable messages.
Mark those producers explicitly (about 25 lines), preserving their lazy format
while native diagnostics use the canonical error text. Toast execute.cc:919-960
distinguishes PUSH_X_NOT_FOUND/type mismatch messages from PUSH_ERROR.
Explicit raise() payloads remain intact.
Fork callback payloads also fail their originating frame assertion: fork
capture used Context.ThisValue (integer zero at a normal root) rather than
the active frame's None/typed receiver. Copy the activation fields directly.

Login slice (about 25 production lines): resolve inherited callable login
handlers and accept returned player objects only before any suspension, using
the task's existing suspension generation. A resumed login must use
switch_player explicitly (Toast tasks.cc:884-921); update read-login fixture
verbs to do that without changing their connection, ordering or orphan-task
assertions. Preserve the existing single-login-task safeguard; reading tasks
still consume their input through the existing earlier path.

## Surprises

#163 was not green when requested: trusted validation reported
"Unknown field in test #1 step #3 restart_server: arguments" before #161 merged.
The existing paired-candidate workflow can test both repositories' immutable
candidates before MCT merges.

## Next step

Connection repair (about 80 lines): enqueue switch notifications on the existing
connection input lane, filter switch recipients by the user flag, and distinguish
boot/recycle/timeout closes from client closes using the connection's existing
mutex. Close a superseded switch target without a second disconnect notification.
Toast server.cc:917-922 and :1729-1768 define these notifications. Prelogin
disconnects use the negative connection id. No new queue or worker is needed.

Released-reference repair (about 45 lines): retain overwritten property values
in the VM's existing pending finalization roots and include those anonymous ids
in the existing deferred sweep floor. Reuse the pending-WAIF path and retain
the existing store/VM reachability checks before running any finalizer.

Read the callback source and full changed suites while paired CI establishes
Toast acceptance and Barn failures. Then implement only the demonstrated gaps,
publish the Barn PR, repair its CI findings, and merge the MCT PRs after proof.
