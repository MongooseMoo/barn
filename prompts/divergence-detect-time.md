# Task: Detect Divergences in Time Builtins

## Context

We need to verify Barn's time builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all time builtins.

## Files to Read

- `spec/builtins/time.md` - expected behavior specification
- `builtins/time.go` - Barn implementation

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Current Time
- `time()` - Unix timestamp
- `ftime()` - floating-point timestamp (ToastStunt)
- `ctime()` - human-readable time string

### Time Formatting
- `strftime()` - format time string
- `strptime()` (if exists) - parse time string

### Server Timing
- `server_log()` - write to server log
- `idle_seconds()` - player idle time
- `connected_seconds()` - player connection time

### Sleeping
- `sleep()` - pause execution (if builtin)

## Edge Cases to Test

- Negative timestamps (pre-1970)
- Very large timestamps
- Format string edge cases
- Timezone handling
- Locale handling

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'time()'
./toast_oracle.exe 'ctime(time())'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return time();"
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return ctime(time());"

# Check conformance tests
grep -r "time\|ctime\|strftime" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-time.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY major time builtin
- Time values will differ between runs - compare format/structure not values
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
