# Task: Fix Verb Call Wildcard Matching

## The Problem

MOO verb names use `*` wildcards for prefix matching:
- `get_conj*ugation` should match calls to `get_conj`, `get_conju`, etc.
- `l*ook` should match calls to `l`, `lo`, `loo`, `look`

**Command parsing works** - player typing "l" matches `l*ook`.

**Verb calls from code are broken** - `this:get_conj()` in MOO code fails with "Verb not found" because it does exact matching instead of wildcard matching.

## Evidence

```
./dump_verb.exe 41 get_conj
Verb 'get_conj' not found on #41
Available verbs:
  get_conj*ugation   <-- THIS should match get_conj
```

The call `this:get_conj(...)` at line 33 of `#41:_do` fails.

## What To Fix

Find where verb calls from MOO code (`obj:verb()` syntax) do their lookup. It's NOT using the same wildcard matching as command parsing.

Likely locations:
- VM opcode handler for CALLVERB or similar
- `builtins/` if there's a call_verb implementation
- `server/scheduler.go` CallVerb function
- `db/` verb lookup functions

The command parser path works - find what IT uses for matching and make the verb-call path use the same logic.

## Test

After fix:
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db toastcore_barn.db -port 9300 &
sleep 3
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard" -cmd "look me"
```

Should show full description without "Verb not found" error.

## Output

Write to `./reports/fix-verb-call-wildcard.md`:
1. What you found (which file/function was wrong)
2. What you changed
3. Test results

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
