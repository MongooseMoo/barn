# Task: Patch Specs Based on Batch 3 Divergence Reports

## Context

Divergence detection completed for batch 3 (verbs, tasks, time). You need to update the spec files to fix documentation gaps and inaccuracies.

## Divergence Reports to Read

1. `reports/divergence-verbs.md` - 1 spec gap found (prep validation behavior)
2. `reports/divergence-tasks.md` - 3 spec gaps found (callers, queued_tasks, ticks/seconds)
3. `reports/divergence-time.md` - ToastStunt builtins unavailable in Toast

## Spec Files to Potentially Modify

- `spec/builtins/verbs.md`
- `spec/builtins/tasks.md`
- `spec/builtins/time.md`

## Process

1. Read each divergence report carefully
2. For each "spec_gap" classification:
   - Identify the specific section in the spec file
   - Draft the corrected text based on verified Toast behavior
   - Apply the fix using the Edit tool
3. Use `[Not Implemented]` markers for builtins that don't exist in Toast

## Specific Patches Required

### tasks.md - 3 spec gaps

1. **callers() example (line ~76-78)**:
   - Current: Shows 5-element frames without line numbers
   - Fix: Show 6-element frames WITH line numbers (both servers include line numbers by default)

2. **queued_tasks() signature (line ~95)**:
   - Current: `queued_tasks([include_vars]) → LIST` with optional arg
   - Fix: `queued_tasks() → LIST` with no arguments (both servers return E_ARGS with any args)

3. **ticks_left()/seconds_left() behavior**:
   - Add note that these return 0 in eval context (non-forked tasks)

### verbs.md - 1 spec gap + documentation improvements

1. **Prep spec validation**: Document that only individual prep names ("with", "using") are valid in add_verb/set_verb_args, not full slash-separated strings ("with/using")

2. **Undocumented behaviors to add** (from report):
   - Verb indexing: verbs can be accessed by 1-based integer index
   - Verb aliases: space-separated names
   - Prep spec expansion: how verb_args() returns expanded preps
   - set_verb_code() error format: returns list of error strings

### time.md - ToastStunt cleanup

1. Mark unavailable ToastStunt builtins:
   - ftime() - [Not Implemented] or clarify availability
   - strftime() - [Not Implemented]
   - strptime() - [Not Implemented]
   - gmtime() - [Not Implemented]
   - localtime() - [Not Implemented]
   - mktime() - [Not Implemented]
   - server_started() - [Not Implemented]
   - uptime() - [Not Implemented]

## Output

Write summary to: `reports/patch-batch3-summary.md`

## CRITICAL RULES

- ONLY fix spec documentation issues identified in divergence reports
- Do NOT modify any Go code
- Do NOT add new features or behaviors not verified in reports
- Preserve existing spec structure and formatting
- Use `[Not Implemented]` marker style from batch 2 (see spec/builtins/types.md)
- If a report says "clean" or "no spec gaps", do NOT modify that file

## Note on Verbs Report

The verbs report found 7 "likely_barn_bugs" (missing permission checks, database loading issues). These are IMPLEMENTATION bugs in Barn, NOT spec issues. Do NOT modify the spec to match broken Barn behavior. Only fix the 1 spec gap about prep validation.
