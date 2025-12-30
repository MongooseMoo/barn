# Toast Oracle CLI Tool - Completion Report

## Status: COMPLETE

## What Was Built

Created `cmd/toast_oracle/main.go` - a CLI tool that wraps ToastStunt's emergency mode to evaluate MOO expressions as a reference oracle.

## Location

- Source: `C:/Users/Q/code/barn/cmd/toast_oracle/main.go`
- Binary: `C:/Users/Q/code/barn/toast_oracle.exe`

## Implementation Details

### Architecture

1. **Direct Execution**: Runs `toast_moo.exe` directly (not through cmd shell) to properly handle stdin piping
2. **Stdin Input**: Sends `;EXPR\nquit\n` to emergency mode
3. **Output Capture**: Combines stdout and stderr (Toast writes to stderr)
4. **Result Parsing**: Extracts result from `(#2): => RESULT` format

### Key Functions

- `evaluateExpression(expr string)`: Orchestrates the Toast process execution
- `parseToastOutput(output, expr string)`: Parses emergency mode output to extract result

### Command Structure

```go
exec.Command("C:/Users/Q/code/barn/toast_moo.exe", "-e", "toastcore.db", "NUL")
```

Direct execution (not via cmd shell) ensures stdin is properly piped to toast_moo.

## Test Results

All test cases pass:

```bash
$ ./toast_oracle.exe 'toint("[::1]")'
0

$ ./toast_oracle.exe '1 + 1'
2

$ ./toast_oracle.exe 'typeof("hello")'
2

$ ./toast_oracle.exe '{1, 2, 3}[2]'
2
```

## Usage

```bash
./toast_oracle.exe '<moo-expression>'
```

### Examples

```bash
# Test toint with IPv6 address
./toast_oracle.exe 'toint("[::1]")'

# Test arithmetic
./toast_oracle.exe '1 + 1'

# Test builtin functions
./toast_oracle.exe 'typeof("hello")'
./toast_oracle.exe 'length({1, 2, 3})'

# Test list operations
./toast_oracle.exe '{1, 2, 3}[2]'
./toast_oracle.exe '{1, 2, 3}[1..2]'

# Test string operations
./toast_oracle.exe '"hello" + " world"'
```

## Implementation Notes

### Challenge: Windows stdin Piping

Initial approach using `cmd //c` didn't properly pipe stdin:
```go
// WRONG: stdin not piped through cmd shell
exec.Command("cmd", "//c", ".\\toast_moo.exe -e toastcore.db NUL")
```

Solution: Execute toast_moo.exe directly:
```go
// CORRECT: direct execution pipes stdin properly
exec.Command("C:/Users/Q/code/barn/toast_moo.exe", "-e", "toastcore.db", "NUL")
```

### Output Format

Toast emergency mode produces:
```
LambdaMOO Emergency Holographic Wizard Mode
...
(#2): => <RESULT>
(#2): Bye.  (saving database)
...
```

Parser looks for lines starting with `(#2): => ` and extracts the result.

## Files Created

- `cmd/toast_oracle/main.go` - Source code (83 lines)
- `toast_oracle.exe` - Compiled binary

## Dependencies

Runtime dependencies (already present):
- `C:/Users/Q/code/barn/toast_moo.exe`
- `C:/Users/Q/code/barn/toastcore.db`
- `C:/Users/Q/code/barn/argon2.dll`
- `C:/Users/Q/code/barn/nettle-8.dll`

## Future Enhancements (Optional)

1. **Error handling**: Parse MOO error messages (E_INVARG, E_VERBNF, etc.)
2. **Timeout**: Add configurable timeout for long-running expressions
3. **Batch mode**: Accept multiple expressions via stdin
4. **JSON output**: Option for machine-readable output format
5. **Comparison mode**: Compare against barn output side-by-side

## Verification

Tool is ready for use. All specified test cases pass. The oracle can now be used to validate barn's conformance against ToastStunt's behavior.
