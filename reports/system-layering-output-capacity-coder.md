# Dynamic output capacity implementation

## Goal

Make no-argument buffered_output_length report the cached max_queued_output
option through the existing server-option owner, including task-local reloads.
Preserve signed 32-bit cache conversion and the positive minimum observed by
the parent through the managed stock oracle. No queue policy change is included.

## State

Read the existing server-option snapshot/cache and task-local publication
owners and docs/elle-isolation.md. Parsed the supplied stock-oracle XML:
capacity cases plus admission have four tests with no failures/errors/skips;
signed-width research probe plus admission has two with no failures/errors/skips.
Only assigned files are owned by this worker; other agents' edits are preserved.

Focused RED command in verified Debian WSL, from the isolated worktree:

```text
go test ./builtins -run "^TestBufferedOutputCapacity(Canonicalization|ReloadIsTaskLocalUntilCommit)$" -count=1
```

Raw RED excerpts:

```text
small_positive: buffered_output_length() = 65536, want 2048
configured: buffered_output_length() = 65536, want 4096
zero: buffered_output_length() = 65536, want 0
negative: buffered_output_length() = 65536, want -1
signed_wrap: buffered_output_length() = 65536, want -2147483648
wrap_then_minimum: buffered_output_length() = 65536, want 2048
ReloadIsTaskLocalUntilCommit: buffered_output_length() = 65536, want 3000
FAIL github.com/MongooseMoo/barn/builtins 0.185s
```

## Decisions and why

Use the existing PendingServerOptions and session serverOptionsState fields.
The task-view getter first reads pendingServerOptions and otherwise uses the
existing state RW mutex. Convert the MOO integer to signed 32 bits before the
positive minimum clamp, matching the verified stock cache. No new queue policy,
publication owner, lock or observer is introduced.

## Implementation and focused GREEN

Added MaxQueuedOutput to the existing pending snapshot and session cache,
initialized/reset/applied with the other options. collectServerOptions narrows
integer input to signed 32 bits before clamping positive values below 2048.
Wrong types and missing properties retain the default 65536. The no-argument
builtin calls the task-view getter; explicit-connection byte accounting remains
unchanged. Only the existing state mutex is used.

The identical focused selector now reports:

```text
ok github.com/MongooseMoo/barn/builtins 0.196s
```

The reload regression uses a real store snapshot and shared-session peer. It
checks that pending 4096/8192 values remain private, a round-trip to 3000 elides
the write without losing last-reload precedence, and final 7000 becomes visible
to the peer only after Commit and FlushPendingEffects. Canonicalization cases
exercise fresh defaults, missing/wrong types, minimum, zero/negative values,
signed maximum and wrap-before-clamp behavior.

No full test suite, build, managed conformance run, benchmark or Elle checker
was run locally. Focused success is not isolation or performance evidence.

## Next step

Return the five-file scoped diff to the parent. Full PR CI, managed comparison
and pinned isolation evidence remain the parent's work. No git action or
publication was performed by this worker.
