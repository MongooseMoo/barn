# Task: Fix add_property() E_INVARG Error

## Context
Barn is a Go MOO server. The `add_property()` builtin returns E_INVARG when called by a programmer.

## The Bug
```
connect programmer
; return eval("add_property(#0, \"temp\", 0, { #3, \"rwc\" }); return 0;");
{1, {0, E_INVARG}}
```

Expected: `{1, {1, 0}}` - add_property should succeed

## The Call
```
add_property(#0, "temp", 0, { #3, "rwc" })
```

Arguments:
- object: #0 (system object)
- propname: "temp"
- value: 0
- info: {#3, "rwc"} - owner #3, perms "rwc"

## Files to Check
- `builtins/properties.go` - add_property implementation

## What to Investigate
1. Is the info argument `{#3, "rwc"}` being parsed correctly?
2. Is there a permission check rejecting programmers incorrectly?
3. Is there argument validation that's too strict?

Compare against ToastStunt's add_property signature and behavior.

## Test Command
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9250 &
sleep 2
printf 'connect programmer\n; return add_property(#0, "temp", 0, { #3, "rwc" });\n' | nc -w 3 localhost 9250
```

Expected: `0` (success, add_property returns 0 on success)

## Output
Write findings to `./reports/fix-add-property-invarg.md`

## CRITICAL: Do NOT modify tests
The tests are correct. Only fix barn implementation.

## CRITICAL: File Modified Error Workaround
If Edit/Write fails:
1. Read the file again
2. Retry the Edit
3. Try path formats: `./builtins/properties.go`, `C:/Users/Q/code/barn/builtins/properties.go`
4. NEVER use cat, sed, echo
