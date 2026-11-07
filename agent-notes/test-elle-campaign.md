# Elle expansion campaign

## Goal
Execute Q's approved Elle expansion plan using isolated worktrees and independent
PRs, through CI, normal merges and cleanup. Authorization: 2026-11-06, "using
subagents, fully execute. many prs. worktrees."

## State
The coordinator owns this worktree. Three workers own independent definitions,
maps/permissions and boundary worktrees; workers commit and draft, root publishes.
Pinned baseline workflow 42344496116 completed successfully. All six healthy
model/seed verdicts were true with exit zero, all ten faults false with exit one,
and results.json errors was empty. Seed 412 reported nonzero selected-path counts.
Raw evidence: `C:/Users/Q/code/barn-elle-evidence/baseline/elle-evidence`.
The primary checkout's unrelated work remains outside this campaign.

## Decisions and why
- Preserve existing serializable model, raw histories and negative controls.
- Add manual seed/round inputs to the existing workflow so larger campaigns use
  the same pinned checker and retained-artifact path as PR checks.
- Keep ordinary PR defaults unchanged. Do not add recurring schedules.
- Workflow actionlint completed; offline zizmor's high/medium findings were
  repaired by pinning existing action major versions and disabling checkout
  credential persistence. Two low advisory findings remain for the existing
  local composite-action syntax; audit command exits zero.
- Generate richer transaction shapes after the deterministic additions integrate.
- Require real MOO observations and demonstrated rejecting faults; coverage counts
  do not stand in for correctness witnesses.

## Workstreams
1. Definitions, inheritance, dispatch: worker elle_definitions.
2. Map transitions and ordinary-user permissions: worker elle_maps_permissions.
3. Renewal/publication and recorder boundaries: worker elle_boundaries.
4. Manual campaign controls and broader generation: coordinator.

## Surprises
The local primary master is behind origin/master and dirty. All implementation
branches start at freshly fetched origin/master without moving the primary tree.

## Next step
Deliver manual campaign inputs as the first small PR while workers finish
source-driven schedules. Inspect and publish each worker
branch independently against master, resolving shared-file changes after merges.
