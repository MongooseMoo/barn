# Task: Scout ctime() Builtin Behavior

## Mission

Quick investigation: Is `ctime()` broken in Barn?

## Evidence

When running `news` in Barn, we see malformed date output:
```
It's the current issue of the News, dated Mon May  1,2 200.
```

That "Mon May  1,2 200" looks wrong - extra spaces, truncated year.

## Quick Tests

```bash
cd ~/code/barn

# Test ctime on Barn (port 9300)
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard" -cmd "; return ctime();"

# Test ctime on Toast (port 9400) for comparison
./moo_client.exe -port 9400 -timeout 5 -cmd "connect wizard" -cmd "; return ctime();"

# Test with specific timestamp
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard" -cmd "; return ctime(0);"
./moo_client.exe -port 9400 -timeout 5 -cmd "connect wizard" -cmd "; return ctime(0);"
```

## Questions to Answer

1. Does `ctime()` return different results on Barn vs Toast?
2. Is the output format wrong?
3. Is this the likely cause of the Range error? (code expecting certain format/length)

## Output

Write brief findings to `./reports/scout-ctime-builtin.md`:
- ctime() Barn output: [X]
- ctime() Toast output: [X]
- Are they different? [yes/no]
- Likely cause of Range error? [yes/no/maybe]

Keep it short - this is just a scout mission.

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
