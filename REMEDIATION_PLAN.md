# Remediation Plan

## Current State: Broken

The implementation is full of TODOs and unverified claims. It does not work.

## Step 1: Audit Actual Test Pass Rate

```bash
go test ./conformance/... -v 2>&1 | tee full_test_output.log
grep -E "^(ok|FAIL|---)" full_test_output.log
```

Document exactly how many tests pass vs fail vs skip.

## Step 2: Identify What Actually Works

Go layer by layer through PLAN.md. For each layer:
1. Run the specific test files for that layer
2. Record actual pass rate
3. Compare to expected pass rate
4. Mark as BROKEN or WORKING

## Step 3: List All TODOs By Severity

```bash
grep -rn "TODO\|FIXME" --include="*.go" | sort by file
```

Categorize:
- CRITICAL: Blocks core functionality (database writer, command dispatch)
- HIGH: Breaks significant features
- MEDIUM: Missing edge cases
- LOW: Nice to have

## Step 4: Prioritized Fix List

Based on audit, create ordered list:
1. What must work for basic functionality
2. What must work for tests to pass
3. What can be deferred

## Step 5: Fix One Thing At A Time

For each fix:
1. Implement
2. Run tests
3. Verify pass rate improved
4. Commit with test results in message
5. Move to next

No batching. No "I'll verify later." Each fix verified before next.

## Known Critical Issues

1. `server/server.go:147` - No database writer
2. `server/connection.go` - Commands don't dispatch to verbs
3. `cmd/barn/main.go` - Stub that doesn't start server
4. `vm/compiler.go` - 8 unimplemented node types
5. All permission checks are stubs

## Success Criteria

Same as PLAN.md:
1. 905 active conformance tests pass
2. Server loads database and runs
3. Clients connect and execute verbs
4. Snapshots save to disk
