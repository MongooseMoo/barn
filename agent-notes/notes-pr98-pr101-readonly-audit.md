# PR 98 and PR 101 current-state audit checkpoint

- Scope: read-only correctness audit of merged PRs 98 and 101 at master commit `911b97f`.
- Constraint: do not inspect historical Markdown; inspect exact merge diffs, current implementation/tests, and live GitHub evidence only.
- PR 98 merge commit: `977f8cb`; changes `server_version()` boolean option values from integer `0`/`1` to string `OFF`/`ON`, with dot/slash key tests and unknown-option tests.
- PR 98 live evidence: build/unit check passed; conformance check failed at capability admission on ephemeral listeners, a different capability later addressed by PR 101.
- PR 101 merge commit: `56e1358`; separates listener descriptor presence from numeric value so descriptor zero can be represented and propagated.
- PR 101 live evidence: build/unit check passed; conformance check failed after admission, with a large cascade of broken-pipe/connection-refused failures; the originating server failure still needs to be isolated from the live log.
- Repository working tree is heavily dirty and local checkout is stale at `aa82abf`; audit uses Git object reads for exact commit `911b97f` and read-only GitHub queries.
- Blocker: none yet; exact PR 101 implementation/test review and CI failure-root isolation remain.
- Next: inspect PR 101 core diff/current code, map all listener metadata constructors and synchronization, inspect tests for true accept-path and duplicate-zero coverage, then issue APPROVE/BLOCK per PR.

## Second checkpoint

- Current `911b97f` retains both PRs' touched production/test files unchanged from their merge commits.
- PR 101 stores both `descriptorPort` and `boundPort`; runtime listeners retain requested descriptor `0`, while primary startup port `0` is replaced with the actual bound OS port.
- Accepted TCP/TLS/WebSocket connections now call `newConnectionFromTransport(transport, record)`, set listener metadata while the manager lock is held, and only then publish the connection in lookup maps.
- The connection mutex protects the new `(listenerPort, listenerPortSet)` pair; builtin fallback to the manager startup port happens only when metadata is unset, not when it is explicitly zero.
- PR 101 tests cover runtime TCP/TLS/WebSocket descriptor-zero registration/removal/duplicate rejection, startup ephemeral-port discoverability, builtin propagation, and pre-publication metadata. The publication test invokes the constructor directly rather than racing a real accept path.
- Live GitHub state confirms both PRs merged without reviews and both had a passing build/unit check plus a failing MOO conformance check. Exact master `911b97f` likewise has passing build/unit and failing conformance checks.
- PR 101 full conformance admitted and ran, but ended with broad existing semantic failures and repeated capability-probe errors after connection loss; the log does not expose a Barn panic/fatal line.
- Next: verify Toast behavior directly for option strings and runtime listener descriptor zero, inspect exact failure summaries for target cases, and decide whether failed integration gates are process blockers separate from code-semantic findings.

## Final checkpoint

- Canonical WSL Toast `build-release` oracle returned `"ON"` for `server_version("options/OUTBOUND_NETWORK")` and aborted with `E_INVARG` semantics for dotted `options.OUTBOUND_NETWORK`; its generated `version_options.h` contains OUTBOUND_NETWORK but no PROMOTE_NUMBERS, so both PROMOTE_NUMBERS spellings are unsupported in that binary.
- Current Barn `builtins/system.go:566-569` accepts both dot/slash spellings and both OUTBOUND_NETWORK/PROMOTE_NUMBERS. This is broader than exact Toast behavior, and the PR 98 unit tests explicitly lock in that divergence.
- Canonical WSL Toast oracle returned `listen(#2, 0) => 0`, exposed `port -> 0` in `listeners(0)`, returned `unlisten(0) => 0`, then exposed no listeners. PR 101 matches this observable behavior.
- Current PR 101 anchors: descriptor/bound separation at `server/connection_manager.go:222-247`; TCP and WebSocket accepted paths at lines 334 and 378; metadata-before-publication at lines 404-417; explicit-zero presence bit at `server/connection.go:253-266`; builtin fallback only when unset at `builtins/network.go:1048` and `1215-1217`.
- Both dedicated PR worktrees are clean. No further local test run is needed because exact live CI build/unit jobs passed; conformance jobs remain red and are a process/integration caveat.
- Verdict: PR 98 BLOCK for Toast semantic divergence and red conformance. PR 101 APPROVE for touched-code correctness, with explicit caveat that its conformance check and current master conformance remain red and its publication-order unit test directly invokes the shared constructor instead of driving a real concurrent accept.
