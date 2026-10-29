# Partial TCP and Telnet input bounds: issue #560

This is an incomplete research result. Source inspection established the stock
Toast policy and where it applies. The attempted managed run failed collection;
none of its boundary assertions or capability admission ran. The research agent
subsequently failed with a platform safety-filter message. No rejected work was
retried, and no Barn input-limit implementation is included.

## Verified source and build context

The target is the canonical Debian WSL oracle documented in
`plans/barn-toast-mongoose-convergence-workstreams.md`:
`/root/src/toaststunt/build-release/moo`. Its tracked source had no modifications
at inspection. `src/include/options.h` matched the configuration checksum in
`profiles/toast/stock-wsl-testdb.json`. The generated CMake compiler flags include
the source include directory and build-release directory, with no command-line
definition overriding MAX_LINE_BYTES. The limit checks below are unconditional.

Selected raw source/build evidence is retained in
`investigations/input-bounds-560/source-evidence.txt`.

| Owner | Inspected behavior |
| --- | --- |
| Toast `src/include/options.h:247` | `MAX_LINE_BYTES` is 5242880 bytes, or 5 MiB. The option comment describes closing an excessive input connection to prevent allocation failures. |
| Toast `src/network.cc:395-399` | Before processing each text/Telnet byte, a retained Telnet command stream at least that long returns failure to the input reader. |
| Toast `src/network.cc:476-480` | Before its next physical read, a text input stream at least that long returns failure. |
| Toast `src/network.cc:484,544-562` | Reads use a 1024-byte buffer. Binary mode encodes and delivers that read, resets the stream, and does not accumulate a newline-delimited message. Text mode calls the Telnet state machine for the bytes read. |
| Barn `server/transport.go:94-195` | The current text builder and Telnet-command slice append until their framing terminator without a byte limit. |
| Barn `server/input_processor.go:151-175` | Authenticated input clears its read deadline; binary input uses ReadChunk where supported. |

The text limit is a **pre-read check**, not a strict per-byte rejection at
exactly 5 MiB. A read can append up to 1024 additional bytes, and a terminator
within that read can deliver/reset the line before the next check. Consequently,
changing Barn to reject its next byte at exactly 5 MiB is not established as
equivalent to this source. The retained partial text stream can reach at most
the threshold plus 1023 bytes through that read path before another check.
This arithmetic is source analysis, not a runtime boundary measurement.

Binary traffic must not acquire a lifetime byte quota based on this line limit:
the inspected target delivers and resets every raw read. A sequence totaling
more than 5 MiB is different from one retained partial text/Telnet item.

## Managed attempt and what failed

The research fixture uses the existing managed harness and real loopback
connections. It sends paced data through the target's existing network builtins,
with cases for a completed below-limit text line, unfinished text/Telnet input,
and binary chunks. It does not launch a manual server or use a tracked DB as
working storage. The packaged Test.db/startup fixtures were copied through the
editing tool and checked for matching hashes inside the candidate checkout.

The saved raw `investigations/input-bounds-560/oracle-initial.xml` records:

```text
errors="1" failures="0" skipped="0" tests="1"
ValueError: requires.config contains unsupported names: OUTBOUND_NETWORK
```

That is a collection failure, not an oracle behavior result. I put a runtime
option in the harness-capability field. The existing schema distinguishes
`requires.config` capabilities such as `log_file` from runtime `option.*`
conditions. The fixture now uses the existing `log_file` requirement and
`skip_if: "not option.OUTBOUND_NETWORK"`. This metadata correction has not been
rerun or claimed validated.

The failed agent did not save its exact command/stdout in this worktree. Its
last progress message reported the canonical managed entrypoint, candidate mode,
admission selection and a Linux-only uv environment; that message is not a
substitute for retained command output. No admission JSON or persistent server
log was created by the failed collection. XML is retained rather than replaced
with a successful-looking record.

The subsequent platform response was:

```text
Agent errored: This content was flagged for possible cybersecurity risk.
```

There is no verified runtime verdict from that agent. Root inspected the saved
artifacts and process inventory and did not retry the rejected research action.

## Bounded implementation direction, pending oracle evidence

The existing input parser should own any eventual accumulator limits. The
smallest direction suggested by the source is a 5 MiB scale, terminal transport
closure on excess, separate accounting for partial text and Telnet commands,
and continued chunked binary input. No global connection quota, new input queue,
worker, storage layer or replacement parser is justified.

The exact text boundary remains open because the target checks before reads;
a strict per-byte cap can change accepted input around the boundary. A faithful
proposal needs managed evidence for completed text near the threshold, text
without a terminator, a complete versus unfinished Telnet command near the
threshold, binary-mode chunks, and cleanup after the peer closes. The prepared
fixture is only a starting point and does not yet cover every one of these.

Do not select a stricter policy, weaken conformance, claim a memory-exhaustion
incident or implement a new input limit from this partial result. Issue #560
remains open because its source-and-oracle acceptance criteria are not met.
