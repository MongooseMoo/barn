# Task: Fix Traceback Line Numbers to Skip Comment-Only Lines

## Problem

Barn's traceback shows wrong line numbers. When an error occurs, the traceback reports "line 1" but line 1 is just a doc string:

```moo
"Usage: news [contents] [articles]";   // line 1 - doc string
"";                                      // line 2 - doc string
"Common uses:";                          // line 3 - doc string
...
set_task_perms(caller_perms());          // actual code starts here
```

The traceback should report the line number of actual code, not count doc-string lines.

## Context

In MOO, lines like `"some string";` are often used as documentation comments. They're technically executable (evaluate string, discard result) but they're essentially no-ops used for documentation.

When reporting errors, line numbers should either:
1. Skip lines that are just string literals (comment-only lines)
2. Or ensure line numbers accurately reflect where actual errors occur

## Where to Look

- Parser/AST code - how lines are numbered
- VM execution - how line numbers are tracked during execution
- Error/traceback generation - how line numbers are reported

## Test Case

The `#6:news` verb in toastcore.db starts with several doc-string lines. When an error occurs deeper in the call chain, the traceback should show the actual line number of real code, not "line 1".

```bash
cd ~/code/barn
./moo_client.exe -port 9600 -timeout 10 -cmd "connect wizard" -cmd "news"
```

Current output shows `#6:news line 1` but line 1 is just a doc string.

## Output

Write findings and fix to `./reports/fix-traceback-line-numbers.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
