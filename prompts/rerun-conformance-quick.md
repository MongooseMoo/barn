# Task: Re-run Conformance Tests After Fixes

## Context
We just fixed the limits registration issue. Need to re-run tests and get updated failure counts.

## Objective
Get updated pass/fail counts and identify remaining failures.

## Steps

1. Ensure barn server is running on port 9300:
```bash
cd /c/Users/Q/code/barn
# Check if running
./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return 1 + 1;"
# If not running, start it:
./barn_test.exe -db Test.db -port 9300 > server.log 2>&1 &
sleep 2
```

2. Run conformance tests with summary output:
```bash
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9300 -q 2>&1 | tee /c/Users/Q/code/barn/conformance_rerun.log
```

3. Also capture just the failing test names:
```bash
uv run pytest tests/conformance/ --transport socket --moo-port 9300 --tb=no -q 2>&1 | grep FAILED | tee /c/Users/Q/code/barn/failing_tests.log
```

## Output
Write to `./reports/conformance-rerun.md`:
1. New pass/fail/skip counts
2. Improvement from previous run (was 161 failed)
3. List of remaining failing tests grouped by file/category

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
5. If all formats fail, STOP and report - do not use bash workarounds
