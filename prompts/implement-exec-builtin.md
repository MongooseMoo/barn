# Task: Implement exec() builtin

## Context
Barn is a Go MOO server. The `exec()` builtin is not implemented, causing 7 test failures.

## What exec() does
The `exec()` builtin executes an external command and returns its output. In ToastStunt:
```
exec(command [, args...]) => {output_lines, exit_status}
```

## Test Commands
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/

# Start server
./barn_test.exe -db Test.db -port 9600 > server_9600.log 2>&1 &

# Test exec
./moo_client.exe -port 9600 -timeout 5 -cmd "connect wizard" -cmd "; return exec(\"echo\", \"hello\");"

# Run conformance tests
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9600 -v -k "exec" -x
```

## Reference
Check ToastStunt behavior:
```bash
cd /c/Users/Q/code/barn
./toast_oracle.exe 'exec("echo", "hello")'
```

## Key Files
- `barn/builtins/` - where builtins are implemented
- `barn/builtins/registry.go` - builtin registration

## Output
Write findings and implementation to `./reports/implement-exec-builtin.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
