# Notify Toast parity

## Goal

Prove notify return values and programmer permissions on managed WSL Toast;
add durable conformance coverage, fix demonstrated Barn differences, and match
offline disconnected-target success without inventing an output mechanism.
Open separate Barn and conformance PRs, watch their checks, and leave both open.
Barn must merge before conformance when merging is later authorized.

## State

Created isolated branches `fix/notify-toast-parity` in
`C:/Users/Q/code/barn-notify-toast-parity` and `tests/notify-toast-parity` in
`C:/Users/Q/code/moo-conformance-notify-toast-parity`, from fetched remote
default branches. Primary checkout changes are outside this task.

Read global, Barn, and conformance repository rules and the managed workflow.
Read Toast server.cc bf_notify/emergency_notify and network.cc enqueue_output.
The stock oracle source matches the documented pin. Three managed Toast runs
passed the five new cases and capability admission. Unchanged Barn failed the
two connected-success returns and both other-target permission cases, while
wizard disconnected success and admission passed. The smallest production
change adds programmer authorization before connection resolution, removes the
no-manager early error, and returns 1 after direct/deferred success.
Go regressions cover direct/deferred permissions and offline eval. Existing
deferred-output fixtures now set their programmer to their intended self target.

Source observations, now behaviorally confirmed for normal sends and permissions: Toast bf_notify rejects a
non-wizard whose programmer differs from the target, and returns enqueue_output's
result for connected targets. enqueue_output returns 1 after normal enqueue and
0 if its overflow remains after push_output and flushing is disallowed. Its
disconnected branch returns 1 and prints through emergency_notify only in
emergency mode. The harness has named secondary socket connections; sends read
until quiet, but an idle secondary connection can remain unread while a primary
eval sends output. It has no queue-capacity control; deterministic full-queue
no_flush coverage was not established and is excluded. No queue behavior changes.
The offline EvalSession owns result writers only at Print; its runtime is
constructed without a notification output writer. No emergency_notify equivalent
was found in the inspected dbtool, moo command, or builtin Host owners.

### Evidence

Fixture: conformance worktree `src/moo_conformance/_db/Test.db`, 2035 bytes,
SHA-256 `1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
Each managed lifecycle uses a disposable copy.

WSL Debian, conformance worktree, environment
`UV_PROJECT_ENVIRONMENT=/root/.cache/uv/notify-toast-parity`:
`uv run moo-conformance _tests/builtins/notify_semantics.yaml --server-command="/root/src/toaststunt/build-release/moo {db} {db}.new -p {port}" -k "notify_semantics or capability_admission" -v --junitxml=/tmp/notify-toast-N.xml`.
Run 1 omitted the file selector and collected all suites before filtering.
Raw summaries, retained in `/tmp/notify-toast-{1,2,3}.xml` in Debian:

```text
6 passed, 13754 deselected, 1 warning in 257.57s (0:04:17)
6 passed, 1 warning in 6.97s
6 passed, 1 warning in 6.60s
```

Windows conformance worktree, environment
`UV_PROJECT_ENVIRONMENT=C:/Users/Q/.cache/uv/notify-toast-parity-windows`:
`uv run moo-conformance _tests/builtins/notify_semantics.yaml --server-command='C:/Users/Q/code/barn-notify-toast-parity/bin/barn.exe -db {db} -port {port}' -k 'notify_semantics or capability_admission' -v --junitxml=C:/Users/Q/AppData/Local/Temp/notify-barn-before.xml`.
Raw unchanged Barn summary:

```text
expected value 1, but got 0
expected value [1, 0, 1], but got [1, 0, 0]
expected error E_PERM, but got success with value: 0
expected error E_PERM, but got success with value: 1
4 failed, 2 passed, 1 warning in 2.91s
```

After the fix, same command with `--frozen`, both notify_semantics.yaml and
notify_call_shapes.yaml selectors, `-k 'notify or capability_admission'`, and
`notify-barn-after.xml`: `61 passed, 1 warning in 2.59s`.
Warnings are pytest's record_property/xunit2 compatibility warning.

Focused Go commands:
`go test ./builtins -run 'TestNotify|TestDiscardPendingNotificationsDropsDeferredNotify|TestBufferedOutputLengthIncludesPendingNotifications' -count=1`
and `go test ./internal/dbtool -run '^TestEvalExpressionNotifyWithoutConnectionManager$' -count=1`:

```text
ok github.com/MongooseMoo/barn/builtins 1.234s
ok github.com/MongooseMoo/barn/internal/dbtool 1.591s
```

Offline ownership: `internal/dbtool/dbtool.go` NewEvalSession constructs the
engine runtime; engine/runtime.go's host has TaskManager but no ConnManager;
EvalSession.Print accepts writers only for result/diagnostics. No notification
sink exists to reuse. Thus no-manager success changes the offline tool path;
server disconnected success already returned 1. Permissions apply to both.

## Decisions and why

- Q's continuation explicitly authorizes execution of this plan. Q's original
  words were "figure what the actual problem is" and "those should codex
  agents". Q did not type "make it so"; the coordinating Stop hook ruled those
  words were the go-ahead and stopping to ask again was wrong. Proceed under
  this explicit continuation; preserve the no-merge and no-comments holds.
- Use bundled Test.db and the managed lifecycle; include capability_admission.
- First add connected normal-send, non-wizard other-target, and self-target
  cases. Repeat unchanged cases on Toast at least three times, then run on
  unchanged Barn before fixing any demonstrated difference.
- Inspect harness support for controlling output pressure before deciding
  whether a deterministic full-queue no_flush case is possible. Do not make a
  timing-dependent test or change harness transport for this task.
- Inspect offline tool owners for an existing notification output mechanism;
  use one only if it exists. Add Go regression coverage for offline return and
  permissions and for demonstrated server differences.
- Stage only named task paths. No merge, comments, issues, or attribution.

Intended paths: conformance
`src/moo_conformance/_tests/builtins/notify_semantics.yaml` and its branch task
note; Barn `builtins/network.go`, `builtins/network_test.go`,
`internal/dbtool/eval_test.go`, and this note. Any additional path needs a
demonstrated producer or check failure before widening this list.

## Surprises

The earlier authorization hold is superseded by Q's explicit continuation above.
Windows uv 0.13.1 imported global exclude-newer settings into uv.lock, which
WSL uv 0.9.4 rejected before an optional fourth oracle run. Restored only the
incidental lock delta; subsequent runs use --frozen. Three oracle runs had
already passed. No dependency change is included.

## Next step

Open separate Barn and conformance PRs, watch exact-head checks to green, and
record their links. Leave both unmerged. Full-queue no_flush and emergency
notification rendering remain unverified; no rendering mechanism was added.
