# Task: Fix listeners() builtin to match ToastStunt

## Context
The `listeners()` builtin in Barn is currently hardcoded to always return a fake listener. This causes the MCP initialization code to run when it shouldn't, leading to E_VERBNF errors during login.

## Objective
Fix `builtins/network.go:builtinListeners` to match ToastStunt's actual behavior.

## Research Required
1. Check ToastStunt source at `~/src/toaststunt/` for how `listeners()` is implemented
2. Understand what data structure listeners actually returns
3. Understand when listeners should return empty vs populated list

## Files to Modify
- `builtins/network.go` - the `builtinListeners` function

## Key Points
- Do NOT hardcode values
- The function should return actual listener information from the server
- If no listeners are registered for an object, return empty list
- May need to add infrastructure to track actual network listeners

## Test
After fix, connecting to barn with toastcore.db should NOT trigger MCP code path (since there's no real listener registered on #0 during login).

## Output
Write findings/status to `./reports/fix-listeners-builtin.md` when done.
