# Research: Barn Mongoose profile and instruction state

## Question

What exact Barn-side tracked changes are required to complete Milestone 0 and
Milestone 1 of `plans/barn-toast-mongoose-convergence-workstreams.md`?

## Scope

- Read the committed convergence plan completely.
- Inspect current tracked Barn instructions, profile/config/manifest code,
  profile tests, and WSL wrapper documentation.
- Verify claims against current source and Git history.
- Ignore unrelated untracked files and do not run servers.

## Output

Write `docs/reports/mongoose-phase0-barn.md` containing:

- verified facts with file:line evidence;
- stale or conflicting instruction surfaces;
- exact profile/config defects involving `PROMOTE_NUMBERS`;
- the smallest coherent Milestone 0 and Milestone 1 Barn file set;
- tests and acceptance gates;
- blockers or risks.

## Constraints

- Do not implement production or documentation changes.
- Modify only the report file.
- Do not stage or commit.
- Do not use forked context or spawn another agent.
