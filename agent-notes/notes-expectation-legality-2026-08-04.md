# Current expectation legality audit

- State: PR #22 head `638a19772407e6c707d350c2217edaf89fc26f78`; issue-12 worktree clean.
- Current schema accepts every expectation field on every action, but runner routes differ: result routes enforce result fields, raw-output routes enforce only `output`, and side-effect routes ignore `expect`.
- Tracked action inventory: result expectations occur on traditional tests, `run`, and `verb_setup`; raw output occurs on `command`, `send`, and `read_connection`; all 421 cleanup steps are `run` with optional `as` and no expectations.
- Current tracked expectations have no explicit nulls. Two empty-output assertions are intentional and must stay legal: `output: {exact: []}` and `output: []`.
- Blockers: `{value}` satisfies binding collides with table substitution and cannot round-trip anonymous objects across a separate eval; Barn eval notifications occur before the transport prefix and are dropped.
- Toast surface: 11,569 collected; 11,431 passed; 138 skipped. No YAML parameter is omitted from collection, but 138 bodies do not execute.
- Next action: deliver a fail-closed action/expectation matrix, combination rules, null/empty-output contract, anon handling, and focused regression inventory to the parent agent.
