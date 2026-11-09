# Real offline flight and magic

Raw `.tmp` links below refer to local retained recordings, not files shipped in
a fresh checkout. The [tracked acceptance summary](reports/mongoose-gameplay-acceptance-20261013.md)
preserves input hashes, outcomes and measurement limits; use fresh output paths
to reproduce those checks.

`scripts/run_mongoose_gameplay.py` runs an owned copy of a SHA-256-pinned Mongoose
checkpoint inside a loopback-only Linux network namespace. It creates private
accounts and ordinary player characters through existing game APIs. Production
credentials are unnecessary. Setup is outside the timed gameplay phase.

The flight scenario constructs a real Type 6 shuttle with installed systems,
grants its crew legitimate access, and drives connected console commands. Only
natural universe cycles advance flight. Position, heading, velocity, fuel,
station membership and actual braking are independent completion checks.

The magic scenario uses actual catalogue Lumos/Nox, ordinary wand fitting,
wielding and lessons. It counts completed light/extinguish sequences, checks
effects and resource receipts, and records natural fizzles separately. No copied
enabled spells, artificial proficiency or wizard casting substitute for work.

From PowerShell, verify the installed distribution and choose a fresh output:

```powershell
wsl --list --quiet
wsl -d Debian --exec unshare --net -- bash -lc 'set -euo pipefail; ip link set lo up; python3 /mnt/c/Users/Q/code/barn-gameplay-baseline/scripts/run_mongoose_gameplay.py --db /mnt/c/Users/Q/projects/mongoose/db/mongoose.db.new.2026-10-12T2349 --db-sha256 FAB6957ACFFA1DB8BC1C4B911FB53A8E8D6E605F4DE2A31B9F8E4FA874C3200D --moo /root/build/toaststunt-profiler-retention-refresh-order-20261013/moo --moo-sha256 f96c79de3ac701557c7fe71e2a9be42816c188d6d085afba1a7b4b3c4e7c7a01 --sound-db /mnt/c/Users/Q/projects/mongoose/db/sound.sqlite.barn395396-20261011T0348 --sound-db-sha256 BA15182E25C60E9795C10F8F9479095FF4D22EA9BF2A03D7D061446CFCA241E4 --out /mnt/c/Users/Q/code/barn-gameplay-baseline/.tmp/gameplay-next --scenario both --iterations 1 --warmup-seconds 30 --deadline-seconds 120'
```

`--scenario flight|magic` selects one scenario. `--iterations` counts complete
sequences, not submitted commands. Seed records controller choices; it does not
claim to reset engine randomness. Normal character permissions, station admission,
training and resource/recovery rules remain active. A refusal, native failure,
missing effect or expired deadline cannot count as productive work.

Default `--profile off --native-profile off` disables both diagnostic profilers.
It still includes controller/observer work and naturally scheduled world work.
Toast CPU deltas cover its main thread; Barn CPU deltas cover the whole process,
including all OS threads and user/system time at the process clock-tick resolution.
Neither is assigned exclusively to a single action.
Loopback input response brackets are separate from domain completion and from
human/client/network latency.

The Barn functional runner uses `--engine barn --barn <Linux executable>
--barn-sha256 <hash>`. The required `--moo` inputs still identify the Toast
emergency bootstrap executable that prepares the private control account. The
registered `barn-linux-mongoose-outbound-on` profile enables numeric promotion
and outbound operations within the same loopback-only namespace. Startup must
pass the operator `/readyz` check before control login; startup, profile, process
identity and final input hashes remain in the run manifest. Owned SIGTERM
shutdown allows up to 90 seconds for Barn's final database dump. A forced stop
invalidates the cohort. Barn process exit0 alone is insufficient: acceptance
also requires verified publication of a fresh `.db.new`/`.db.new.waifids` pair,
a sidecar header matching the database SHA-256, no recovery journal or explicit
checkpoint failure, and the owned shutdown log boundary/completion. The signal
path emits no positive checkpoint-success log. Missing or uncertain evidence
invalidates the cohort while preserving completed domain results separately.
This verifies publication/integrity, not a semantic database round-trip.

The [October 14 Barn cohort report](reports/mongoose-barn-gameplay-20261014.md)
records completed real flight, the magic refusal failure and the original
exit-code-only gate's missed final-checkpoint failures. Its raw recordings remain
unchanged; they are not accepted performance baselines.

Barn rejects both Toast profiling flags. Add `--go-cpu-seconds 15` for a separate
Barn diagnostic cohort: the owned private HTTP listener supplies a bounded Go CPU
profile and before/after runtime vars and natural heap snapshots (`gc=0`). The
default is 0; accepted requests are 1..30 seconds and require `--engine barn`.
Set `BARN_RUNTIME_DIAGNOSTICS=1` inside the namespace to retain debug phase records.
CPU request transmission is recorded without asserting profiler activation or
exact action overlap. Whole-process CPU includes background, runtime and observer
work; heap allocation totals can include loading, and snapshot windows differ.
Raw diagnostics remain local. Transport/gzip checks cannot validate pprof sample
types or nonzero samples, so requested profiles leave scenario status
`diagnostic-unvalidated` and `go_profiles_validated=false` pending independent
offline parsing. The runner exits nonzero; other failures still invalidate the
cohort. Preserve its original manifest alongside the analysis rather than
rewriting it. The [diagnostic report](reports/mongoose-barn-diagnostics-20261014.md)
records actual parsed CPU shares, memory and remaining correctness blockers.

Every scenario now attempts a metadata baseline after setup and another
capture immediately on failure, before cleanup. A MOO-side whitelist projects
bounded actor-specific issue/history frames before FileIO or recording; messages,
source, arguments and runtime values are excluded. Parsed frame identifiers,
positions, shape/omission counters and correlation status are retained. Schema 2
also retains a whitelisted error enum from a typed native ERR history slot.
Four-field formatted and five-field structured history records can both match
the actor issue through their complete bounded formatted-frame list. Only a
unique match supplies the primary `error_code`; ambiguous, unmatched, malformed
or unknown codes remain `unavailable`. Messages and error values are excluded.
Effective programmer/player and unresolved line positions remain explicit.
A unique bounded history match is not exclusive task/command causality, and
`E_MAXREC` alone does not identify which Barn execution limit fired. See the
[magic diagnostic follow-up](reports/mongoose-magic-diagnostics-20261014.md).
Unavailable diagnostic metadata cannot replace the primary gameplay failure.
This best-effort metadata collection is not a mandatory functional-success gate;
read its own status before claiming complete diagnostic evidence.

A successful functional run alone cannot establish equivalent engine performance:
bootstrap images are prepared separately per run, and identical prepared images across
engines have not yet been proved.

Run this Barn flight command with a fresh `--out`
directory for every attempt. Its result must be read from the final manifest.
The pinned binary includes the
[stock-verified queued-fork checkpoint repair](reports/mongoose-fork-checkpoint-20261014.md),
which passed real flight and verified final publication. When rebuilding, verify
`./cmd/barn` with `go list`, build there and substitute the new executable hash.

```powershell
wsl --list --quiet
wsl -d Debian --exec unshare --net -- bash -lc 'set -euo pipefail; ip link set lo up; python3 /mnt/c/Users/Q/code/barn-gameplay-baseline/scripts/run_mongoose_gameplay.py --engine barn --db /mnt/c/Users/Q/projects/mongoose/db/mongoose.db.new.2026-10-12T2349 --db-sha256 FAB6957ACFFA1DB8BC1C4B911FB53A8E8D6E605F4DE2A31B9F8E4FA874C3200D --moo /root/build/toaststunt-profiler-retention-refresh-order-20261013/moo --moo-sha256 f96c79de3ac701557c7fe71e2a9be42816c188d6d085afba1a7b4b3c4e7c7a01 --barn /mnt/c/Users/Q/code/barn-gameplay-baseline/.tmp/conformance/barn --barn-sha256 13ef3890cf48a0c53ae2d48bdb471059062765232a4c59802342e4b2ea749e16 --sound-db /mnt/c/Users/Q/projects/mongoose/db/sound.sqlite.barn395396-20261011T0348 --sound-db-sha256 BA15182E25C60E9795C10F8F9479095FF4D22EA9BF2A03D7D061446CFCA241E4 --out /mnt/c/Users/Q/code/barn-gameplay-baseline/.tmp/barn-gameplay-next --scenario flight --iterations 1 --warmup-seconds 30 --deadline-seconds 120'
```

For a separate Toast diagnostic cohort, add `--profile definitions` and optionally
`--native-profile required`. The native collector targets only the owned main
thread, proves local perf recording capability and preserves raw data/report.
`auto` records explicit unavailability; `required` invalidates the cohort when
collection fails. Native user-space CPU excludes kernel and worker CPU. These
diagnostic runs are not uninstrumented performance comparisons.

Manifests preserve perf's raw exit code. Perf's requested SIGINT shutdown can
return `-2` after flushing; the collector accepts it only with its verified own
stop, acknowledged disable and valid nonempty capture. Unexpected exits and
forced stops remain failures. Both sampling rates must be 10..1000Hz.

On this WSL installation perf lacks unwind support, so the collector records
flat instruction samples and reports that limitation. It cannot supply inclusive
native call stacks. Raw recording uses a private Linux filesystem directory;
only the collector's artifacts are copied into the output directory.

Artifacts include initial/final manifests, source/binary/controller hashes,
ordinary command transcripts, setup/actions/drain phase boundaries, before/after
domain state, failed and unfinished work, process CPU/RSS chronology, cleanup
and owned shutdown. Failed cohorts are retained. Profile export and its request
duration remain separate; source-line resolution is bounded and unresolved
positions stay visible. RSS is not a heap/retained-allocation profile.

Every run checks source checkpoint/catalog hashes again and stops only its owned
PID. A final successful status requires completed scenarios, requested collectors,
unchanged input/controller hashes and a successful owned shutdown. Read the raw
results; an implementation or unit-test pass alone does not prove gameplay.

This runner implements the first two scenarios in the
[campaign plan](../plans/mongoose-gameplay-performance-campaign.md). The same
domain outcomes must be proved on Barn before comparing engines. Allocation
attribution, mixed fleet/caster load, scheduled arrivals and latency percentiles
need separate subsequent cohorts; they are not implied by a single successful run.

Validated local evidence from October 13, 2026 is retained in
[both01](../.tmp/gameplay-both-01/manifest-final.json). One command completed one
flight sequence and one actual Lumos/Nox pair; magic included one natural fizzle
and three total attempts. The final manifest and scenario results record:

```text
status: complete
scenarios: [flight, magic]
completed_iterations: 1 each
shutdown.exit_code: 0
source_database/executable/controller_sources/sound_catalog/owned_shutdown: true
```

Both diagnostic profilers were disabled in this cohort. The recorded action
windows were flight 3.180644918s wall / 3.182204075s main CPU and magic
3.082636184s wall / 3.077598100s main CPU. Whole-process high-water RSS was
604636KiB. These windows include observer requests and scheduled background
work. They are one functional cohort, not repeat measurements, exclusive action
costs, a pristine-stock benchmark or evidence of faster execution/lower memory.
Setup, drain, cleanup and shutdown have separate evidence.

The isolated world retains inherited background tasks and their native errors.
For example, both01's bounded retained history reported `count=15, other=15`;
this is neither a complete error inventory nor an error arrival rate. Full
external-service parity and steady background behavior still need separate
validation before treating this workload as a production performance baseline.

[Flight05](../.tmp/gameplay-flight-05/flight/result.json) and
[magic04](../.tmp/gameplay-magic-04/magic/result.json) preserve the earlier
individual successful runs. Failed attempts remain alongside them. Tooling
regressions ran in WSL: `Ran 94 tests in 0.152s / OK`.

The separate [diagnostic02](../.tmp/gameplay-diagnostic-02/manifest-final.json)
also completed both scenarios, with native profiles collected (588 flight /
510 magic samples) and valid MOO profiles (747 / 664 accounted CPU periods,
unknown or missing 0 / 1). It ended with owned shutdown exit0, all five final
checks true and no finalization errors. Read the
[flight MOO report](../.tmp/gameplay-diagnostic-02/flight-cpu-report.md) and
[magic MOO report](../.tmp/gameplay-diagnostic-02/magic-cpu-report.md).

When perf lacks ELF support, resolve saved captures offline with
`scripts/symbolize_mongoose_native.py`. It verifies the captured executable SHA
and build ID, converts recorded mappings through ELF load segments, and checks
finite symbol bounds. It preserves every sample/period, including unresolved
shared libraries, and never attaches to or changes a running MOO. Use a fresh
output directory:

```powershell
wsl -d Debian --exec python3 /mnt/c/Users/Q/code/barn-gameplay-workloads/scripts/symbolize_mongoose_native.py /mnt/c/Users/Q/code/barn-gameplay-workloads/.tmp/gameplay-diagnostic-02/flight/native --output /mnt/c/Users/Q/code/barn-gameplay-workloads/.tmp/native-symbol-flight-next
```

Saved [flight symbols](../.tmp/native-symbol-flight-20261013-02/symbols.json) and
[magic symbols](../.tmp/native-symbol-magic-20261013-02/symbols.json) resolve
476/588 and 413/510 samples, with the remaining weights explicitly unresolved.
`db_fixup_owners(long)` accounts for estimated native self-IP shares 17.86% /
20.00%; identified profiler-specific functions account for 19.56% / 20.59%.
These are instrumented whole-main-thread user CPU estimates. Exact source
lines, native call graphs and total off/on overhead are unproven. Native and
MOO sampling boundaries differ. See
[the updated diagnosis](../why-mongoose-slow.md) for interpretation.
