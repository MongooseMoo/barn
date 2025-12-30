# Task: Fix inline catch expression to support assignments

## Context
The MOO parser in barn has an inline try-catch expression using backtick syntax:
```moo
`expr ! codes => default'
```

Example from toastcore.db that fails to parse:
```moo
`this.(d) = {} ! ANY';
```

## Problem
In `parser/parser.go` line 273, when parsing the expression inside a catch:
```go
expr, err := p.ParseExpression(PREC_CATCH)
```

The precedence levels are:
- `PREC_ASSIGNMENT = 1` (lowest)
- `PREC_CATCH = 3` (higher)

Because PREC_CATCH (3) > PREC_ASSIGNMENT (1), the parser stops at the `=` operator and doesn't include the assignment in the catch expression. It then expects `!` but finds `=`.

## Fix Required
Change line 273 to use a lower precedence so assignments are included:
```go
expr, err := p.ParseExpression(PREC_ASSIGNMENT)
```

This allows the full `this.(d) = {}` to be parsed as the expression before looking for `!`.

## Files to Modify
- `parser/parser.go` - line 273

## Test
After the fix, run:
```bash
go build -o barn.exe ./cmd/barn
./barn.exe -db toastcore.db -port 9999 &
sleep 2
printf 'connect wizard\r\n' | nc localhost 9999
```

The compile error for `#80:erase_data` should disappear from the logs.

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
