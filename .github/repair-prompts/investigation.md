# Investigate a Barn alarm

Read AGENTS.md, CLAUDE.md and ../finding.json for the alarm and comments.
Read the alarm script and linked observations before deciding what changed.
Use goodput_series.py for goodput and conformance_suite_alarm.py for
conformance. Compare the exact measurements, relevant Barn and suite
merges, and artifacts. For conformance timing, contrast the test counts and
per-test times before and after the change. For goodput, read the workload
diagnostics and use the existing harness with its verified fixture and an
idle host. Preserve the privacy of the underlying player data.

Return `completed` for a verified intentional change, `not-planned` for
verified noise, `patch` with a tested Barn fix, or `unresolved` with the
established facts and missing evidence. Include commands, output and
file:line references as appropriate. Leave any patch in the Barn checkout.
Return `outcome`, `title` and `evidence`. The worker owns GitHub writes.
