# Task: Build Toast Oracle Tool

## Context
We need a tool to test MOO expressions against ToastStunt as a reference oracle. ToastStunt's emergency mode lets us evaluate MOO expressions.

## What Works
```bash
cd C:/Users/Q/code/barn
cmd //c ".\\toast_moo.exe -e toastcore.db NUL"
```
This enters emergency mode. The `;EXPR` command evaluates a MOO expression and prints the result.

## Required Files
- `C:/Users/Q/code/barn/toast_moo.exe` (already there)
- `C:/Users/Q/code/barn/argon2.dll` (already there)
- `C:/Users/Q/code/barn/nettle-8.dll` (already there)
- `C:/Users/Q/code/barn/toastcore.db` (already there)

## Objective
Create `cmd/toast_oracle/main.go` - a CLI tool that:
1. Takes a MOO expression as argument
2. Starts toast_moo.exe in emergency mode
3. Sends `;EXPR` followed by `quit`
4. Captures and prints the result
5. Exits cleanly

## Usage
```bash
./toast_oracle.exe 'toint("[::1]")'
# Should output: 0

./toast_oracle.exe '1 + 1'
# Should output: 2

./toast_oracle.exe 'typeof("hello")'
# Should output: 2
```

## Implementation Notes
- Use os/exec to run cmd.exe with the toast_moo.exe command
- Write to stdin: `;EXPR\nquit\n`
- Parse stdout to extract the result (skip the banner, find the result line)
- Emergency mode output format: after `(#2): ;EXPR` it prints `=> RESULT`

## Test
After building, verify with:
```bash
./toast_oracle.exe 'toint("[::1]")'
```
Expected output: `0`

## Output
Write completion report to `./reports/build-toast-oracle.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
