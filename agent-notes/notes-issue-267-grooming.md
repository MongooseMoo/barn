# Issue 267 grooming checkpoint — 2026-09-02
- Key finding: scope property-facet COW to property-only transaction staging; do not weaken the global deep-clone boundary.
- Current state: strategy plan merged via PR #281; PR #282 is unrelated repository housekeeping and remains open.
- Blocker: none; the issue wording still conflates property-facet cloning, global verb sharing, and a persistent map.
- Next action: revise #267 acceptance criteria around a property-private staging helper, mixed-write upgrade safety, and 16-player before/after evidence.
