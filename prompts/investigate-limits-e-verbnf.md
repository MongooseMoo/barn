# Task: Investigate Limits E_VERBNF Root Cause

## Context
39 conformance tests for limits are failing with E_VERBNF instead of E_QUOTA or success. This suggests the limits-checking code is trying to call a function/verb that doesn't exist.

Examples of failures:
- `limits::setadd_checks_list_max_value_bytes_small` - E_VERBNF instead of E_QUOTA
- `limits::string_concat_limit` - E_VERBNF instead of success
- `limits::tostr_exceeds_limit` - E_VERBNF instead of E_QUOTA

## Objective
Find the root cause of why limits tests return E_VERBNF.

## Investigation Steps

1. Run a specific failing limit test to get the exact error:
```bash
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/test_limits.py::test_string_concat_limit --transport socket --moo-port 9300 -v -s 2>&1 | head -50
```

2. Look at the test to understand what it's testing:
```bash
# Find the test file
grep -n "string_concat_limit" /c/Users/Q/code/cow_py/tests/conformance/test_limits.py
```

3. Try running the MOO code manually:
```bash
cd /c/Users/Q/code/barn
./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "; return \"a\" + \"b\";"
```

4. Search the barn codebase for how limits are enforced:
```bash
grep -r "E_QUOTA" /c/Users/Q/code/barn/
grep -r "max_string" /c/Users/Q/code/barn/
grep -r "limit" /c/Users/Q/code/barn/builtins/
```

5. Check if there's a limits configuration or builtin:
```bash
grep -r "server_options\|options_cache\|limit" /c/Users/Q/code/barn/vm/
```

6. Look at how ToastStunt handles limits for reference:
```bash
grep -n "E_QUOTA" /c/Users/Q/src/toaststunt/src/*.cc | head -20
```

## Key Questions to Answer

1. Is there a limits system in Barn at all?
2. If yes, what function/builtin does it call that returns E_VERBNF?
3. If no, how should limits be implemented?
4. What triggers the E_VERBNF - is it trying to read server options?

## Output
Write findings to `./reports/investigate-limits-e-verbnf.md` with:
1. The exact MOO code that triggers E_VERBNF
2. The stack trace or error context if available
3. The root cause identified
4. Recommended fix approach

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
5. If all formats fail, STOP and report - do not use bash workarounds
