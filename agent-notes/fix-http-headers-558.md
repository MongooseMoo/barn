# Bound HTTP header reads (#558)

## Goal

Bound incomplete HTTP headers on existing WebSocket, operator and debug listeners without shortening established MOO sessions or diagnostic response lifetimes.

## State

Implementation authorized by Q's instruction to execute the four filed issues. Source and two focused behavior regressions are complete and committed locally; both authored tests passed. Worktree: `barn-http-headers-558`; branch: `fix/http-headers-558`. Root publishes and monitors PR CI; this worker does not publish.

## Decisions and why

Smallest change: define one five-second `HTTPReadHeaderTimeout` policy in existing `internal/listener`, then set only `http.Server.ReadHeaderTimeout` in the two existing constructors. Initial production estimate is roughly seven added lines plus an import. The listener package already owns shared listener vocabulary and is already a dependency of the connection manager. The operator endpoint's existing shutdown allowance is also five seconds; this is a bounded startup allowance, not a measured optimum. No new package, option, protocol, admission queue or handler/write/idle timeout is needed.

Go documents that the header deadline is reset after headers are read. The WebSocket upgrade, MOO idle lifetime and long profile responses must retain their existing behavior. Primary reference: https://pkg.go.dev/net/http#Server.

Focused regressions drive incomplete headers to the actual listener constructors and verify closure, while a concurrent complete HTTP request or WebSocket upgrade remains usable beyond the header allowance. Only those authored tests ran locally after edits; remaining checks belong to PR CI. The source diff is eight insertions and one deletion across the existing three production files.

## Surprises

None established. WSS remains unsupported by the existing listener admission path; this change does not add it. Origin policy is separately tracked in #417.

## Next step

Root publishes the scoped commit and follows PR CI and review. No outstanding implementation concern is known from this focused verification; repository-wide CI and managed conformance are not claimed here.

## Checkpoint 1

- Read #558 and both live HTTP server constructors; neither sets a header timeout.
- Root approved the single shared five-second header allowance in the existing listener package.
- Work remains scoped to HTTP header reads; post-upgrade sessions and long responses keep their existing lifetimes.
- No implementation or tests have run yet; no blocker is known.
- Next: apply source and focused behavior-test edits, then run only the authored tests.

## Focused verification

Commands run after all source edits completed:

```text
go test ./internal/app -run '^TestHTTPEndpointHeaderTimeoutPreservesLongResponse$' -count=1 -v
--- PASS: TestHTTPEndpointHeaderTimeoutPreservesLongResponse (5.01s)
PASS
ok github.com/MongooseMoo/barn/internal/app 7.576s

go test ./server -run '^TestWebSocketHeaderTimeoutPreservesUpgradedSession$' -count=1 -v
--- PASS: TestWebSocketHeaderTimeoutPreservesUpgradedSession (5.01s)
PASS
ok github.com/MongooseMoo/barn/server 7.579s
```

## PR description draft

Set a shared five-second HTTP header-read allowance on WebSocket and debug/operator listeners so incomplete requests cannot retain their connections indefinitely. The timeout ends at header parsing; long diagnostic responses and established WebSocket sessions keep their existing lifetimes.

Closes #558.

Validation: the two focused tests listed above passed, exercising incomplete-header closure alongside a long HTTP response and an idle upgraded WebSocket session. Remaining validation comes from the PR's existing CI.
