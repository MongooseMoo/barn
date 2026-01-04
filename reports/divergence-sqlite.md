# Divergence Report: SQLite Builtins

**Spec File**: `spec/builtins/sqlite.md`
**Barn Files**: None (not implemented)
**Toast Implementation**: None (not implemented)
**Status**: not_implemented
**Date**: 2026-01-03

## Summary

SQLite builtins are documented in the spec but **NOT implemented in either Toast or Barn**. This represents a spec-only feature with no reference implementation available.

- **Functions documented**: 13 (sqlite_open, sqlite_close, sqlite_execute, sqlite_query, sqlite_query_maps, sqlite_last_insert_id, sqlite_changes, sqlite_begin, sqlite_commit, sqlite_rollback, sqlite_tables, sqlite_columns, plus parameterized query support)
- **Barn implementation**: None found
- **Toast implementation**: None found
- **Conformance tests**: None found

## Key Findings

### 1. Toast Does Not Support SQLite

Testing against Toast oracle confirms no SQLite support:

```bash
$ ./toast_oracle.exe 'sqlite_open(":memory:")'
(#2): ** 1 errors during parsing:
  Line 2:  Unknown built-in function: sqlite_open
```

```bash
$ ./toast_oracle.exe 'sqlite_execute(1, "SELECT 1")'
(#2): ** 1 errors during parsing:
  Line 2:  Unknown built-in function: sqlite_execute
```

### 2. Barn Does Not Support SQLite

Testing against Barn (port 9500):

```bash
$ ./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return sqlite_open(\":memory:\");"
Sending command 2: ; return sqlite_open(":memory:");
{2, {E_VERBNF, "", 0}}
```

The `E_VERBNF` error indicates the function is not found in the builtin registry.

Verification of Barn's builtin registry (`builtins/registry.go`):
- No `sqlite_*` functions registered
- Total of ~111 registered builtins
- No `builtins/sqlite.go` file exists

### 3. No Conformance Tests

Search of conformance test suite:

```bash
$ grep -r "sqlite" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
# No results
```

## Divergences

**None** - Both servers have identical behavior (neither implements SQLite).

## Classification

**spec_only_feature**: This is a documented feature with no implementation. The spec describes it as a "ToastStunt extension" but Toast 2.7.3_2 does not include it.

## Analysis

The spec describes SQLite as a "ToastStunt extension" but testing confirms:
1. ToastStunt 2.7.3_2 does **not** include these functions
2. The spec may be aspirational or based on a fork/branch
3. The detailed Go implementation code in the spec suggests someone drafted implementation plans
4. No MOO server (Toast or Barn) currently supports this feature

## Test Coverage Gaps

**All behaviors** documented in spec have zero test coverage:

### Database Connection
- `sqlite_open(path)` - open/create database, return handle
- `sqlite_close(handle)` - close connection
- Edge cases: invalid paths, permission denied, in-memory databases (`:memory:`)

### Query Execution
- `sqlite_execute(handle, sql [, params])` - execute with optional parameters
- `sqlite_query(handle, sql [, params])` - alias for sqlite_execute
- `sqlite_query_maps(handle, sql [, params])` - return rows as maps
- Edge cases: invalid SQL, SQL injection attempts, NULL values, BLOBs, large result sets

### Metadata
- `sqlite_last_insert_id(handle)` - get last auto-generated ID
- `sqlite_changes(handle)` - get rows affected
- Edge cases: no insert performed, multiple inserts

### Transactions
- `sqlite_begin(handle)` - start transaction
- `sqlite_commit(handle)` - commit transaction
- `sqlite_rollback(handle)` - rollback transaction
- Edge cases: nested transactions, commit without begin, rollback without begin

### Schema Information
- `sqlite_tables(handle)` - list table names
- `sqlite_columns(handle, table)` - get column info
- Edge cases: empty database, non-existent table

### Error Handling
- E_INVARG for invalid SQL or handle
- E_FILE for database I/O errors
- E_PERM for permission denied
- E_TYPE for wrong parameter types

### Data Type Mapping
- INTEGER → INT
- REAL → FLOAT
- TEXT → STR
- BLOB → STR (binary)
- NULL → INT (0)

## Behaviors Verified Correct

**None** - Feature is not implemented in either server.

## Recommendations

1. **Clarify Spec Status**: Mark spec as "planned" or "draft" rather than "ToastStunt extension"
2. **Remove or Annotate**: Either remove this spec (no implementation exists) or add prominent disclaimer
3. **Implementation Decision**: If implementing in Barn, this would be a **new feature** not a conformance effort
4. **Security Review**: If implementing, carefully review:
   - SQL injection prevention (parameterized queries)
   - File system sandboxing
   - Permission checks
   - Resource limits (query timeout, result size, disk space)
   - BLOB handling

## Notes

The spec includes detailed Go implementation code which suggests this was planned for implementation. However:
- No working implementation exists in any known MOO server
- No tests exist to verify behavior
- Security implications need careful consideration
- This would be a significant new feature, not a conformance item

**This should NOT be treated as a divergence to fix** - it's a spec for an unimplemented feature.
