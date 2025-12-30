# Task: Fix Traceback to Show Correct Verb Location

## Problem

When an error occurs in an inherited verb, Barn's traceback shows the wrong object.

**Current behavior:**
```
#2 <- #2:news (this == #2), line 1:  Range error
```

**Expected behavior:**
```
#6:news (this == #2), line 1:  Range error
```

The verb `news` is defined on #6 ("generic player"), but #2 inherits it. The traceback should show where the verb is DEFINED (#6), not the `this` object (#2).

## Context

- #2 is "Wizard" with parent chain: #2 → #57 → #58 → #4 → #88 → #40 → #100 → #6
- The `news` verb is on #6
- When #2 calls `news`, `this` is #2 but the verb code lives on #6
- Toast (reference) shows: `#6:news, line 45` - correct verb location

## Where to Look

- Error handling / traceback generation code in Barn
- Likely in `vm/` directory
- Look for where error messages are formatted with verb location

## Test

```bash
cd ~/code/barn

# Restart Barn with toastcore
taskkill //F //IM barn_test.exe 2>/dev/null
./barn_test.exe -db toastcore_barn.db -port 9300 > server_barn_9300.log 2>&1 &
sleep 3

# Trigger the error
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard" -cmd "news"

# Should show #6:news in traceback, not #2:news
```

## Deliverable

1. Fix the traceback to show the object where verb is DEFINED
2. Test that traceback now shows correct location
3. Write report to `./reports/fix-traceback-verb-location.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
