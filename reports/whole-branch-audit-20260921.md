# Whole-branch audit and delivery record

Scope: the entire branch against master `83c3e8d7ef3546ce2ae82d550684790f77706975`,
not only the recent correctness and speed repairs. The initial audited head was
`5729e21` (36 commits, 177 changed files). Final runtime source is
`0d5254ad74f059932c9bac7321cca159effd3732`. User authorized PR publication followed
by a normal merge after the audit and GitHub CI pass.

## Coverage

| Area | Review focus and evidence |
| --- | --- |
| Admission, gate, scheduler and task lifecycle | FIFO grants, cancellation/ownership, preemption, nested scopes, wakeups and ready batches, input ordering, checkpoint barriers; independent analyst source review and broad race tests. |
| Persistent maps and value finalization | Typed keys, collision buckets, immutable path copying, constructor ownership, deletion/reinsertion order, byte accounting and synchronized finalization cache; parent and independent verifier review, randomized alias/collision/bulk/concurrent tests. |
| Compiler and parser | Complete local-bearing opcode inventory, optional/wide bindings, unchanged jump widths/source names, no persisted-bytecode rewrite, catch-default precedence; parent/verifier review and managed stock-Toast candidate tests. |
| Store, WAIFs and checkpoints | Mixed object/WAIF publication, read dependencies, domain ownership, renewal, cache invalidation, history floor races, weak registry lifetime and frozen identity-preserving checkpoint payloads; parent/verifier review and race tests. |
| VM roots and callbacks | Pending identity-set resets, historical versus committed reachability, task-root handoff, callback tick/seconds charging, retry/irreversible boundaries; parent/analyst/verifier review and regression tests. |
| TCP/WebSocket output | Captured binary/no-newline/no-flush semantics, deferred output, UTF-8 text framing and buffered-error recovery; analyst review plus repaired transport regressions. |
| Tools and documentation | New Python/PowerShell sources, disposable fixture handling, benchmark assertions/provenance, operational defaults, historical claims and current conformance commands. Changed scripts parse; private-key/token signature scan found no matches. |
| Publication contents | Changed-file and binary-extension audits contain no Mongoose database, SQLite fixture, executable or raw profile. Preexisting untracked coder report remains excluded. |

The independent [scheduling audit](whole-branch-scheduling-audit-20260921.md) and
[merge gate](whole-branch-verifier-20260921.md) record their own scope and output.
The coverage above does not claim that every line received three independent reviews.

## Findings and repairs

1. **P1: standalone server hooks had no seconds budget.** The new periodic VM
   budget check aborted a finite hook at 1024 charged ticks. Reproduced with
   direct, argstr and inherited execution-owned calls; commit `ae8fafb` initializes
   the foreground budget/deadline through production helpers. Tests additionally
   cover sweep-owned hooks, actual expiration and nested inherited budgets.
2. **P2: binary notify could emit invalid WebSocket text.** Byte `0xff` reached
   the text writer, violating the existing text-only transport contract. Commit
   `0d5254a` validates before queueing and again at the writer. Invalid buffered
   notes cannot poison subsequent output; TCP raw-byte output is unchanged.
3. **P2: newly added conformance helpers used obsolete invocation paths.** Three
   wrappers invoked direct pytest or delegated to that old wrapper, and the
   oracle default was workload-specific. Commit `218fb4d` removes them and
   documents the current managed CI workflow. Historical reports explicitly
   disclaim their old baseline-failure classification as current evidence.
4. **P2: three candidate cases never ran.** Two YAML files required a nonexistent
   `fork` capability. Removing that gate in `218fb4d` preserves every assertion
   and enables the cases. Stock Toast passed all eleven candidate/admission cases,
   then the rebuilt Barn passed all eleven.

Raw focused evidence:

```text
Hook RED_EXIT=1: finite hook E_MAXREC: seconds limit exceeded
Hook independent race: ok engine 1.246s
WebSocket RED_EXIT=1: invalid UTF-8 reached writer; invalid buffered note queued
WebSocket independent race: ok server 1.145s
Stock Toast: 11 passed, 1 warning in 7.64s; EXIT=0
Barn 0d5254a: 11 passed, 1 warning in 7.62s; END mode=candidate exit=0
POWERSHELL_PARSE_ERRORS=0
PYTHON_CHANGED_SCRIPTS_PARSE_PASS=8
PRIVATE_KEY_TOKEN_SIGNATURE_MATCHES=0
```

The first managed candidate attempt was rejected before startup because packaged
fixtures were outside the candidate root. Retrying with disposable copies inside
the candidate checkout resolved that harness restriction; the documented command
now includes that requirement. No manual server or direct pytest probe was used.

## Final validation and limits

The final-source complete local CI and full managed conformance passed. The
independent verifier approved the whole-branch source review, conditional on
green GitHub checks for the exact published PR head before the actual merge.

```text
CI_COMMANDS_PASS
12524 passed, 420 skipped, 1 warning in 329.20s (0:05:29)
END mode=full exit=0
Verifier: MERGE for source 0d5254ad74f059932c9bac7321cca159effd3732
```

Earlier speed evidence is pinned to optimization source `311f234` versus corrected
baseline `853cba5`: five development pairs and three independent holdout pairs,
all PBT and counter checks passing. See the
[complete experiment record](../experiments/2026-09-21-waif-prune-watermark.md).
Those timings are not relabeled as measurements of the later audit fixes.
The benchmark uses one machine and a disposable PBT-repaired fixture; the
untouched production snapshot is not claimed to pass PBT. Databases, credentials,
raw live transcripts and profiles remain outside Git.
