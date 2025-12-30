# Task: Investigate Why Barn Doesn't Show Tracebacks to Players

## Objective
Find where in Barn's code errors should be sent to the player connection, and why they aren't.

## Context
When MOO code errors during execution, the player should see a traceback. Toast shows these. Barn is silent - errors are swallowed somewhere.

## Investigation Steps

1. Search for how Toast/MOO sends errors to players (look for "traceback", "notify", "error" in context of player output)

2. In Barn's Go code, find:
   - Where runtime errors are caught
   - How/if they're sent to player connections
   - The notify() builtin implementation

3. Check if there's a disconnect between:
   - Error occurring
   - Error being formatted as traceback
   - Traceback being sent to player's connection

## Key Files to Check
- `builtins/` - especially notify, output-related
- `vm/` - error handling, task execution
- `server/` or `net/` - connection handling

## Output
Write to `./reports/investigate-barn-tracebacks.md`:
1. How errors SHOULD flow to player (based on Toast/spec)
2. Where Barn's error flow breaks
3. Specific file:line where fix is needed

Do NOT write code. Just identify the gap.
