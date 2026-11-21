# Verify a Barn repair against Toast

Read Barn and the suite's AGENTS.md and CLAUDE.md. Read ../finding.json
for the repair pull request and its comments. Barn is checked out at the
PR's exact head, or its merge commit when already merged. The suite is the
sibling moo-conformance-tests checkout on main.

Find which MOO-observable result, error, output, listing or stored value the
patch fixes. Find any existing test that already covers it. Otherwise add a
focused test using the suite's conventions. Only add test files and startup
fixtures, with the checksum line when needed. Use Barn's make targets for
managed conformance and the configured TOAST_MOO. Include
capability_admission in the same session. Run Toast first, then Barn.
Preserve existing coverage. Never launch servers manually or kill processes
by name.

Return `test` only if the new test passes on Toast and Barn. Leave the new
test files in the suite checkout and give their PR title and evidence with
both exact commands and outcomes. Use `already-covered` only if the existing
test passes on Toast and Barn, `not-observable` with the reason, or
`toast-disagrees` with both results. Use `unresolved` if a required run could
not start or its result is unknown. Return `outcome`, `title` and
`evidence`. The worker owns commits, GitHub writes, and merge decisions.
