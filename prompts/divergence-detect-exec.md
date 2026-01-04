# Task: Detect Divergences in Exec Builtins

## Context

We need to verify Barn's exec builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all exec/system builtins.

## Files to Read

- `spec/builtins/exec.md` - expected behavior specification
- `builtins/exec.go` - Barn implementation (if exists)

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Process Execution
- `exec()` - execute external command
- `shell()` - shell command execution (if exists)
- `system()` - system command (if exists)

### Process Control
- `kill()` - kill process (if exists)
- `wait()` - wait for process (if exists)

## Edge Cases to Test

- Command not found
- Permission denied
- Timeouts
- Output capture
- Exit codes
- Command injection attempts
- Path handling

## Testing Commands

```bash
# Toast oracle - check if function exists
# WARNING: exec() can be dangerous, test carefully
./toast_oracle.exe 'exec({"echo", "hello"})'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return exec({\"echo\", \"hello\"});"

# Check conformance tests
grep -r "exec(" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-exec.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- exec() is DANGEROUS - only test safe read-only commands
- Do NOT execute arbitrary commands
- These are ToastStunt-only extensions
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
