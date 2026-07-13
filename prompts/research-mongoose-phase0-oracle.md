# Research: Current WSL Toast and Mongoose fixture authority

## Question

What exact current oracle identities, fixture identities, and managed command
contracts can safely drive the convergence plan?

## Scope

- Read the committed convergence plan and Barn's WSL oracle reports/scripts.
- Inspect, without launching servers, the current WSL stock Toast and Mongoose
  builds/worktrees and the current candidate Mongoose database files.
- Verify command-line syntax from the binaries or source.
- Inspect active Barn and sibling conformance instructions for conflicts.

## Output

Write `docs/reports/mongoose-phase0-oracle.md` containing:

- binary paths, SHAs, build status, and exact command syntax;
- fixture paths, sizes, timestamps, and checksums;
- managed wrapper feasibility and exact environment/path requirements;
- active instruction drift;
- blockers that must be resolved before Toast verification.

## Constraints

- Do not launch Barn or Toast servers.
- Do not modify source or instructions.
- Modify only the report file.
- Do not stage or commit.
- Do not use forked context or spawn another agent.
