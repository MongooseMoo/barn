# Task: Run Conformance Tests and Capture All Failures

## Context
Barn is a Go MOO server being tested against conformance tests from cow_py. The cow_py team has added many new tests. We need a complete picture of what's failing.

## Objective
Run the full conformance test suite against Barn and capture a detailed failure report.

## Steps

1. Build barn if needed:
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
```

2. Kill any existing barn process on port 9300:
```bash
# Check if something is on 9300
netstat -an | grep 9300
# If so, you may need to wait or use a different port
```

3. Start barn server:
```bash
./barn_test.exe -db Test.db -port 9300 > server.log 2>&1 &
```

4. Wait for server startup:
```bash
sleep 3
```

5. Run full conformance test suite, capturing ALL output:
```bash
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9300 -v 2>&1 | tee /c/Users/Q/code/barn/conformance_full.log
```

6. After tests complete, also capture a summary:
```bash
uv run pytest tests/conformance/ --transport socket --moo-port 9300 --tb=no -q 2>&1 | tee /c/Users/Q/code/barn/conformance_summary.log
```

## Output
Write to `./reports/conformance-failures.md`:

1. Total tests run
2. Total passed
3. Total failed
4. Total errors
5. List of each failing test with:
   - Test name
   - Brief description of failure (expected vs actual)
   - Error message if applicable

Group failures by test file if helpful.

## CRITICAL
- Do NOT skip any tests
- Do NOT modify any tests
- Do NOT modify barn code
- Just run and report
- Capture COMPLETE output - we need every failure

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
5. If all formats fail, STOP and report - do not use bash workarounds
