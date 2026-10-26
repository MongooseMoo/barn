# Research #515: historical database compatibility

## Goal

Produce a source-backed compatibility matrix and reuse recommendation for db/format. Own documentation only; no parser implementation, publishing, push, issue comments or PR creation. Parent owns source verification and publication. Smallest deliverable: one report of approximately 130 lines and this task note, using existing source and fixtures.

## State

Report: `docs/reports/db-format-compatibility.md`. Pinned source observations cover versions 0–17, same-version build variants, seven Barn fixture hashes, three named reuse candidates and remaining historical provenance gaps. No managed oracle, runtime compatibility, performance or CI test run was performed. No implementation files changed in this worktree.

## Decisions and why

- Recommend improving the existing Go package. Python moodb has dispatch for 1–4/17, mooR explicitly skips tasks and WAIFs, and the extractor skips to object records; none is an evidenced lossless Go replacement.
- Keep #517's layout boundaries explicit: upstream changes object representation at 5, section order at 13, last_move at 15 and activation fields at several other versions.
- Treat old queued activation and anonymous-program attachment differences as questions for #516's producer-backed investigation. Static discrepancy alone is not an oracle verdict.
- Report missing original producers/builds and uninspected fork codecs explicitly. Historical fixture recovery remains follow-up work; do not equate a source matrix with universal support.

## Surprises

LambdaMOO version 4 does not distinguish Codepoint/Unicode, number-width, float or optional-WAIF variants. The maintained Python reader rejects 5–16 despite having version-dependent helpers. Extractor native parsing scans for a numeric line followed by #0 rather than decoding preceding saved tasks.

A GitHub ChangeLog lookup used the wrong filename and failed; the initial read wrapper then displayed stale decoded content. That block was discarded. The corrected probe checked the native API exit code before decoding, resolved `ChangeLog.txt` from the repository tree and read the correct source. A manually transcribed Broken4 hash omitted one character; comparison with the actual file hash identified it and the report was corrected. No compatibility inference relies on either erroneous output.

## Next step

Parent should verify the pinned functions/lines before publishing the PR draft below. Retain the issue's explicit corpus/provenance gaps; no successful-load claim is available. Parent has received the layout conclusions for #517. A higher-priority checkpoint hook required a separate uncommitted `C:/Users/Q/code/barn/notes-format-research-checkpoint.md`; it is outside this worktree and is not part of this deliverable.

## PR draft

Title: docs: map MOO database compatibility and parser reuse

Body:

Records source-backed format transitions, same-version build differences, existing fixture hashes, and reuse limits of moodb, mooR and the ToastStunt extractor. Recommends keeping Barn's codec and explicit layout boundaries while addressing current preservation questions first.

Research deliverable for #515. Historical producer/build provenance and corpus execution remain explicitly unresolved; this does not claim universal compatibility or successful checkpoint restoration. Validation was source inspection and SHA-256 comparison of existing fixtures; no CI steps were run locally. Documentation only.
