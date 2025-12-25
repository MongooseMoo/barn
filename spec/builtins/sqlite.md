# MOO SQLite Built-ins

## Overview

Functions for embedded SQLite database operations (ToastStunt extension).

---

## 1. Database Connection

### 1.1 sqlite_open

**Signature:** `sqlite_open(path) → INT`

**Description:** Opens or creates SQLite database.

**Parameters:**
- `path`: Database file path

**Returns:** Database handle (integer).

**Examples:**
```moo
db = sqlite_open("data.db");
```

**Errors:**
- E_PERM: File access denied
- E_FILE: Cannot open database

---

### 1.2 sqlite_close

**Signature:** `sqlite_close(handle) → none`

**Description:** Closes database connection.

**Examples:**
```moo
sqlite_close(db);
```

---

## 2. Query Execution

### 2.1 sqlite_execute

**Signature:** `sqlite_execute(handle, sql [, params]) → LIST`

**Description:** Executes SQL query with optional parameters.

**Parameters:**
- `handle`: Database handle
- `sql`: SQL statement
- `params`: List of parameter values (for ? placeholders)

**Returns:** List of rows, each row is a list of values.

**Examples:**
```moo
// Create table
sqlite_execute(db, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)");

// Insert with parameters
sqlite_execute(db, "INSERT INTO users (name) VALUES (?)", {"Alice"});
sqlite_execute(db, "INSERT INTO users (name) VALUES (?)", {"Bob"});

// Query
rows = sqlite_execute(db, "SELECT * FROM users");
// => {{1, "Alice"}, {2, "Bob"}}

// Query with parameters
rows = sqlite_execute(db, "SELECT * FROM users WHERE name = ?", {"Alice"});
// => {{1, "Alice"}}
```

**Errors:**
- E_INVARG: Invalid SQL or handle
- E_FILE: Database error

---

### 2.2 sqlite_query

**Signature:** `sqlite_query(handle, sql [, params]) → LIST`

**Description:** Alias for sqlite_execute (query-focused).

---

## 3. Results as Maps

### 3.1 sqlite_query_maps (ToastStunt)

**Signature:** `sqlite_query_maps(handle, sql [, params]) → LIST`

**Description:** Returns rows as maps with column names as keys.

**Examples:**
```moo
rows = sqlite_query_maps(db, "SELECT * FROM users");
// => {["id" -> 1, "name" -> "Alice"], ["id" -> 2, "name" -> "Bob"]}

for row in (rows)
    notify(player, "User: " + row["name"]);
endfor
```

---

## 4. Last Insert ID

### 4.1 sqlite_last_insert_id

**Signature:** `sqlite_last_insert_id(handle) → INT`

**Description:** Returns last auto-generated ID.

**Examples:**
```moo
sqlite_execute(db, "INSERT INTO users (name) VALUES (?)", {"Carol"});
id = sqlite_last_insert_id(db);
// => 3
```

---

## 5. Affected Rows

### 5.1 sqlite_changes

**Signature:** `sqlite_changes(handle) → INT`

**Description:** Returns number of rows affected by last statement.

**Examples:**
```moo
sqlite_execute(db, "UPDATE users SET name = ? WHERE id = ?", {"Charlie", 1});
changed = sqlite_changes(db);
// => 1
```

---

## 6. Transactions

### 6.1 sqlite_begin

**Signature:** `sqlite_begin(handle) → none`

**Description:** Starts a transaction.

---

### 6.2 sqlite_commit

**Signature:** `sqlite_commit(handle) → none`

**Description:** Commits transaction.

---

### 6.3 sqlite_rollback

**Signature:** `sqlite_rollback(handle) → none`

**Description:** Rolls back transaction.

**Examples:**
```moo
sqlite_begin(db);
try
    sqlite_execute(db, "INSERT INTO users (name) VALUES (?)", {"Test1"});
    sqlite_execute(db, "INSERT INTO users (name) VALUES (?)", {"Test2"});
    sqlite_commit(db);
except (ANY)
    sqlite_rollback(db);
endtry
```

---

## 7. Table Information

### 7.1 sqlite_tables

**Signature:** `sqlite_tables(handle) → LIST`

**Description:** Returns list of table names.

**Examples:**
```moo
tables = sqlite_tables(db);
// => {"users", "posts", "comments"}
```

---

### 7.2 sqlite_columns

**Signature:** `sqlite_columns(handle, table) → LIST`

**Description:** Returns column information for table.

**Returns:** List of column info: `{name, type, nullable, default, pk}`

---

## 8. SQL Injection Prevention

**Always use parameterized queries:**

```moo
// WRONG - SQL injection vulnerability
sql = "SELECT * FROM users WHERE name = '" + user_input + "'";

// RIGHT - Use parameters
sql = "SELECT * FROM users WHERE name = ?";
sqlite_execute(db, sql, {user_input});
```

---

## 9. Data Type Mapping

| SQLite Type | MOO Type |
|-------------|----------|
| INTEGER | INT |
| REAL | FLOAT |
| TEXT | STR |
| BLOB | STR (binary) |
| NULL | INT (0) |

---

## 10. Error Handling

| Error | Condition |
|-------|-----------|
| E_INVARG | Invalid SQL or handle |
| E_FILE | Database I/O error |
| E_PERM | Permission denied |
| E_TYPE | Wrong parameter type |

---

## 11. Security Considerations

1. **Parameterized queries** - Always use ? placeholders
2. **Path restrictions** - Database files may be sandboxed
3. **Permission checks** - May require wizard
4. **Resource limits** - Query timeout, result size

---

## 12. Go Implementation Notes

```go
import "database/sql"
import _ "github.com/mattn/go-sqlite3"

type SQLiteHandle struct {
    ID   int
    Path string
    DB   *sql.DB
}

var sqliteHandles = make(map[int]*SQLiteHandle)
var nextSQLiteHandle = 1

func builtinSqliteOpen(args []Value) (Value, error) {
    path := string(args[0].(StringValue))

    // Security: validate path
    if !isAllowedPath(path) {
        return nil, E_PERM
    }

    db, err := sql.Open("sqlite3", path)
    if err != nil {
        return nil, E_FILE
    }

    handle := nextSQLiteHandle
    nextSQLiteHandle++
    sqliteHandles[handle] = &SQLiteHandle{
        ID:   handle,
        Path: path,
        DB:   db,
    }

    return IntValue(handle), nil
}

func builtinSqliteExecute(args []Value) (Value, error) {
    handleID := int(args[0].(IntValue))
    h, ok := sqliteHandles[handleID]
    if !ok {
        return nil, E_INVARG
    }

    sqlStr := string(args[1].(StringValue))

    // Convert MOO params to interface{}
    var params []any
    if len(args) > 2 {
        paramList := args[2].(*MOOList)
        params = make([]any, len(paramList.data))
        for i, v := range paramList.data {
            params[i] = mooToGo(v)
        }
    }

    rows, err := h.DB.Query(sqlStr, params...)
    if err != nil {
        return nil, E_INVARG
    }
    defer rows.Close()

    cols, _ := rows.Columns()
    var results []Value

    for rows.Next() {
        values := make([]any, len(cols))
        ptrs := make([]any, len(cols))
        for i := range values {
            ptrs[i] = &values[i]
        }

        rows.Scan(ptrs...)

        row := make([]Value, len(cols))
        for i, v := range values {
            row[i] = goToMoo(v)
        }
        results = append(results, &MOOList{data: row})
    }

    return &MOOList{data: results}, nil
}

func goToMoo(v any) Value {
    switch val := v.(type) {
    case int64:
        return IntValue(val)
    case float64:
        return FloatValue(val)
    case string:
        return StringValue(val)
    case []byte:
        return StringValue(string(val))
    case nil:
        return IntValue(0)
    default:
        return IntValue(0)
    }
}
```
