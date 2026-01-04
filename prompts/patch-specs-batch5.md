# Task: Patch Specs for Batch 5 Divergences

## Context

Divergence detection completed for batch 5 (fileio, network, crypto, regex, sqlite, exec). Multiple spec corrections needed.

## Reports to Read

- `reports/divergence-fileio.md`
- `reports/divergence-network.md`
- `reports/divergence-crypto.md`
- `reports/divergence-regex.md`
- `reports/divergence-sqlite.md`
- `reports/divergence-exec.md`

## Patches Required

### 1. spec/builtins/fileio.md

**Add availability note at top of spec:**
> **Note:** File I/O functions require server configuration to enable. They are disabled by default in Test.db and cannot be tested without enabling `file_io` in server options.

**Remove 4 functions that don't exist:**
- `file_exists(path)` - documented but NOT in Toast source
- `file_copy(source, dest)` - documented but NOT in Toast source
- `file_path_info(path)` - documented but NOT in Toast source
- `file_resolve(path)` - documented but NOT in Toast source

**Add 6 functions that exist but aren't documented:**
- `file_handles()` - list open file handles
- `file_name(int)` - get file path from handle
- `file_openmode(int)` - get mode from handle
- `file_grep(int, str, int)` - grep in file
- `file_count_lines(int)` - count lines
- `file_flush(int)` - needs complete documentation

### 2. spec/builtins/network.md

**Mark as [Not Implemented]:**
- `connection_info()` - Returns E_VERBNF in both servers
- `curl()` - Returns E_VARNF in both servers

### 3. spec/builtins/crypto.md

**Fix default algorithm:**
- CURRENT: Spec says MD5 is default for `string_hash()`
- CORRECT: SHA256 is the actual default (confirmed in both servers)
- Evidence: `string_hash("hello")` returns 64-char hex (SHA256, not 32-char MD5)

**Mark as [Not Implemented]:**
- `encode_hex()` - documented but doesn't exist
- `decode_hex()` - documented but doesn't exist

### 4. spec/builtins/regex.md

**NO SPEC CHANGES** - The regex report found a **Barn bug**, not a spec error.

Barn's `%` escape handling is backwards from Toast. The spec correctly documents Toast's behavior. This needs a Barn code fix, not a spec change.

### 5. spec/builtins/sqlite.md

**Add prominent notice at top:**
> **Status: Draft/Planned Feature - Not Implemented**
>
> This specification describes a planned SQLite integration that has not been implemented in ToastStunt or Barn. The API described below is aspirational and no reference implementation exists.

All functions should be marked with `[Not Implemented]` or the entire spec marked as draft.

### 6. spec/builtins/exec.md

**Mark as [Not Implemented]:**
- `exec_async()` - documented but returns E_VERBNF
- `exec_timeout()` - documented but returns E_VERBNF
- `kill_process()` - documented but returns E_VERBNF
- `wait_process()` - documented but returns E_VERBNF
- `exec_env()` - documented but returns E_VERBNF
- String form `exec("command")` - documented but returns E_INVARG

## Style Guide

For [Not Implemented] markers, follow the pattern:

```markdown
### function_name() [Not Implemented]

**Signature:** `function_name(args)`

> **Note:** This function is documented but not implemented in ToastStunt or Barn.

Description of what it would do if implemented.
```

## Output

Apply patches directly to the spec files. Be surgical - only change what's documented above.

## CRITICAL

- Do NOT change regex.md - the issue is a Barn bug, not spec error
- Do NOT invent behaviors - only document verified findings
- For SQLite, mark the ENTIRE spec as draft/planned
- Preserve existing spec structure and formatting
