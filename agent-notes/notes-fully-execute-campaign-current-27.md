# Current checkpoint 27

- PR #45 hardening remains isolated in `C:\Users\Q\code\.worktrees\moo-conformance-shutdown-gc-edges`; no Barn changes.
- Exact panic dump inspected under the managed WSL Toast harness: suspended `state` serializes WAIF class `#9`, owner `#8`, then anonymous `#10`, owner `#8` in the first direct seed. Final recipe ownership is being made static (`#3`) for deterministic evidence.
- Added three checked-in startup recipe database inputs and three provenance suites. The provenance suites run pinned Toast without requiring a port handshake, wait for natural exit code 0, and will byte-compare generated `.new` output with the checked-in behavioral fixture.
- The recipe input databases intentionally freeze the one connection timestamp created while authoring them; all state produced by their `server_started` recipes uses static wizard ownership and is deterministic. Re-running a recipe does not involve a client connection.
- Managed Toast successfully regenerated the recycle fixture from `RecipePendingWaifRecycleShutdown.db`; that exact 2861-byte output now replaces `PendingWaifRecycleShutdown.db`.
- Next: regenerate nested and panic fixtures the same way, replace temporary pauses with `equals_file`, update behavioral assertions/checksums/workflow, run all gates/matrix, commit/push/update draft PR and issue.
