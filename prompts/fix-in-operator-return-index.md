# Task: Fix `in` Operator to Return Index

## Context
The `in` operator in MOO should return the **1-based index** of the element in a list, or **0** if not found. Currently Barn returns a boolean (true/false).

## Evidence
```bash
# Toast (correct) - returns position 4:
./toast_oracle.exe '"wizard" in {"programmer", "generic", "Core", "Wizard"}'
# Returns: 4

# Barn (wrong) - returns 1 (boolean true):
# Returns: 1
```

## File to Fix
`vm/operations.go` - the `executeIn()` function around line 298

## Current (Wrong) Implementation
```go
case types.ListValue:
    for i := 0; i < coll.Len(); i++ {
        if element.Equal(coll.Get(i + 1)) {
            vm.Push(types.BoolValue{Val: true})  // WRONG
            return nil
        }
    }
    vm.Push(types.BoolValue{Val: false})  // WRONG
    return nil
```

## Correct Implementation
```go
case types.ListValue:
    for i := 0; i < coll.Len(); i++ {
        if element.Equal(coll.Get(i + 1)) {
            vm.Push(types.IntValue{Val: int64(i + 1)})  // Return 1-based index
            return nil
        }
    }
    vm.Push(types.IntValue{Val: 0})  // Return 0 if not found
    return nil
```

## Also Check
There may be a similar `in` operator in `vm/eval.go` or `vm/operators.go` for the interpreter path. Search for other `in` implementations and fix them all.

## Verification
1. Build barn: `go build -o barn_test.exe ./cmd/barn/`
2. Start server: `./barn_test.exe -db Test.db -port 9700 > server_9700.log 2>&1 &`
3. Test:
   ```bash
   ./moo_client.exe -port 9700 -cmd "connect wizard" -cmd '; return "wizard" in {"programmer", "generic", "Core", "Wizard"};'
   ```
4. Expected: `{1, 4}` (not `{1, 1}`)

5. Also test not found:
   ```bash
   ./moo_client.exe -port 9700 -cmd "connect wizard" -cmd '; return "notfound" in {"a", "b", "c"};'
   ```
6. Expected: `{1, 0}` (not `{1, false}`)

## Output
Write completion report to `./reports/fix-in-operator-return-index.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
