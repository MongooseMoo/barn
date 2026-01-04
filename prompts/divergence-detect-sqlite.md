# Task: Detect Divergences in SQLite Builtins

## Context

We need to verify Barn's SQLite builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all SQLite builtins.

## Files to Read

- `spec/builtins/sqlite.md` - expected behavior specification
- `builtins/sqlite.go` - Barn implementation (if exists)

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Database Operations
- `sqlite_open()` - open database
- `sqlite_close()` - close database
- `sqlite_execute()` - execute SQL statement
- `sqlite_query()` - execute query and return results
- `sqlite_last_insert_rowid()` - get last insert ID
- `sqlite_changes()` - get rows affected

### Prepared Statements (if exists)
- `sqlite_prepare()` - prepare statement
- `sqlite_bind()` - bind parameters
- `sqlite_step()` - execute prepared statement
- `sqlite_finalize()` - clean up statement

## Edge Cases to Test

- Invalid SQL syntax
- SQL injection attempts
- NULL values
- Binary data (BLOBs)
- Large result sets
- Transaction handling
- Database not found

## Testing Commands

```bash
# Toast oracle - check if function exists
./toast_oracle.exe 'sqlite_open(":memory:")'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return sqlite_open(\":memory:\");"

# Check conformance tests
grep -r "sqlite_" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-sqlite.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- These are ToastStunt-only extensions
- Do NOT create persistent database files
- Use :memory: for in-memory databases if testing
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
