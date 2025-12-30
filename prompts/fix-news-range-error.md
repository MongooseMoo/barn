# Task: Investigate and Fix News Command Range Error

## Context

When running `news` command in Barn with toastcore.db, we get:
```
It's the current issue of the News, dated Mon May  1,2 200.
#2 <- #2:news (this == #2), line 1:  Range error
#2 <- (End of traceback)
```

Toast (reference implementation) also has issues but different ones:
```
#61:description (this == #61), line 1:  Invalid argument
... called from #61:news_display_seq_full (this == #61), line 4
... called from #6:news, line 45
```

## Known Facts

1. **Verb `news` is defined on #6 ("generic player")**, not #2
2. **#2's ancestry**: #2 → #57 → #58 → #4 → #88 → #40 → #100 → #6
3. **Barn traceback shows wrong object**: `#2:news` instead of `#6:news`
4. **Date formatting is malformed**: "Mon May  1,2 200" (extra spaces, truncated year)
5. **Line 1 of #6:news is just a doc string**: `"Usage: news [contents] [articles]";`

## Issues to Investigate

### Issue 1: Traceback showing wrong verb location
Barn's traceback format `#2 <- #2:news (this == #2)` should show `#6:news` since that's where the verb is defined. This is a traceback formatting bug.

### Issue 2: "Range error" on line 1
Line 1 is just a string literal (doc comment). It shouldn't cause a Range error. Either:
- Line numbers are wrong
- The error is from a different verb
- Something in MOO expression evaluation is wrong

### Issue 3: Malformed date output
"Mon May  1,2 200" suggests issues with date formatting. This might be related to `ctime()` builtin.

## Servers for Testing

- **Toast (oracle)**: port 9400 (cd ~/src/toaststunt && ./build-win/Release/moo.exe toastcore.db toastcore.db.new 9400)
- **Barn (test)**: port 9300 with toastcore_barn.db

## Test Commands

```bash
cd ~/code/barn

# Test news command
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard" -cmd "news"

# Test specific expressions
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard" -cmd '; return ctime();'

# Compare with Toast
./moo_client.exe -port 9400 -timeout 5 -cmd "connect wizard" -cmd '; return ctime();'
```

## Relevant Barn Code

- `vm/` - MOO VM execution
- `builtins/` - Builtin functions (including ctime)
- Traceback formatting - wherever errors are reported

## Objective

1. **Diagnose** the root cause(s) of the Range error
2. **Fix** the traceback to show correct verb location (#6 not #2)
3. **Fix** the Range error (or identify if it's expected MOO behavior)
4. **Verify** by running `news` command successfully (or at least matching Toast's behavior)

## Output

Write findings and fixes to `./reports/fix-news-range-error.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
