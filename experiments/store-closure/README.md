# Store closure experiment evidence

Base implementation: b91e36f5, baseline checkout note-only HEAD 98779472.
Candidate: experiment/store-closure; exact candidate commit recorded with results.
Existing opt-in Mongoose workload is used for application-level measurement,
not as a local CI gate or isolated traversal benchmark. Preserve invalid runs.
Go 1.27.1 windows/amd64. Players 1,4; warmup 2s; measurement 8s; drain 10s;
GOGC 400; same database and sound catalog for both worktrees.
No production speed claim follows from one sample or from unrelated benchmarks.

## Diagnostic baseline outcome

The baseline workload completed with exit 0 / PASS in 35.03s, but logged home
commands failing with uncaught E_NACC. This is not valid throughput-parity evidence.
No candidate throughput comparison is claimed. Repairing the database workload
is outside this traversal consolidation; no fixture or permission changes made.

Database SHA256: EE84F1E848A4A2D22C2BCCE1340D19F9CEC7BDB752B416355D6AD4831E8F4F79.
Sound SHA256: 250B9B69FAE2DAD6788061A1ABB029332F564B92E630514C93181030F7277CA9.
Diagnostic settings additionally enabled BARN_RUNTIME_DIAGNOSTICS=1, debug log
capture and a Go CPU profile. These instrumented timings are not an A/B result.
Local-only raw artifacts: baseline-diagnostic.cpu and
baseline-diagnostic-logs/latest.jsonl (6,041,291 bytes at capture).
Console output exceeded tool capture limits; the full console stream is not
preserved. The raw structured diagnostic log and CPU profile remain on disk.

Observed failure excerpt:

    home -> uncaught:E_NACC task=3367
    home -> uncaught:E_NACC task=3424
    --- PASS: TestMongooseRealWorkload (35.03s)

CPU profile: duration 29.95s, total samples 83.37s; persistentWaifRootSet and
liveWaifs each appeared with 1.95s cumulative samples. This includes work under
those functions, not isolated traversal cost. Do not infer a speed result.
