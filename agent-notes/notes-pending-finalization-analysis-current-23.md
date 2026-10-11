# Pending-finalization analysis checkpoint

- Scope: independently analyze Barn head `8b774ce65dae7567e254a367040402713bfd3863` in `C:\Users\Q\code\.worktrees\barn-pr96-pending-roots-repair`.
- Current state: exact local head `8b774ce65dae7567e254a367040402713bfd3863` was reviewed without code changes. Live PR #96 remains a draft at old remote head `0450cf22c84d0c3ebdb96a9172b6072db1a7f56a` with its old failing conformance check.
- Required checks: shutdown overtaking a claimed GC sweep, concurrent flushes, roots arriving during handoff, ordinary generic cancellation, VM handoff for running/not-running/panic, explicit shutdown state, direct roots versus nested reachability, canonical `*task.Task`, and canonical `anonObjects` persistence.
- Constraints: current code and live issue #95 / PR #96 only; do not read historical Markdown; use managed conformance only; report exact-head MERGE or NO-MERGE and note the PR remote head remains older.
- Verification: focused packages passed; focused race passed; the three shutdown boundary tests passed 50 repetitions; `go test ./... -count=1`, `go build ./...`, and `git diff --check` passed.
- Blocker: a claimed deferred-GC sweep executes MOO `:recycle` while `gcRunning` is true, but a `shutdown()` invoked by that callback re-enters `BeginShutdown` and waits for `gcRunning` to clear. The sweep cannot clear it until the callback returns, producing a self-deadlock.
- Blocker: panic checkpoints begin immediately after handing off only the caller VM. Other concurrently running task VMs are neither stopped nor captured before the one and only emergency checkpoint, so anonymous roots can be omitted and appended only after the checkpoint has already been written.
- Major: pending-root normalization recursively promotes a WAIF nested under another pending WAIF into a second top-level pending-finalization root, rather than treating it as reachability beneath the direct root.
- Coverage gap: no deterministic test exercises two concurrent flush callers or a producer adding roots while the shutdown sink is executing; the host callback tests also never run a live server with concurrent tasks during panic.
- Verdict: `NO-MERGE` for exact head `8b774ce`.
- Next action: return these findings to the parent; implementation must add red tests for the deadlock, panic concurrency, nested-WAIF topology, concurrent flushes, and roots arriving during sink handoff before another analyst pass.
