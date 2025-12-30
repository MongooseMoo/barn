# Task: Skip String-Only Lines When Computing Line Numbers

## Problem

Line numbers in tracebacks include lines that are just string literals (doc comments). They shouldn't.

## The Fix

When computing/assigning line numbers, skip any line that is ONLY a string literal expression.

Example:
```moo
"doc string";      // skip - don't count this line
"";                // skip - don't count this line
x = 1;             // this is line 1
"mid-verb doc";    // skip - don't count this line
y = x + 1;         // this is line 2
```

## Where to Fix

This is likely in the parser or compiler - wherever line numbers are assigned to statements/bytecode. NOT a runtime fix.

Look for where the parser processes statements and assigns line numbers. When a statement is just a string expression (ExprStmt containing only a StrLiteral), don't increment the line counter.

## Test

After fix:
```bash
cd ~/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db toastcore_barn.db -port 9700 > server.log 2>&1 &
sleep 3
./moo_client.exe -port 9700 -timeout 10 -cmd "connect wizard" -cmd "news"
```

The error should show a real line number (not line 1 which is a doc string).

## Output

Write brief report to `./reports/fix-line-numbers-skip-strings.md`

## CRITICAL

- This is a SIMPLE fix - don't overcomplicate
- Skip string-only lines when counting, that's it
- Don't add runtime line tracking - fix line number assignment
