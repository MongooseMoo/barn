# Go MOO Server Implementor

You are implementing a MOO server in Go. Work through PLAN.md layer by layer.

## Setup

First, create and switch to a work branch:
```bash
cd ~/code/barn
git checkout -b impl/attempt-{N}  # Replace {N} with attempt number
```

## Resources Available

| Resource | Path | Use For |
|----------|------|---------|
| **Implementation Plan** | `barn/PLAN.md` | Layer-by-layer tasks, done criteria |
| **Language Specs** | `barn/spec/*.md` | Authoritative behavior definitions |
| **Architecture Notes** | `barn/notes/*.md` | Go patterns, directory structure |
| **Conformance Tests** | `cow_py/tests/conformance/*.yaml` | Test cases (shared with Python impl) |
| **Python Reference** | `moo_interp/` | Working implementation to check behavior |
| **C++ Reference** | `~/src/toaststunt/` | ToastStunt source for edge cases |

## How To Work

1. **Read the current layer** in PLAN.md
2. **Read the spec refs** listed for that layer
3. **Implement** the tasks
4. **Run tests** per done criteria
5. **Update progress checklist** in PLAN.md (check the box)
6. **Commit** after each layer: `git commit -m "Layer X.Y: description"`
7. **Move to next layer**

## When Stuck

If you cannot proceed, STOP and report:

```
BLOCKED at Layer X.Y

## What I tried
[Describe what you attempted]

## What failed
[Exact error or issue]

## What's missing from spec/plan
[What information would unblock you]

## Suggested fix
[How to fix the spec/plan, if you have ideas]
```

Do NOT:
- Guess at unspecified behavior
- Invent APIs not in the spec
- Skip layers
- Continue past a blocker

## Phase Gates

At each **GATE** marker, stop and report status:

```
GATE: Phase N Complete

## Layers completed
[List]

## Tests passing
[Count and command output]

## Issues encountered
[Any workarounds or spec clarifications needed]

## Ready for Phase N+1
[yes/no]
```

## Commit Discipline

- Commit after each layer completes
- Commit message format: `Layer X.Y: brief description`
- Never commit broken code
- Keep commits small and focused

## Starting Point

Begin with:
1. Read PLAN.md "Progress Tracking" section
2. Find the first unchecked layer
3. Start there

If this is attempt 1, start at Layer 0.1.

## File Modified Error Workaround

If you see "File has been unexpectedly modified":
1. Read the file again with the Read tool
2. Retry the Edit
3. If still fails, try different path formats (forward/back slashes)

## Success Criteria

You have succeeded when:
- All layers through the assigned phase are checked
- All done criteria pass
- Gate verification command succeeds
- Code compiles with no errors
