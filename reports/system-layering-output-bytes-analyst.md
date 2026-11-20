# Output byte accounting analyst review

## Scope and state

Reviewed the actual open PR #636 (`Count buffered output in bytes`) through
`gh pr view`, `gh pr diff` and the GitHub file patches; local checkout matched
the dispatched PR head. Source review is bounded to the byte-accounting change.
No source edits, local builds, local tests or published reviews were performed.

## Findings so far

No concrete implementation defect found in the reviewed change. Production
`outputBuffer` mutations are confined to `server/connection.go`: construction,
the two enqueue sites and `flushLocked`. Both enqueue sites add the notification
byte length while holding the existing connection mutex. Flush subtracts only
after successful delivery; failures retain the failed entry and its suffix.
The getter uses the same mutex and remains constant time.

`builtinNotify` decodes binary strings and captures `NoNewline` before creating
the deferred notification. `builtinBufferedOutputLength` uses that captured
payload/framing and filters task-local notification effects by recipient.
The shared calculation counts payload bytes plus optional CRLF; it does not
represent physical socket progress or WebSocket/TLS overhead.

## Evidence inspected

The edited regressions assert exact byte totals, including first/middle/last
write failures, successful retries, UTF-8, empty framed and unframed payloads,
binary decoding, option changes after capture and another recipient's effects.
The existing suffix/backing-slot and write-attempt checks remain in place.

Parsed the managed XML artifacts directly:

```text
toast-byte-v2.xml: tests=5 failures=0 errors=0 skipped=0
barn-byte-before.xml: tests=5 failures=4 errors=0 skipped=0
barn-byte-after.xml: tests=5 failures=0 errors=0 skipped=0
```

The five cases are capability admission and four text/binary byte-accounting
cases. These artifacts establish the bounded oracle comparison, not whole-suite
coverage or a measured performance result. Full PR CI is checked separately.

## Final bounded verdict and unknowns

No actionable defect found in the byte-only change. Effect publication clears
the task-local effect list before replay and passes each captured notification
to the existing connection owner; this change introduces no second publication
or queue lifecycle. Existing WebSocket rejection coverage asserts rejected
buffered UTF-8 leaves the queued count at zero. Validation remains before both
queue append and byte increment (`server/connection.go:103-117`). All byte-cache
accesses use the existing connection mutex; no new goroutine or lock is added.
Queue updates and queries remain constant time; pending-effect summation keeps
its existing linear scan. No measured speedup or performance non-regression is
established by this review.

PR check snapshot at review completion:

```text
Build & unit tests: pass (1m34s)
MOO conformance suite: pending
Serializable histories and negative control: pending
```

This is a source-level analyst verdict, not merge approval. Full pending PR CI,
race instrumentation and benchmarking remain outside this review's evidence.
Known preexisting buffer capacity/refusal behavior is outside this change and
is not reported here as a regression.

## Coordinator CI evidence follow-up

The Elle artifact is preserved locally at
.tmp/system-layering/elle-byte-pr/elle-evidence.tar.gz, including raw histories,
manifests, checker outputs and anomaly witnesses. results.json reports true for
both models for healthy seeds 412, 395 and 396, and false for all twenty fault
controls. The seed-412 list-append manifest reports nonzero paths including
parallel 484, exclusive 357, exclusive_gate 460, hot_renewal 4, fork 14,
suspend 241, native_exception 24 and verb_memo 474. This is explicit checker
and path evidence beyond the job status. Full conformance remains pending.
