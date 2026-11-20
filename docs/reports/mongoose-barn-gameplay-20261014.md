# First Barn gameplay cohorts, October 14, 2026

This records functional attempts, not an accepted performance baseline. The
runner used unchanged real flight and Lumos/Nox scenarios, ordinary actors,
private database/catalog copies and a loopback-only Linux namespace. No live
Mongoose process was accessed. Raw `.tmp` links are local retained recordings,
not artifacts included in a fresh checkout.

## Pinned inputs

- Barn source checkout: `2ec34abbadb6cc91a38154fb788d0a2189a32192`, current-main
  `db01daf` plus committed tooling. Lifecycle extensions were uncommitted during
  these cohorts; their exact sources are captured in each `controller-sources`.
- Barn executable SHA-256:
  `2bb5a49b6da0736fe3424653f77b301eabef37e1eb65153906c11e9a12f04d9a`.
- Historical source checkpoint SHA-256:
  `fab6957acffa1db8bc1c4b911fb53a8e8d6e605f4de2a31b9f8e4fa874c3200d`.
- Sound catalog SHA-256:
  `ba15182e25c60e9795c10f8f9479095ff4d22ea9bf2a03d7d061446cfca241e4`.
- Toast emergency-bootstrap executable SHA-256:
  `f96c79de3ac701557c7fe71e2a9be42816c188d6d085afba1a7b4b3c4e7c7a01`.
- Registered runtime profile: `barn-linux-mongoose-outbound-on`;
  `OUTBOUND_NETWORK=1`, `PROMOTE_NUMBERS=1`, port 17778. Toast diagnostic
  collectors disabled. Per-run bootstrap images are not proven identical.
- Frozen lifecycle/runner SHA-256:
  `9c9544b091e024d10200a37fee786726992f87cfee508699201224a13a52ca6c` /
  `dac7872f5620a5592aa1d8b2be1c2575d28c22b4fe5f9c265e040251d7534a98`.

## Magic01: failed ordinary refusal

[Final manifest](../../.tmp/barn-gameplay-magic-01/manifest-final.json),
[scenario result](../../.tmp/barn-gameplay-magic-01/magic/result.json),
[runtime log](../../.tmp/barn-gameplay-magic-01/logs/latest.jsonl).

Barn passed operator readiness and control login; startup took 16.888546889s.
The ordinary actor had wizard=0, is_player=1 and its own fitted wand. The
`weave lumos with 0 focus` negative control returned:

```text
*** CODING ERROR DETECTED! Notifying staff... ***
ScenarioFailure: expected ordinary refusal was absent
status: failed
controller exit: 1
owned shutdown exit: 0; forced: false
finalization_errors: []
```

No complete magic sequence was proved. The refusal observations show no
pending reservation, receipt, debit or light change. Actor, wand, tutor and
room cleanup each returned 1. The private account remains only in the owned
world according to the documented account-retention policy.

All nine original manifest checks passed, but these check immutable inputs and
process shutdown, not successful checkpoint publication. The runtime log says:

```text
level=ERROR msg="final checkpoint failed"
err="write database: write queued tasks: write queued task 473852115:
     write rtenv: runtime environment has 3 names for 24 values"
```

The published `.db.new`/`.db.new.waifids` pair is absent, so the saved input
database is **not verified as post-failure state**. Barn normally leaves its
input database unchanged and publishes a separate pair; that distinction matters
when checking retention. The original tool's exit-code-only shutdown gate is
insufficient; later tooling must verify final publication and retain this failure.
The SIGTERM path emits no positive checkpoint-success log: verification needs
the owned shutdown interval, a fresh published pair with a matching SHA-256
sidecar header, and absence of a recovery journal or explicit failure. The log
also records 21 suspended-task restoration warnings. Neither message establishes
a root cause without further evidence.

The failed action window was 11.064568255s wall and 18.07s whole-process
user+system CPU (100 clock ticks/sec). Its final RSS observation was 3684380KiB.
These include all OS threads, background and observer work. They describe failed
work, cannot be attributed to the command, and cannot be compared with successful
Toast gameplay or used as a speed/memory result.

## Flight01: completed domain sequence, failed checkpoint

[Final manifest](../../.tmp/barn-gameplay-flight-01/manifest-final.json),
[scenario result](../../.tmp/barn-gameplay-flight-01/flight/result.json),
[runtime log](../../.tmp/barn-gameplay-flight-01/logs/latest.jsonl).

The independent fresh world completed one ordinary-crew flight sequence. Natural
cycles advanced 41115264 to 41115271; position changed from
`[1000000000000, 1000000000000, 1000000000000]` to
`[1000000000001, 999999999998.268, 1000000000000]`. The first tank fell from
999.9999004471 to 999.981257209751. Observed stopping velocity was zero, target
speed zero and engine cutoff true. Actor wizard=0; station admission and real
system checks passed. Cleanup released the station and removed ship/console.

```text
scenario.status: complete
actions.completed_iterations: 1
original cohort.status: complete
original controller exit: 0
owned shutdown exit: 0; forced: false
runtime log: final checkpoint failed
```

The domain sequence is proved, but the cohort's original overall success is
not accepted: it missed the same checkpoint error as magic01. The raw manifest
is preserved unchanged to demonstrate the false-success gate. The next tooling
revision must reject this exact combination rather than rewriting the original
recording or weakening gameplay checks.

Flight's action window was 7.776194168s wall / 9.48s whole-process user+system
CPU; final observed RSS was 3478460KiB. These are one mixed-background functional
window with failed checkpoint retention, not repeated speed/memory evidence.

## Verification and next evidence

Before the cohorts, Linux tooling checks reported:

```text
Ran 146 tests in 0.267s
OK
```

Independent reviews cleared ownership/readiness/failure handling for a first
functional run. That verdict did not certify final dump or gameplay success.
The actual failure exposed a missing publication gate and missing verb exception
frames: aggregate retained-error counts are not enough. Preserve failed cohorts,
capture the relevant frames before shutdown, and verify uncertain shared behavior
against the managed stock Toast oracle before any Barn engine fix or optimization.

The immutable checkpoint's existing error storage has two representations:
`#0:handle_uncaught_error` appends `{code,msg,value,traceback}` for a valid player,
but `{code,msg,value,stack,traceback}` for an invalid player. The fourth slot is
therefore not universally structured. Its history limit is 15. The existing
`$issues:record_traceback` also stores the latest `who`, Unix `time` and formatted
`lines` per player before the coding-error notification. Those are source/schema
findings, not observations of magic01's lost post-failure history.

The next capture should project bounded metadata before cleanup, with separate
capture timestamps/failures and explicit unavailable fields. Structured frames
are `{receiver,verb,programmer,verb_definer,player,line}`; formatted headers can
supply only unambiguous definer/verb/line and sometimes receiver. They cannot
establish programmer or per-frame player. Export no messages, values, source,
arguments, raw traceback strings or runtime variables. Use owned FileIO plus a
compact acknowledgement to avoid eval's observed 1000-character transport cap.
Actor-specific issue time/player identifies context, not command-exclusive
causality; compare history only when matching records unambiguously. Capture
limits and unknown/malformed/truncated counts must remain visible.

Source fingerprints from the pinned checkpoint:

```text
#0:handle_uncaught_error 189c0f9ae73c86ed09299cd8793bcb6a7f9d50432567d14854786e89915d1c80
$issues:record_traceback 8bd28046cd3277f895c0d29e464b835b28a43e7cc6ae2f5f0835c6404c064c12
```

## Repaired publication gate

The later tooling revision requires `owned_final_checkpoint` independently of
process exit. Unknown, missing, stale, corrupt, incomplete or explicitly failed
publication cannot pass. It checks the actual signal-shutdown contract without
inventing a positive success log or requiring database contents to change: a
fresh atomic publication of identical bytes can be valid. Symlink/replaced-log
and unfinished-journal evidence is rejected. This proves pair publication and
integrity, not semantic database round-trip behavior.

Read-only replay of the retained logs reported:

```text
magic01: failed -> failed; final_checkpoint: failed
flight01: complete -> invalidated; final_checkpoint: failed
domain_results_preserved: true, both
Ran 152 tests in 0.369s
OK
```

The original cohort files were not rewritten. Replay tests the new verifier
against those artifacts; it does not retroactively prove runtime ownership or
constitute another world run. The exact log SHA-256 values are:

```text
magic01 6b1eafbed58c9240c1f2399bfdf7fc5ec2715b286c933caf249cf9d0b166495a
flight01 4dd14144be8686024214ab2662f58bea3412e3f47c4e10ad456f93af9b949723
```

No successful actual Barn final publication, complete magic sequence, Go CPU/
heap diagnostic profile, paired speedup or lower-memory result is established
by this slice. Those remain required subsequent evidence.
