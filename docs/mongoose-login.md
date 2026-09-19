# Comparing Mongoose login on Barn and Toast

Run these commands from the Barn checkout in PowerShell. Evidence and disposable
database copies go under `.tmp/mongoose-account-20260917` by default; override
`-RunDir` consistently to use another directory. These are live workload probes,
not the generic conformance harness.

## Refresh the inputs

```powershell
./scripts/fetch-mongoose.ps1
```

This fetches `mongoose@mongoose.world:~/mongoose/mongoose.db.new` and creates a
consistent backup of `~/mongoose/files/sqlite/sound.sqlite` using SQLite's backup
API, including data in its WAL. It replaces the local `mongoose.db.new` and
`files/sqlite/sound.sqlite`, preserving their previous contents in the run
directory. The server's sound file is singular, `sound.sqlite`. The script
retains the remote `/tmp/barn-sound-<timestamp>.sqlite` backup and prints its path.

A password change only appears in a downloaded MOO database after the live
server checkpoints. An `Access Denied` result on both engines needs a refreshed
checkpoint or corrected credentials before account equivalence can be claimed.

## Start and probe

The Toast source worktree must already be checked out at the intended revision.
The current comparison uses `/root/src/toaststunt-mongoose-login-20260917`, created
from `mongoosemoo/mongoose`. `-BuildOracle` rebuilds it from scratch and records
the revision and binary hash. CMake runs **inside the build directory** because
this branch generates `version_options.h` in its working directory.

```powershell
./scripts/mongoose-login.ps1 -Engine Toast -BuildOracle -Start -Proxy -Commands guest,look
./scripts/mongoose-login.ps1 -Engine Barn -Start -Proxy -Commands guest,look -CaptureDebug
```

Defaults: WSL distribution `Debian`, Toast port 17880, Barn port 11485, Barn debug
port 11486, operator port 11487. Stop an earlier instance on those ports before
using `-Start`; PID files are in the run directory. `-Start` uses disposable
database and SQLite copies. Barn enables numeric promotion and disables periodic
checkpoints; Toast uses the Mongoose build's numeric-promotion configuration.

Omit `-Start` to probe an existing instance. For an account, supply `-Username`
and `-Password`; subsequent `-Commands` can select a character or issue `look`.
The temporary command file is removed after the probe, and displayed password
text is redacted. JSONL events record received bytes and send timestamps without
sent command text. Treat transcripts as private account data.

`-Proxy` sends a PROXY prelude because this MOO trusts localhost as a proxy and
can suppress its initial banner until the real client address is supplied.
Compare runs both with and without this switch when diagnosing the welcome flow.
Use the same mode on both engines.

Account probes send the PROXY prelude immediately, then wait for the username,
password, and complete welcome prompts. They report password-to-welcome and
password-to-MOTD latency separately from process startup. Prompts can span socket
reads and need not end with a newline. A connection-hook failure also releases
the prompt wait so diagnostic commands can still run.

Use `-FixedDelays` to reproduce the old account probe. Guest probes also use that
mode: 3 seconds before input and 2.5 seconds between commands. Commands after
account login retain `-InterCommand` pacing. Both modes default to 20 seconds of
receive silence and at most 60 seconds total (`-Timeout`, `-MaxDuration`).

Each probe prints its evidence prefix. `-CaptureDebug` saves Barn's built-in
expvar metrics and goroutine stacks after the probe. During a blocked probe,
capture `http://127.0.0.1:11486/debug/pprof/goroutine?debug=2` or a CPU profile from
`/debug/pprof/profile?seconds=5`. This distinguishes input dispatch, VM work,
commit-gate waits, and socket problems.
The probe also prints elapsed milliseconds to the banner, username prompt,
guest/account welcome, character selection, guest room, and any authentication
or connection-hook error. `-Timeout` (seconds) must exceed both `-BannerWait`
and `-InterCommand` (milliseconds), or the client stops reading before sending.

For a repeatable workload capture while a client is connected, run
`./scripts/capture-mongoose-debug.ps1`. It saves expvar, goroutine stacks, a
five-second CPU profile, its top functions, and recent warnings/errors through
Barn's `barn_logs` tool. Override `-Seconds`, `-DebugUrl`, or `-RunDir` as needed.
The `*-tasks.txt` report groups CPU samples by `moo.task` and `moo.verb`, so a
busy scheduler callback can be distinguished from login work. Debug logs also
record `slow task slice` entries for slices taking at least 100ms; their elapsed
time includes execution, contention, and cleanup, not just CPU time.
`moo.verb` identifies the task's root verb, including jobs called by a forked
worker; it does not identify the currently executing nested verb. Slow-slice
records include `ticks` for the final execution attempt and `gate_held` when
the slice acquired the exclusive commit gate at any point.
`external command completed` records subprocess elapsed time without arguments
or input. `external task resumed` records `queue_wait` (completion to dispatch)
and `ready_to_vm` (completion to VM entry, including commit-gate contention).
Run `./scripts/summarize-mongoose-exec.ps1 -LogPath <latest.jsonl>` to pair these
records, including repeated external calls from the same task, in milliseconds.
Use `-Commands @('look','inventory','who','north','look','south')` with the login
script for a short exploration pass. An account already connected may show a
character chooser: use `-FixedDelays` with the character selection as the first
entry in `-Commands` before issuing room commands. Prompt login currently assumes
the account's default character can connect without a chooser.

Run the focused generic regressions against both engines with
`./scripts/test-mongoose-deltas.ps1 -Engine Toast` and then `-Engine Barn`.
These use the managed conformance runner and include capability admission.
The selected generic suites live in `tests/mongoose-conformance`; use `-Suites`
to select paths under that directory. Rebuild Barn before testing changed code.
The installed moo-conformance package supplies the managed runner and admission.
Use `-Packaged -Suites @('server/exec.yaml','server/exec_recent_regressions.yaml','builtins/exec_call_shapes.yaml')`
to audit the packaged exec assertions against either engine.
Use `-ConformanceRoot <checkout>` to test a separate conformance worktree
without changing the installed package or its working copy. For example,
`-Packaged -ConformanceRoot .tmp/conformance-mongoose-workload -Suites server/exec_fixture_delay.yaml`
runs the Windows sleep-fixture regression. This uses the managed runner and
capability admission on both platforms.

## Time-offset dependency

Mongoose's `#43:time_offset` runs `executables/tz`; it is not implemented by
the server's time builtin. The fetch script downloads the live Bash helper.
Toast startup installs that helper; Barn startup builds `cmd/mongoose_tz` as
`executables/tz.exe`, with embedded IANA timezone data for Windows. This helper
supports IANA zone names used by Mongoose, rather than arbitrary POSIX TZ rules.

Add `-VerifyTimeOffset` to a wizard account probe to require successful UTC
subprocess output and the actual Denver MOO time-offset call. A missing helper
or a timeout makes the probe fail. `UTC` itself is not accepted by this
checkpoint's MOO timezone whitelist, so the smoke check uses it only for the
external helper. Under heavy background load use `-Timeout 90 -MaxDuration 100`;
a passing result does not imply acceptable latency.

This checkpoint's `#0:server_started` starts SQL services only when `#0:prod()`
is true, which requires a listener on `$network.port` (7777). Use `-Port 7777`
for that startup path and run the engines sequentially to avoid port conflicts.
SQL initialization itself runs in a fork; a login before it finishes can report
`This database is not open` even when `sound.sqlite` is installed correctly.

Use `-Start -UntilCleanLogin -Username <account> -Password <password>` to retry
account connections until the complete welcome hook is observed without a
connection-hook error. The default completion marker is `MESSAGE OF THE DAY:`;
override `-CleanLoginMarker` for another checkpoint. Each attempt saves a
`*-login-timing.json` record. `clean_login_ms` starts at process launch (after
build/copy), and excludes the client's trailing receive timeout. `-LoginDeadline`
defaults to 300 seconds and is checked between attempts; each attempt remains
bounded by `-MaxDuration`. Use identical client waits and marker on both engines.
Prompt-driven clean-login-only attempts close immediately on the welcome marker.
Connection-hook failures retain the receive timeout so retries do not flood a
busy startup. `-RetryDelay` adds one second between failed attempts. These waits
are separate from the reported password-to-welcome latency.

`./scripts/summarize-mongoose-workload.ps1 -LogPath <latest.jsonl> -LastSeconds 600`
summarizes root-verb slow slices and scheduler-reset boundary frequency. Its duty
value includes only slices above the 100ms logging threshold and can include a
slice that began before the selected window; it is not a CPU utilization metric.
Retain the JSON output beside the CPU capture when comparing builds.

`./scripts/measure-mongoose-cycles.ps1 -Engine Barn -Username <account> -Password <password>`
measures ticks and elapsed time for the first three simulation objects on a local
disposable server. Use `-Engine Toast` for the same probe on the oracle. These
calls advance object state: compare identical snapshots and record server age.
Different tick counts on different live world states do not alone prove a
server regression. The script requires a successful result marker.

## Self-description regression

Use `-Commands @('look me',';return $string_utils:pronoun_sub("%s %p %r", player);')`
with the account probe to check pronoun rendering. For q in the pinned September
18 fixture, the description starts with `You look at yourself.` and clothing
uses `He` and `his`; the explicit substitution returns `he his himself`.
`Verb not found` in those positions previously came from a parser rejection of
a conditional expression inside a catch default in the inherited pronoun verb.

The generic regression can be repeated on both engines:

```powershell
./scripts/test-mongoose-deltas.ps1 -Engine Toast -OracleDir /root/src/toaststunt -RunDir <run-directory> -Suites builtins/catch_ternary_fallback.yaml
./scripts/test-mongoose-deltas.ps1 -Engine Barn -RunDir <run-directory> -Suites builtins/catch_ternary_fallback.yaml
```

Build the candidate as `<run-directory>/barn.exe` first. Each managed session
includes capability admission. This check does not establish a clean login:
the separate SQLite connection-hook failure can still precede the description.

## PBT fixture cleanup wait

The September 18 checkpoint's `$pbt:_verify_and_recycle` waits for the entire
server's `queued_tasks()` count to drop to ten before recycling each fixture,
calling `suspend(0)` up to 300 times. Periodic world jobs make this an unrelated
and unreliable prerequisite. In the September 19 reproduction, the original
current checkpoint took 30.75s on Barn and failed three cleanup tests. Mongoose
Toast failed the same three tests in 507.02ms; that faster run was not successful.

Removing only that polling block gave 65/65 passes in 818.50ms on Barn and
1.51s on Mongoose Toast. These are individual live-world runs, not a statistical
comparison. The separate SQLite login-hook error still occurred. Canonical
stock WSL Toast and unmodified Barn both passed the generic managed regression
`builtins/recycle_unrelated_tasks.yaml`, including capability admission.

Apply the guarded repair to a **stopped server's checkpoint**, writing a new file:

```powershell
python scripts/fix-mongoose-pbt-cleanup.py mongoose.db.new mongoose.fixed.db
python scripts/test_fix_mongoose_pbt_cleanup.py
```

The script verifies the exact known block inside the PBT cleanup verb, preserves
all other bytes, prints both hashes, and refuses existing output files or changed
source. Keep the original checkpoint as a backup before selecting the repaired
file for the next server start. Re-fetching an uncorrected remote checkpoint
restores the workaround; this script does not modify mongoose.world.

Repeat the live check with `scripts/mongoose-login.ps1 -Commands @('@test $pbt')`.
Use `-Timeout 55 -MaxDuration 120` so a slow original run's final report is captured.
For this mixed-numeric live workload use the documented Mongoose Toast build;
canonical stock Toast fails additional numeric tests and is not a valid full-suite
performance baseline. Generic MOO reductions still use the canonical oracle.
