# Research compatible input bounds (#560)

## Goal

Deliver issue #560: inspect the enabled canonical Toast input implementation and verify overlong-line/unfinished-Telnet behavior with the documented managed oracle. Produce a bounded Barn policy proposal accounting for normal text, binary input and Telnet framing. Q authorized executing the issue; this is a research deliverable, not authorization to choose new MOO-visible overflow behavior.

## State

Branch research/input-bounds-560 in C:/Users/Q/code/barn-input-research-560, isolated from the dirty main checkout. Source inspection established the canonical 5 MiB scale, pre-read text check, per-byte Telnet check and reset-per-read binary mode. The attempted managed run failed collection, so no behavior/admission verdict exists. The delegated research agent then failed with a possible-cybersecurity-risk filter message. Root is preserving the partial result without retrying rejected work. Report: docs/reports/input-bounds-560.md.

## Decisions and why

Use managed conformance only, including capability_admission with every focused run. Do not launch manual servers or run against tracked database files. Preserve raw histories/manifests/output. Read enabled build configuration before treating optional Toast branches as behavior.

## Surprises

The initial witness incorrectly put OUTBOUND_NETWORK in requires.config; the saved XML records the unsupported-name collection failure. Root corrected that existing metadata to log_file plus option.OUTBOUND_NETWORK skip conditions after reading the schema. The correction has not been rerun. The failed agent did not retain exact command/stdout, admission JSON or a server log. Do not claim oracle verification from its progress message.

## Next step

Publish the partial source report as a draft PR referencing #560 without closing it. Existing CI can validate repository integration, not replace missing oracle/admission evidence. Preserve the platform refusal and incomplete result, continue independent repair issues, and do not implement an unsettled input-limit policy.

## Checkpoint

- Root independently verified source/build configuration and retained selected source evidence.
- The managed attempt produced oracle-initial.xml with a collection error, not test results; no boundary assertion ran.
- Fixture metadata is corrected in place, but rejected research work has not been retried.
- Report and prepared witness are partial; #560 remains open and no Barn input policy has changed.
- Next: inspect and publish the partial report as draft, then finish existing CI and the independent repairs.

## PR draft

Title: research: record input-bound source findings and blocked oracle evidence

Body: Record the canonical Toast input checks, prepared managed witness and failed collection artifact for #560. The source distinguishes a pre-read text threshold from per-byte Telnet checks and reset-per-read binary input. The managed attempt failed collection; a subsequent platform safety filter stopped the agent. No successful oracle/admission result or Barn input-limit change is claimed. Refs #560; acceptance remains incomplete.
