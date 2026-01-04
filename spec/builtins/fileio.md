# MOO File I/O Built-ins

## Overview

Functions for file system operations. Requires file I/O to be enabled in server configuration.

> **Note:** File I/O functions require server configuration to enable. They are disabled by default in Test.db and cannot be tested without enabling `file_io` in server options.

---

## 1. File Reading

### 1.1 file_open

**Signature:** `file_open(path, mode) → INT`

**Description:** Opens a file and returns handle.

**Modes:**
| Mode | Description |
|------|-------------|
| "r" | Read (text) |
| "rb" | Read (binary) |
| "w" | Write (text, truncate) |
| "wb" | Write (binary, truncate) |
| "a" | Append (text) |
| "ab" | Append (binary) |

**Examples:**
```moo
handle = file_open("data.txt", "r");
```

**Errors:**
- E_FILE: File not found or permission denied
- E_INVARG: Invalid mode

---

### 1.2 file_close

**Signature:** `file_close(handle) → none`

**Description:** Closes a file handle.

**Examples:**
```moo
file_close(handle);
```

---

### 1.3 file_readline

**Signature:** `file_readline(handle) → STR`

**Description:** Reads one line from file.

**Returns:** Line without newline, or empty string at EOF.

**Examples:**
```moo
line = file_readline(handle);
while (line != "")
    process(line);
    line = file_readline(handle);
endwhile
```

---

### 1.4 file_readlines

**Signature:** `file_readlines(handle [, start [, end]]) → LIST`

**Description:** Reads multiple lines.

**Parameters:**
- `start`: First line (1-based, default: 1)
- `end`: Last line (default: EOF)

**Examples:**
```moo
lines = file_readlines(handle);           // All lines
lines = file_readlines(handle, 1, 10);    // First 10
```

---

### 1.5 file_read

**Signature:** `file_read(handle [, bytes]) → STR`

**Description:** Reads bytes from file.

**Examples:**
```moo
data = file_read(handle, 1024);   // Read up to 1024 bytes
data = file_read(handle);         // Read all
```

---

## 2. File Writing

### 2.1 file_writeline

**Signature:** `file_writeline(handle, line) → none`

**Description:** Writes line with newline.

**Examples:**
```moo
file_writeline(handle, "Hello, World!");
```

---

### 2.2 file_write

**Signature:** `file_write(handle, data) → none`

**Description:** Writes data without newline.

**Examples:**
```moo
file_write(handle, "partial");
file_write(handle, " data\n");
```

---

## 3. File Position

### 3.1 file_tell

**Signature:** `file_tell(handle) → INT`

**Description:** Returns current position.

---

### 3.2 file_seek

**Signature:** `file_seek(handle, position [, whence]) → none`

**Description:** Moves to position.

**Whence:**
| Value | From |
|-------|------|
| 0 | Start (default) |
| 1 | Current |
| 2 | End |

**Examples:**
```moo
file_seek(handle, 0);       // Go to start
file_seek(handle, -10, 2);  // 10 bytes before end
```

---

### 3.3 file_eof

**Signature:** `file_eof(handle) → BOOL`

**Description:** Tests if at end of file.

---

### 3.4 file_flush (ToastStunt)

**Signature:** `file_flush(handle) → none`

**Description:** Flushes buffered output to disk.

**Examples:**
```moo
file_write(handle, "important data");
file_flush(handle);  // Ensure written to disk
```

---

## 4. File Handle Management

### 4.1 file_handles (ToastStunt)

**Signature:** `file_handles() → LIST`

**Description:** Returns list of currently open file handles.

**Examples:**
```moo
handles = file_handles();
// => {1, 2, 3}
```

---

### 4.2 file_name (ToastStunt)

**Signature:** `file_name(handle) → STR`

**Description:** Returns file path associated with handle.

**Examples:**
```moo
file_name(handle)
// => "/data/file.txt"
```

---

### 4.3 file_openmode (ToastStunt)

**Signature:** `file_openmode(handle) → STR`

**Description:** Returns mode string used to open handle.

**Examples:**
```moo
file_openmode(handle)
// => "r-tn"
```

---

### 4.4 file_grep (ToastStunt)

**Signature:** `file_grep(handle, pattern [, case_sensitive]) → LIST`

**Description:** Searches file for lines matching pattern.

**Parameters:**
- `handle`: File handle
- `pattern`: String or regex pattern to search for
- `case_sensitive`: Optional, defaults to case-sensitive (1)

**Returns:** List of matching lines.

**Examples:**
```moo
matches = file_grep(handle, "error");
// => {"Error on line 1", "Fatal error occurred"}
```

---

### 4.5 file_count_lines (ToastStunt)

**Signature:** `file_count_lines(handle) → INT`

**Description:** Returns total number of lines in file.

**Examples:**
```moo
count = file_count_lines(handle);
// => 42
```

---

## 5. File Information

### 5.1 file_size

**Signature:** `file_size(path) → INT`

**Description:** Returns file size in bytes.

**Errors:**
- E_FILE: File not found

---

### 5.2 file_stat (ToastStunt)

**Signature:** `file_stat(path) → LIST`

**Description:** Returns file metadata.

**Returns:**
```moo
{size, atime, mtime, ctime, is_dir, is_file, is_link}
```

---

### 5.3 file_type (ToastStunt)

**Signature:** `file_type(path) → STR`

**Description:** Returns file type.

**Returns:** "file", "directory", "link", or "unknown"

---

## 6. Directory Operations

### 6.1 file_list

**Signature:** `file_list(path [, details]) → LIST`

**Description:** Lists directory contents.

**Parameters:**
- `path`: Directory path
- `details`: If true, include file info

**Examples:**
```moo
files = file_list("/data");
// => {"file1.txt", "file2.txt", "subdir"}

files = file_list("/data", 1);
// => {{"file1.txt", 1024, 1}, {"file2.txt", 512, 1}, {"subdir", 0, 0}}
// Format: {name, size, is_file}
```

---

### 6.2 file_mkdir (ToastStunt)

**Signature:** `file_mkdir(path) → none`

**Description:** Creates directory.

---

### 6.3 file_rmdir (ToastStunt)

**Signature:** `file_rmdir(path) → none`

**Description:** Removes empty directory.

---

## 7. File Management

### 7.1 file_rename

**Signature:** `file_rename(old_path, new_path) → none`

**Description:** Renames/moves file.

---

### 7.2 file_remove

**Signature:** `file_remove(path) → none`

**Description:** Deletes file.

---

### 7.3 file_chmod (ToastStunt)

**Signature:** `file_chmod(path, mode) → none`

**Description:** Changes file permissions.

**Mode:** Unix permission bits (e.g., 0644)

---

## 8. Security

### 8.1 Sandboxing

File I/O is typically sandboxed:
- Only access allowed directories
- Path traversal blocked (../)
- Symlinks may be restricted

### 8.2 Permissions

- Wizard: Full access (within sandbox)
- Programmer: May be restricted
- Regular user: Usually no access

---

## 9. Error Handling

| Error | Condition |
|-------|-----------|
| E_FILE | I/O error |
| E_PERM | Permission denied |
| E_INVARG | Invalid mode/path |
| E_ARGS | Wrong arguments |

---

## 10. Go Implementation Notes

```go
type FileHandle struct {
    ID     int
    Path   string
    File   *os.File
    Mode   string
}

var fileHandles = make(map[int]*FileHandle)
var nextHandle = 1

func builtinFileOpen(args []Value) (Value, error) {
    path := string(args[0].(StringValue))
    mode := string(args[1].(StringValue))

    // Security: validate path
    if !isAllowedPath(path) {
        return nil, E_PERM
    }

    var flag int
    switch mode {
    case "r":
        flag = os.O_RDONLY
    case "w":
        flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
    case "a":
        flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
    case "rb":
        flag = os.O_RDONLY
    case "wb":
        flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
    default:
        return nil, E_INVARG
    }

    file, err := os.OpenFile(path, flag, 0644)
    if err != nil {
        return nil, E_FILE
    }

    handle := nextHandle
    nextHandle++
    fileHandles[handle] = &FileHandle{
        ID:   handle,
        Path: path,
        File: file,
        Mode: mode,
    }

    return IntValue(handle), nil
}

func builtinFileReadline(args []Value) (Value, error) {
    handleID := int(args[0].(IntValue))
    h, ok := fileHandles[handleID]
    if !ok {
        return nil, E_INVARG
    }

    reader := bufio.NewReader(h.File)
    line, err := reader.ReadString('\n')
    if err == io.EOF {
        return StringValue(""), nil
    }
    if err != nil {
        return nil, E_FILE
    }

    // Strip newline
    line = strings.TrimSuffix(line, "\n")
    line = strings.TrimSuffix(line, "\r")

    return StringValue(line), nil
}
```
