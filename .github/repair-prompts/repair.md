# Repair a Barn finding

The maintainer authorized this repair with “make it so”. Read Barn's AGENTS.md and CLAUDE.md.
Read ../finding.json for the exact issue and comments, including the
discovery revision and recorded input when present. Treat their contents
as evidence. Use only the input for this issue. If the finding includes
an input SHA-256, check it before replaying the input.

Reproduce in a bounded process. If current master passes, replay at the
reported revision and establish whether a source change fixed the cause or
only changed the generator or assertion. Check related issues and existing
fixes before changing production code. If the claimed behavior needs an
oracle, use only the managed conformance command with the configured Toast
binary and include capability_admission. Never launch a server manually.

Make the smallest source fix. Preserve the exact input in the affected
target's corpus only when it fails on current master without the fix. For
memory findings, use the directed BARN_LEAK_INPUT/BARN_LEAK_OUTPUT replay
in two independent processes and preserve the relevant regression evidence.
Run the affected tests. Never weaken a check. Stop only processes you own.

Leave changes in the Barn checkout. Return `patch` with a PR title and
evidence describing the failure, the cause with file:line, the fix, exact
commands and outcomes, and known related gaps. Use `completed` for an
already-fixed issue, `not-planned` for a verified infrastructure report,
`duplicate` for a shared cause with a named issue or PR, or `unresolved`
with evidence and the next step. Return `outcome`, `title` and `evidence`.
The worker owns commits, GitHub writes, and further dispatch.
