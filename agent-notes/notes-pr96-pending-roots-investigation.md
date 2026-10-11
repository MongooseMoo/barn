# PR 96 pending-root repair investigation

## State

- Investigation target is the read-only worktree at exact HEAD `25d689eeb6817970f0332c3e1ee5db61f03af4c5`.
- No edits, commits, pushes, or historical Markdown reads have been made in that worktree.

## Current findings

- Graceful `shutdown()` invokes the registered host hook while `ctx.CallerVM` still names the calling VM. The hook appends `vm.CollectPendingFinalizationValues(store, callerVM)` before it cancels the server context.
- The collector scans VM frames, operand stack, pending waifs, yield/fork values, and several `exec.Context` fields. It scans `exec.Context.TaskLocal`.
- The `task_local()` and `set_task_local()` builtins read and write the owning `*task.Task` via `ctx.Task`; task snapshots also persist `task.TaskLocal`. No observed synchronization mirrors that value into `TaskContext.TaskLocal`. This explains missing anonymous roots held only by real task-local storage.
- The store keeps loaded and newly appended pending values as arbitrary `types.Value`, but snapshot normalization recursively extracts only anonymous-object references. Pending WAIF values therefore produce an empty anonymous candidate set and disappear from the emitted pending-finalization section.
- Anonymous candidates are filtered against persistent reachability, expanded through the frozen anonymous graph, sorted by decreasing closure size then identity, and reduced to one deterministic bare ANON root per covered component/cycle. The serialization plan is seeded from persistent ANON roots plus these normalized ANON roots.

## Competing hypotheses

1. The startup-repair failures are caused by two independent root-domain losses: task-local ownership mismatch at shutdown and ANON-only snapshot normalization dropping WAIF pending values.
2. Shutdown/checkpoint ordering races the task completion path and loses roots after collection.
3. Serializer format rejects composite or WAIF pending values, forcing the ANON-only normalization.

Current source directly supports hypothesis 1. Further tracing is needed to rule out 2 and determine the real serializer contract for 3.

## Blocker

- None yet. Runtime proof will use only the documented managed conformance command if static source/tests are insufficient.

## Next action

- Trace the exact shutdown-to-checkpoint ordering, value writer/reader WAIF support, startup repair consumption, anonymous/waif reachability domains, and existing tests. Then specify the smallest coherent architecture and exact red tests without implementing.
