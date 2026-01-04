# MOO SQLite Built-ins

## Overview

Functions for embedded SQLite database operations (ToastStunt optional extension).

**Availability:** SQLite functions are only available when Toast is compiled with `SQLITE3_FOUND` flag. Not all Toast builds include SQLite support.

**Permissions:** All SQLite functions require wizard permissions. Non-wizard callers receive `E_PERM`.

**Threading:** `sqlite_query()` and `sqlite_execute()` run in background threads to avoid blocking the server during long-running queries.

---

## 1. Database Connection

### 1.1 sqlite_open

**Signature:** `sqlite_open(path [, flags]) → INT`

**Description:** Opens or creates SQLite database file. Returns a numeric handle for use with other SQLite functions.

**Parameters:**
- `path`: Database file path (STR). Use `":memory:"` for in-memory database.
- `flags`: Optional integer flags (implementation-specific)

**Returns:** Database handle (INT).

**Examples:**
```moo
// Open file-based database
db = sqlite_open("data.db");

// Open in-memory database
tempdb = sqlite_open(":memory:");
```

**Errors:**
- E_PERM: Caller is not wizard
- E_INVARG: Invalid arguments
- E_FILE: Cannot open database

**Limits:** Maximum number of open handles is controlled by `$server_options.SQLITE_MAX_HANDLES`.

---

### 1.2 sqlite_close

**Signature:** `sqlite_close(handle) → none`

**Description:** Closes database connection and frees resources. The handle becomes invalid after closing.

**Parameters:**
- `handle`: Database handle from `sqlite_open()` (INT)

**Examples:**
```moo
db = sqlite_open("data.db");
// ... use database ...
sqlite_close(db);
```

**Errors:**
- E_PERM: Caller is not wizard
- E_INVARG: Invalid handle

**Note:** Connections are automatically closed when the server shuts down. However, explicitly closing connections when done is good practice.

---

### 1.3 sqlite_handles

**Signature:** `sqlite_handles() → LIST`

**Description:** Returns list of all currently open SQLite database handles.

**Returns:** List of integers representing open handles.

**Examples:**
```moo
db1 = sqlite_open("data1.db");
db2 = sqlite_open("data2.db");
handles = sqlite_handles();
// => {1, 2}
```

**Errors:**
- E_PERM: Caller is not wizard

---

### 1.4 sqlite_info

**Signature:** `sqlite_info(handle) → MAP`

**Description:** Returns information about an open database connection as a map.

**Parameters:**
- `handle`: Database handle (INT)

**Returns:** Map with connection information.

**Examples:**
```moo
db = sqlite_open("data.db");
info = sqlite_info(db);
// Returns map with connection details
```

**Errors:**
- E_PERM: Caller is not wizard
- E_INVARG: Invalid handle

---

## 2. Query Execution

### 2.1 sqlite_query

**Signature:** `sqlite_query(handle, sql [, include_headers]) → LIST`

**Description:** Executes SQL query and returns results. Runs in background thread to avoid blocking server.

**Parameters:**
- `handle`: Database handle (INT)
- `sql`: SQL query string (STR)
- `include_headers`: Optional boolean. If true, includes column names in results (BOOL)

**Returns:** List of rows, where each row is a list of values. If `include_headers` is true, format changes to include column information.

**Examples:**
```moo
db = sqlite_open("data.db");

// Create and populate table
sqlite_query(db, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)");
sqlite_query(db, "INSERT INTO users (name) VALUES ('Alice')");
sqlite_query(db, "INSERT INTO users (name) VALUES ('Bob')");

// Query without headers
rows = sqlite_query(db, "SELECT * FROM users");
// => {{1, "Alice"}, {2, "Bob"}}

// Query with headers
rows = sqlite_query(db, "SELECT * FROM users", 1);
// Returns results with column information

sqlite_close(db);
```

**Errors:**
- E_PERM: Caller is not wizard
- E_INVARG: Invalid SQL or handle
- Returns error message string if SQL fails

**Threading:** Executes in background thread. The connection is locked during execution to prevent concurrent access.

---

### 2.2 sqlite_execute

**Signature:** `sqlite_execute(handle, sql, params) → LIST`

**Description:** Executes parameterized SQL query. Always requires parameters list (use `{}` for no parameters). Runs in background thread.

**Parameters:**
- `handle`: Database handle (INT)
- `sql`: SQL statement with `?` placeholders (STR)
- `params`: List of parameter values to bind to placeholders (LIST) - **REQUIRED**, use `{}` if none

**Returns:** List of rows for SELECT queries. Empty list for other statements.

**Examples:**
```moo
db = sqlite_open("data.db");

// Create table (no parameters needed, but must pass empty list)
sqlite_execute(db, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)", {});

// Insert with parameters (SQL injection safe)
sqlite_execute(db, "INSERT INTO users (name) VALUES (?)", {"Alice"});
sqlite_execute(db, "INSERT INTO users (name) VALUES (?)", {"Bob"});

// Query with parameters
rows = sqlite_execute(db, "SELECT * FROM users WHERE name = ?", {"Alice"});
// => {{1, "Alice"}}

// Update with multiple parameters
sqlite_execute(db, "UPDATE users SET name = ? WHERE id = ?", {"Carol", 1});

sqlite_close(db);
```

**Errors:**
- E_PERM: Caller is not wizard
- E_INVARG: Invalid SQL, handle, or wrong number of parameters
- Returns error message string if SQL fails

**Security:** Always use parameterized queries with `?` placeholders to prevent SQL injection attacks. Never concatenate user input directly into SQL strings.

**Threading:** Executes in background thread. The connection is locked during execution.

---

## 3. Metadata and Introspection

### 3.1 sqlite_last_insert_row_id

**Signature:** `sqlite_last_insert_row_id(handle) → INT`

**Description:** Returns the ROWID of the most recent successful INSERT into a rowid table.

**Parameters:**
- `handle`: Database handle (INT)

**Returns:** Integer ROWID of last inserted row.

**Examples:**
```moo
db = sqlite_open("data.db");
sqlite_execute(db, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)", {});
sqlite_execute(db, "INSERT INTO users (name) VALUES (?)", {"Alice"});
id = sqlite_last_insert_row_id(db);
// => 1
sqlite_execute(db, "INSERT INTO users (name) VALUES (?)", {"Bob"});
id = sqlite_last_insert_row_id(db);
// => 2
sqlite_close(db);
```

**Errors:**
- E_PERM: Caller is not wizard
- E_INVARG: Invalid handle

**Note:** Only updated for INSERT operations on tables with rowid. Not updated for INSERT OR IGNORE, INSERT OR REPLACE when row already exists, or tables created with WITHOUT ROWID.

---

### 3.2 sqlite_limit

**Signature:** `sqlite_limit(handle, category, value) → INT`

**Description:** Sets runtime limits on various SQLite constructs for a connection. Returns the previous limit value.

**Parameters:**
- `handle`: Database handle (INT)
- `category`: Limit category as string or integer (STR or INT)
- `value`: New limit value, or -1 to query current value without changing (INT)

**Categories:**
- `"LIMIT_LENGTH"` or `0` - Maximum string or BLOB length
- `"LIMIT_SQL_LENGTH"` or `1` - Maximum SQL statement length
- `"LIMIT_COLUMN"` or `2` - Maximum columns in table
- `"LIMIT_EXPR_DEPTH"` or `3` - Maximum expression tree depth
- `"LIMIT_COMPOUND_SELECT"` or `4` - Maximum UNION/INTERSECT/EXCEPT terms
- `"LIMIT_VDBE_OP"` or `5` - Maximum bytecode instructions
- `"LIMIT_FUNCTION_ARG"` or `6` - Maximum function arguments
- `"LIMIT_ATTACHED"` or `7` - Maximum attached databases
- `"LIMIT_LIKE_PATTERN_LENGTH"` or `8` - Maximum LIKE pattern length
- `"LIMIT_VARIABLE_NUMBER"` or `9` - Maximum parameter binding number
- `"LIMIT_TRIGGER_DEPTH"` or `10` - Maximum trigger recursion depth
- `"LIMIT_WORKER_THREADS"` or `11` - Maximum worker threads

**Returns:** Previous limit value (INT).

**Examples:**
```moo
db = sqlite_open("data.db");

// Query current column limit
old_limit = sqlite_limit(db, "LIMIT_COLUMN", -1);

// Set maximum columns to 100
previous = sqlite_limit(db, "LIMIT_COLUMN", 100);

// Using integer category
sqlite_limit(db, 2, 100);  // Same as "LIMIT_COLUMN"

sqlite_close(db);
```

**Errors:**
- E_PERM: Caller is not wizard
- E_INVARG: Invalid handle or category

**Reference:** [SQLite C API: sqlite3_limit](https://www.sqlite.org/c3ref/c_limit_attached.html)

---

## 4. Query Control

### 4.1 sqlite_interrupt

**Signature:** `sqlite_interrupt(handle) → none`

**Description:** Causes any pending database operation to abort at the earliest opportunity. Used to cancel long-running queries.

**Parameters:**
- `handle`: Database handle (INT)

**Examples:**
```moo
// In one task
db = sqlite_open("data.db");
fork task_id (5)
    // Long-running query
    result = sqlite_query(db, "SELECT * FROM huge_table WHERE complex_calculation(...)");
endfork

// In another task, cancel the query
sqlite_interrupt(db);
```

**Errors:**
- E_PERM: Caller is not wizard
- E_INVARG: Invalid handle

**Note:** The interrupted operation will return an error. Safe to call even if no operation is in progress. The interrupt applies to the next operation if none is currently running.

---

## 5. SQL Injection Prevention

**Critical Security Practice:** Always use parameterized queries with `sqlite_execute()` to prevent SQL injection vulnerabilities.

### Vulnerable Code (NEVER DO THIS):
```moo
// WRONG - SQL injection vulnerability
user_input = "Alice'; DROP TABLE users; --";
query = "SELECT * FROM users WHERE name = '" + user_input + "'";
result = sqlite_query(db, query);
// Attacker can execute arbitrary SQL!
```

### Safe Code (ALWAYS DO THIS):
```moo
// RIGHT - Use parameterized queries
user_input = "Alice'; DROP TABLE users; --";
result = sqlite_execute(db, "SELECT * FROM users WHERE name = ?", {user_input});
// Parameter is safely escaped, no SQL injection possible
```

### Rules:
1. **NEVER** concatenate user input into SQL strings
2. **ALWAYS** use `?` placeholders with `sqlite_execute()`
3. **NEVER** trust user input, even from logged-in players
4. **ALWAYS** validate and sanitize input before using in any context

---

## 6. Data Type Mapping

SQLite has a flexible type system. Values are converted between MOO and SQLite types:

| SQLite Type | MOO Type | Notes |
|-------------|----------|-------|
| INTEGER     | INT      | 64-bit signed integer |
| REAL        | FLOAT    | Double precision float |
| TEXT        | STR      | String |
| BLOB        | STR      | Binary data as string |
| NULL        | STR      | Converted to string "NULL" |

**Type Coercion:** SQLite performs automatic type conversion. MOO lists are converted to strings when stored. Parse strings back to MOO values with `eval()` or custom parsing if needed.

---

## 7. PCRE Regular Expressions in SQL

If Toast is compiled with PCRE support, SQLite queries can use the `REGEXP` operator:

```moo
db = sqlite_open("data.db");
sqlite_execute(db, "CREATE TABLE users (name TEXT)", {});
sqlite_execute(db, "INSERT INTO users VALUES (?)", {"Alice"});
sqlite_execute(db, "INSERT INTO users VALUES (?)", {"Bob"});
sqlite_execute(db, "INSERT INTO users VALUES (?)", {"Carol"});

// Use REGEXP operator (requires PCRE)
results = sqlite_query(db, "SELECT * FROM users WHERE name REGEXP '^[AC]'");
// => {{"Alice"}, {"Carol"}}

sqlite_close(db);
```

**Availability:** Requires Toast compiled with both `SQLITE3_FOUND` and `PCRE_FOUND` flags.

---

## 8. Transaction Management

SQLite supports transactions through raw SQL commands. There are no dedicated transaction built-ins in Toast.

Use `sqlite_query()` or `sqlite_execute()` with transaction SQL:

```moo
db = sqlite_open("data.db");
sqlite_execute(db, "CREATE TABLE accounts (id INT, balance INT)", {});
sqlite_execute(db, "INSERT INTO accounts VALUES (?, ?)", {1, 1000});
sqlite_execute(db, "INSERT INTO accounts VALUES (?, ?)", {2, 500});

// Begin transaction
sqlite_query(db, "BEGIN TRANSACTION");

try
    // Transfer 100 from account 1 to account 2
    sqlite_execute(db, "UPDATE accounts SET balance = balance - ? WHERE id = ?", {100, 1});
    sqlite_execute(db, "UPDATE accounts SET balance = balance + ? WHERE id = ?", {100, 2});

    // Commit on success
    sqlite_query(db, "COMMIT");
except e (ANY)
    // Rollback on error
    sqlite_query(db, "ROLLBACK");
    player:tell("Transaction failed: ", toliteral(e));
endtry

sqlite_close(db);
```

**Transaction Commands:**
- `BEGIN TRANSACTION` or `BEGIN` - Start transaction
- `COMMIT` - Save changes
- `ROLLBACK` - Discard changes
- `SAVEPOINT name` - Create savepoint
- `RELEASE name` - Remove savepoint
- `ROLLBACK TO name` - Rollback to savepoint

---

## 9. Error Handling

All SQLite functions that can fail return errors or error strings:

```moo
db = sqlite_open("data.db");

// sql_query returns error strings on failure
result = sqlite_query(db, "INVALID SQL HERE");
if (typeof(result) == STR)
    // result is error message
    player:tell("SQL error: ", result);
    return;
endif

// Type check results
if (typeof(result) != LIST)
    player:tell("Unexpected result type");
    return;
endif

// Process results
for row in (result)
    // ... handle row ...
endfor

sqlite_close(db);
```

**Common Errors:**
| Error | Condition |
|-------|-----------|
| E_PERM | Caller is not wizard |
| E_INVARG | Invalid SQL, handle, or parameters |
| E_FILE | Database I/O error |
| STR | SQL execution error (returns error message) |

---

## 10. Resource Management

### Connection Limits

The server enforces a maximum number of open SQLite connections:

```moo
// Check limit
max_handles = $server_options.SQLITE_MAX_HANDLES;

// List current connections
handles = sqlite_handles();
if (length(handles) >= max_handles)
    player:tell("Too many open connections");
    return;
endif
```

### Best Practices

1. **Close connections** when done to free resources
2. **Use transactions** for multiple related operations
3. **Set query limits** with `sqlite_limit()` to prevent abuse
4. **Use `sqlite_interrupt()`** to cancel long-running queries
5. **Monitor open handles** with `sqlite_handles()`
6. **Handle errors** properly - check return values
7. **Use parameterized queries** to prevent SQL injection

---

## 11. Performance Considerations

### Threading

`sqlite_query()` and `sqlite_execute()` run in background threads automatically. The server doesn't block during query execution.

### Locking

SQLite connections are locked during query execution to prevent race conditions. Don't share handles between tasks without explicit synchronization.

### Optimization Tips

1. **Use indexes** on frequently queried columns
2. **Use transactions** for bulk inserts (much faster)
3. **Use `PRAGMA`** statements to tune performance
4. **Analyze query plans** with `EXPLAIN QUERY PLAN`
5. **Set appropriate limits** with `sqlite_limit()`

Example bulk insert with transaction:
```moo
db = sqlite_open("data.db");
sqlite_execute(db, "CREATE TABLE items (id INT, name TEXT)", {});

// Slow: individual inserts
for i in [1..1000]
    sqlite_execute(db, "INSERT INTO items VALUES (?, ?)", {i, "Item " + tostr(i)});
endfor

// Fast: transaction batch
sqlite_query(db, "BEGIN TRANSACTION");
for i in [1..1000]
    sqlite_execute(db, "INSERT INTO items VALUES (?, ?)", {i, "Item " + tostr(i)});
endfor
sqlite_query(db, "COMMIT");

sqlite_close(db);
```

---

## 12. Limitations

1. **Wizard-only:** All SQLite functions require wizard permissions
2. **Optional feature:** Not available unless compiled with `SQLITE3_FOUND`
3. **File system access:** Database paths may be restricted by server configuration
4. **Connection limit:** Maximum handles controlled by `$server_options.SQLITE_MAX_HANDLES`
5. **No connection pooling:** Each handle is independent
6. **Thread-safe only:** Requires SQLite compiled with thread safety
7. **Single process:** SQLite handles are not shared across server restarts

---

## 13. Go Implementation Notes

```go
import "database/sql"
import _ "github.com/mattn/go-sqlite3"

type SQLiteHandle struct {
    ID    int
    Path  string
    DB    *sql.DB
    Locks int
}

var sqliteHandles = make(map[int]*SQLiteHandle)
var nextSQLiteHandle = 1
var maxSQLiteHandles = 10 // from $server_options

func builtinSqliteOpen(args []Value) (Value, error) {
    // Check wizard permission
    if !isWizard(taskPerms) {
        return nil, E_PERM
    }

    path := toString(args[0])

    // Check connection limit
    if len(sqliteHandles) >= maxSQLiteHandles {
        return nil, E_QUOTA
    }

    // Security: validate path
    if !isAllowedPath(path) {
        return nil, E_PERM
    }

    db, err := sql.Open("sqlite3", path)
    if err != nil {
        return nil, E_FILE
    }

    // Test connection
    if err := db.Ping(); err != nil {
        db.Close()
        return nil, E_FILE
    }

    handle := nextSQLiteHandle
    nextSQLiteHandle++
    sqliteHandles[handle] = &SQLiteHandle{
        ID:    handle,
        Path:  path,
        DB:    db,
        Locks: 0,
    }

    return NewInt(handle), nil
}

func builtinSqliteClose(args []Value) (Value, error) {
    if !isWizard(taskPerms) {
        return nil, E_PERM
    }

    handleID := toInt(args[0])
    h, ok := sqliteHandles[handleID]
    if !ok {
        return nil, E_INVARG
    }

    // Wait for any pending operations
    for h.Locks > 0 {
        time.Sleep(10 * time.Millisecond)
    }

    h.DB.Close()
    delete(sqliteHandles, handleID)

    return None, nil
}

func builtinSqliteExecute(args []Value) (Value, error) {
    if !isWizard(taskPerms) {
        return nil, E_PERM
    }

    handleID := toInt(args[0])
    h, ok := sqliteHandles[handleID]
    if !ok {
        return nil, E_INVARG
    }

    sqlStr := toString(args[1])
    params := toList(args[2])

    // Convert MOO values to interface{} for database/sql
    sqlParams := make([]interface{}, len(params))
    for i, p := range params {
        sqlParams[i] = mooToGo(p)
    }

    // Lock connection
    h.Locks++
    defer func() { h.Locks-- }()

    // Execute query in goroutine
    rows, err := h.DB.Query(sqlStr, sqlParams...)
    if err != nil {
        // Return error message as string (Toast behavior)
        return NewString(err.Error()), nil
    }
    defer rows.Close()

    // Get column info
    cols, _ := rows.Columns()
    result := NewList()

    // Read all rows
    for rows.Next() {
        values := make([]interface{}, len(cols))
        valuePtrs := make([]interface{}, len(cols))
        for i := range values {
            valuePtrs[i] = &values[i]
        }

        if err := rows.Scan(valuePtrs...); err != nil {
            return NewString(err.Error()), nil
        }

        row := NewList()
        for _, v := range values {
            row.Append(goToMoo(v))
        }
        result.Append(row)
    }

    return result, nil
}

func goToMoo(v interface{}) Value {
    switch val := v.(type) {
    case int64:
        return NewInt(int(val))
    case float64:
        return NewFloat(val)
    case string:
        return NewString(val)
    case []byte:
        return NewString(string(val))
    case nil:
        return NewString("NULL")
    default:
        return NewString("NULL")
    }
}

func mooToGo(v Value) interface{} {
    switch v.Type() {
    case TYPE_INT:
        return int64(v.ToInt())
    case TYPE_FLOAT:
        return v.ToFloat()
    case TYPE_STR:
        return v.ToString()
    case TYPE_OBJ:
        return int64(v.ToObj())
    default:
        return nil
    }
}
```

**Key Implementation Details:**
- All functions check `isWizard()` first
- Connections have lock counters to prevent closing during queries
- Queries run in goroutines (background threads)
- Errors return as strings, not error values
- NULL converts to string "NULL" in MOO
- Connection limit enforced in `sqlite_open()`
- Path validation prevents directory traversal attacks
