# Live Mongoose admission measurement

## Scope and protocol

Compare baseline `83751cc` with admission implementation `b4120d8`, using native Windows Barn, the same copied Mongoose database, and `GOMAXPROCS=1` and `16`. The new build also sets `ADMISSION_LIMIT` to that value. Baseline foreground execution is intentionally not capped by admission: this comparison measures the actual policy change.

Database SHA-256: `c44051f14006778f1c7185f8914fd08ca2ca629119c3bb5ec0d4c9db3c0e039d`.

Host: AMD Ryzen 9 5950X, 16 cores / 32 logical processors, Windows 11 Pro 10.0.26200, Go 1.26.0 windows/amd64. Go parallelism is constrained by the environment, not CPU affinity. Other desktop activity is not controlled; the large regression is distinct from the small multi-thread differences.

The binaries embed revisions `83751cc2fc951e96395b4562be996dc23c5ec0b3` and `b4120d84110726981a9c738fd0c64500d6d43765`; both embed `vcs.modified=true`. This is a local branch comparison, not a clean-release build certification. The [machine-readable summary](mongoose-admission-summary-20260918.json) retains the exact binary and input hashes. No Barn runtime source was changed during measurement.

Each trial uses its own writable database, sound database and timezone executable. Later trials also use standalone SQLite backups of the earlier working run's `news.sqlite` and `log.sqlite`. Every input SQLite file and executable is hashed in the trial summary. The prior instrumented Barn process is temporarily suspended during each experiment and resumed afterward; its database and in-memory state are preserved. No benchmark server shares that process's files or ports.

The final command experiment uses three paired repetitions, alternating baseline/new order by repetition. Each authenticated session waits ten seconds, drains prior output through a completion marker, then issues `look`, `@who`, and a queued-task count in rotation for thirty seconds, with a 100 ms pause between completed probes. Every probe includes a second MOO command returning a unique marker, so latency includes the operation and that marker's processing. TCP_NODELAY avoids a client-side delayed-ack floor. This is one interactive player amid the real database's background workload, not sixteen simulated players or a maximum-throughput test.

CPU and debug metrics are sampled around the command window. Structured logs retain warnings/errors, irreversible-boundary waits/retries, and completed `s_run` slices exceeding the existing 100 ms logging threshold. These slow-slice observations are not complete background throughput or a starvation proof; slices can begin before the measurement window. Gate-wait expvars exist only on the new build, so old/new total wait is not directly comparable. Checkpoints are requested after the command window and judged by the server's terminal success/failure log event.

Three runs are exploratory evidence, not a statistically established speedup. Report per-run tails and variability; do not turn many commands from one process into independent startup trials. Failed login, command and checkpoint outcomes remain visible.

## Startup and fixture checks

- The first pilot incorrectly treated the debug HTTP listener as MOO readiness. Both connection attempts were refused; those trials are invalid. The harness now waits for Barn's actual MOO-listening event without opening a probe connection.
- With one-second login retries and a 180-second clean-login deadline, both concurrency-1 builds failed: baseline made 132 attempts and new made 27. Both repeatedly returned `Confunc failed: This database is not open.` Their MOO listener times were 12.623 s and 11.274 s respectively. A listening port is not a usable-login measurement.
- Baseline concurrency 16 also failed the deadline with five-second retries (32 attempts). Repeating that baseline check with frozen news/log SQLite backups still failed. This does not support attributing the startup failure solely to absent support files.
- The initial repeated matrix was stopped to check that fixture difference. Its unfinished new-16 trial is censored and excluded. The prior suspended process was explicitly resumed successfully.
- A command pilot confirmed that the failed connection hook nevertheless left the account authenticated and able to execute commands. Its command window produced median 7.260 ms, p95 29.831 ms and maximum 1301.887 ms at baseline concurrency 16. The checkpoint reply-marker probe was invalid because this database's semicolon command does not execute an arbitrary list of statements. Its command data is pilot evidence only; the final matrix uses a single checkpoint request and server log events.
- That pilot independently recorded an actual checkpoint failure: `write queued task 43199186: write rtenv: runtime environment has 3 names for 38 values`. Do not confuse this server error with the harness's subsequent missing-marker timeout.

Final sessions explicitly allow the authenticated-but-unclean state. This permits a responsiveness comparison but does **not** establish successful login or usable Mongoose convergence. Clean-login results and command-session timings must not be combined.

## Final results

All twelve planned trials completed in `.tmp/admission-measure-final/results.json`. Numbers below retain the three independent runs in order; units are milliseconds.

| GOMAXPROCS | Admission | Build | Median, each run | p95, each run | Maximum, each run | Probes |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | none | baseline | 26.52, 25.77, 30.36 | 87.49, 94.66, 93.16 | 733.02, 802.64, 852.24 | 635 |
| 1 | 1 | new | 842.60, 1494.78, 1232.73 | 5615.65, 6033.07, 6737.91 | 5615.65, 6033.07, 6737.91 | 49 |
| 16 | none | baseline | 7.34, 7.40, 7.20 | 27.74, 30.23, 24.40 | 759.88, 758.36, 784.18 | 738 |
| 16 | 16 | new | 7.25, 6.45, 6.92 | 23.88, 23.92, 25.12 | 983.62, 656.93, 708.11 | 754 |

At one Go execution slot, the median of run medians rose from **26.52 ms to 1232.73 ms** (about 46x); the median of run p95s rose from **93.16 ms to 6033.07 ms** (about 65x). Small sample counts on the slow side make its p95 equal the maximum; the median regression is also large in every repetition. Both builds consumed approximately one CPU core. The new build recorded more completed slow `s_run` slices (43/33/31 versus 21/18/14), consistent with increased background progress but insufficient to quantify total throughput.

At sixteen Go execution slots, median and tail differences are small relative to the single-slot regression. Background slow-slice counts and CPU use are similar (baseline about 1.42–1.45 cores, new 1.40–1.44). This small three-run experiment does not establish a general speedup.

Every final session was explicitly unclean. Every checkpoint failed with the same environment-length error quoted above (**0/12 successful checkpoints**). Error text appeared in three probe responses in each group except new admission-1 (zero); these are heuristic error markers, not full command correctness assertions. The windows overlap real startup activity and are not proven steady state.

The first new admission-1 trial accumulated only **0.5373 ms total commit-gate wait** during a 30.64-second command window, while p95 command latency was 5.62 seconds. That does not explain the latency as commit-gate contention. Source mechanism: admission-1 holds the only execution reservation through the entire cooperative VM slice; the baseline permits foreground/background goroutines to interleave under the Go scheduler even with GOMAXPROCS=1. The evidence points to coupling admitted invocations to CPU parallelism.

The follow-up held GOMAXPROCS at one and raised admission to two, using the same executable and inputs. This changes configuration only, including the default per-principal cap inherited from the global cap.

| Admission | Median, each run (ms) | p95, each run (ms) | Maximum, each run (ms) |
| --- | --- | --- | --- |
| 2 | 27.65, 33.32, 32.57 | 102.97, 108.07, 94.52 | 1189.77, 737.62, 1215.96 |

The median of run medians is **32.57 ms**, and the median of run p95s is **102.97 ms**. The multi-second delay disappears, although the small experiment does not prove baseline equivalence. These trials are in `.tmp/admission-measure-capacity2/results.json`; they were conducted after the paired matrix, not interleaved with its baseline runs. The existing instrumented process was resumed after both experiments.

**Acceptance: reject the current single-slot default as a responsiveness improvement.** The next policy change should separate admission capacity from Go CPU parallelism; capacity two is a measured mitigation for this workload, not a universal optimum or proof of fairness under many players. The startup database-not-open error and queued-task checkpoint failure also need their own fixes. Neither issue was repaired or hidden by this measurement work.

Final artifact audit: 15 completed trials, one database hash, one set of SQLite input hashes, zero incomplete trials, zero clean logins and zero successful checkpoints. Both Python scripts compiled. The prior instrumented process resumed and accumulated 1.047 CPU seconds during a one-second check; no benchmark server was left running.

## Reproduction

Set `MONGOOSE_USERNAME` and `MONGOOSE_PASSWORD` in the process environment. They are not included in the manifest or sent-command logs. Use frozen input files; do not point the support directory at a live SQLite database with WAL sidecars.

```powershell
python scripts/measure-mongoose-admission.py --baseline .tmp/shared-admission/barn-base.exe --new .tmp/shared-admission/checkpoint/barn.exe --database .tmp/mongoose-fable-20260918/mongoose.db.new --sound .tmp/mongoose-fable-20260918/sound.sqlite --tz .tmp/mongoose-fable-20260918/barn/executables/tz.exe --support-dir .tmp/admission-sqlite-snapshot --output .tmp/admission-measure-final --repeats 3 --limits 16 1 --seconds 30 --accept-unclean
```

Omit `--accept-unclean` to require the MOTD clean-login marker. `--retry-delay` defaults to five seconds. `--pause-pid` is optional: verify the process's executable and identity first; it resumes in a `finally` block. Create the experiment output directory's `STOP` file to stop gracefully at the next supported boundary (an in-progress socket wait can take up to sixty seconds). Use a new output directory for every experiment; existing trial directories are refused.

For the capacity-two follow-up, use the same inputs with `--labels new --limits 1 --admission-limit 2 --output .tmp/admission-measure-capacity2`. Summarize both experiments without pooling trials:

```powershell
python scripts/summarize-mongoose-admission.py .tmp/admission-measure-final/results.json .tmp/admission-measure-capacity2/results.json --json-out reports/mongoose-admission-summary-20260918.json
```

Trial artifacts include the read-only received transcript, input hashes, settings manifest, debug snapshots, structured server logs, command samples, checkpoint outcome and errors. Initial experiments are retained separately under `.tmp/admission-measure-pilot*`, `.tmp/admission-measure-repeated`, `.tmp/admission-measure-support`, and `.tmp/admission-measure-commands`.
