# Current checkpoint 28

- Managed fixture provenance is implemented and focused-green: three startup recipe DBs rebuild all three checked-in fixtures under WSL Toast, wait for natural exit code 0, and byte-compare exact output (`3 passed in 5.20s`).
- Behavioral panic suite now waits for natural process exit code `-6`; focused managed Toast proof passed (`1 passed in 5.91s`). Recycle and nested behavior suites use exact exit code 0.
- Current temporary action: a 60-second post-assertion pause was just re-added to the panic behavior suite solely to inspect the new deterministic panic dump. It must be removed after recording the exact suspended-task serialization.
- Next: run that managed panic inspection, add exact WAIF class/owner and anonymous object/owner assertions plus missing/corrupt false-positive unit regressions; update startup checksums and workflow provenance step; run local gates and full four-profile Toast matrix; commit/push/update draft PR #45 and issue #44.
- No blocker. No Barn source edits, no PR #22 overlap, and no manual Toast process was used.
