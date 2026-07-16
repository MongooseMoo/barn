# Research: Mongoose conformance harness and coverage

## Question

What exact current conformance changes are required for truthful Mongoose
profiles and a managed Toast-first real-Mongoose login/look gate?

## Scope

- Read the committed Barn convergence plan completely.
- Inspect `C:/Users/Q/code/moo-conformance-tests` profile gating, managed server
  lifecycle, login-script handling, schema/steps, existing PROXY/login/hold-input
  tests, and test organization.
- Identify reusable surfaces and avoid proposing a new helper when an existing
  harness feature suffices.
- Do not run servers.

## Output

Write `C:/Users/Q/code/barn/docs/reports/mongoose-phase0-conformance.md`
containing:

- verified facts with file:line evidence;
- exact profile-gate gaps;
- exact existing coverage versus missing coverage;
- smallest proposed YAML/harness changes for the first real-Mongoose gate;
- required tests and managed command shapes;
- blockers or risks.

## Constraints

- Do not implement changes in either repository.
- Modify only the report file in Barn.
- Do not stage or commit.
- Do not use forked context or spawn another agent.
