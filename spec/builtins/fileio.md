# MOO File I/O Built-ins

## Overview

Functions for file system operations. All file I/O functions require wizard permissions.

> **Note:** File I/O functions are compiled into ToastStunt by default (`#define FILE_IO 1` in `src/fileio.cc`) but all functions require wizard permissions to execute. Non-wizards receive E_PERM. All functions are implemented and verified against Toast source code.

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

**Signature:** `file_readlines(handle, start, end) → LIST`

**Description:** Reads multiple lines from file.

**Parameters:**
- `handle`: File handle (INT)
- `start`: First line (1-based, INT)
- `end`: Last line (INT)

**Examples:**
```moo
lines = file_readlines(handle, 1, 999999);  // Read many lines
lines = file_readlines(handle, 1, 10);      // First 10 lines
lines = file_readlines(handle, 5, 15);      // Lines 5-15
```

**Note:** All three arguments are required. Use a large end value to read to EOF.

---

### 1.5 file_read

**Signature:** `file_read(handle, bytes) → STR`

**Description:** Reads specified number of bytes from file.

**Parameters:**
- `handle`: File handle (INT)
- `bytes`: Number of bytes to read (INT)

**Examples:**
```moo
data = file_read(handle, 1024);   // Read up to 1024 bytes
data = file_read(handle, 999999); // Read many bytes (will stop at EOF)
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

**Signature:** `file_write(handle, data) → INT`

**Description:** Writes data without newline.

**Returns:** Number of bytes written.

**Examples:**
```moo
bytes = file_write(handle, "partial");
bytes = file_write(handle, " data\n");
```

---

## 3. File Position

### 3.1 file_tell

**Signature:** `file_tell(handle) → INT`

**Description:** Returns current position.

---

### 3.2 file_seek

**Signature:** `file_seek(handle, position, whence) → none`

**Description:** Moves to position in file.

**Parameters:**
- `handle`: File handle (INT)
- `position`: Byte position (INT)
- `whence`: Position reference (STR)

**Whence values:**
| Value | From |
|-------|------|
| "SEEK_SET" | Start of file |
| "SEEK_CUR" | Current position |
| "SEEK_END" | End of file |

**Examples:**
```moo
file_seek(handle, 0, "SEEK_SET");      // Go to start
file_seek(handle, -10, "SEEK_END");    // 10 bytes before end
file_seek(handle, 100, "SEEK_CUR");    // Move forward 100 bytes
```

**Note:** Whence parameter is case-insensitive.

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

**Signature:** `file_size(path_or_handle) → INT`

**Description:** Returns file size in bytes.

**Parameters:**
- `path_or_handle`: File path (STR) or file handle (INT)

**Examples:**
```moo
size = file_size("/data/file.txt");  // => 1024
size = file_size(handle);             // => 1024
```

**Errors:**
- E_FILE: File not found
- E_INVARG: Invalid handle

---

### 5.2 file_stat (ToastStunt)

**Signature:** `file_stat(path_or_handle) → LIST`

**Description:** Returns file metadata.

**Parameters:**
- `path_or_handle`: File path (STR) or file handle (INT)

**Returns:** List with 8 elements:
```moo
{size, type, mode, owner, group, atime, mtime, ctime}
```

| Position | Type | Description |
|----------|------|-------------|
| 1 | INT | File size in bytes |
| 2 | STR | File type: "reg", "dir", "fifo", "block", "socket", "unknown" |
| 3 | STR | File mode (octal string, e.g., "644") |
| 4 | STR | Owner (always empty string) |
| 5 | STR | Group (always empty string) |
| 6 | INT | Last access time (Unix timestamp) |
| 7 | INT | Last modify time (Unix timestamp) |
| 8 | INT | Last change time (Unix timestamp) |

**Examples:**
```moo
stat = file_stat("/data/file.txt");
// => {1024, "reg", "644", "", "", 1234567890, 1234567890, 1234567890}

stat = file_stat(handle);
// => {2048, "reg", "600", "", "", 1234567891, 1234567891, 1234567891}
```

---

### 5.3 file_type (ToastStunt)

**Signature:** `file_type(path_or_handle) → STR`

**Description:** Returns file type.

**Parameters:**
- `path_or_handle`: File path (STR) or file handle (INT)

**Returns:** One of:
- "reg" - Regular file
- "dir" - Directory
- "fifo" - Named pipe (FIFO)
- "block" - Block device
- "socket" - Socket
- "unknown" - Unknown type

**Examples:**
```moo
file_type("/data/file.txt")  // => "reg"
file_type("/data/subdir")    // => "dir"
file_type(handle)            // => "reg"
```

---

### 5.4 file_mode (ToastStunt)

**Signature:** `file_mode(path_or_handle) → STR`

**Description:** Returns file permission mode as octal string.

**Parameters:**
- `path_or_handle`: File path (STR) or file handle (INT)

**Examples:**
```moo
file_mode("/data/file.txt")  // => "644"
file_mode("/data/script.sh") // => "755"
file_mode(handle)            // => "600"
```

---

### 5.5 file_last_access (ToastStunt)

**Signature:** `file_last_access(path_or_handle) → INT`

**Description:** Returns last access time as Unix timestamp.

**Parameters:**
- `path_or_handle`: File path (STR) or file handle (INT)

**Examples:**
```moo
atime = file_last_access("/data/file.txt");  // => 1234567890
atime = file_last_access(handle);            // => 1234567890
```

---

### 5.6 file_last_modify (ToastStunt)

**Signature:** `file_last_modify(path_or_handle) → INT`

**Description:** Returns last modification time as Unix timestamp.

**Parameters:**
- `path_or_handle`: File path (STR) or file handle (INT)

**Examples:**
```moo
mtime = file_last_modify("/data/file.txt");  // => 1234567890
mtime = file_last_modify(handle);            // => 1234567890
```

---

### 5.7 file_last_change (ToastStunt)

**Signature:** `file_last_change(path_or_handle) → INT`

**Description:** Returns last status change time (ctime) as Unix timestamp.

**Note:** This is inode change time (metadata change), not file creation time.

**Parameters:**
- `path_or_handle`: File path (STR) or file handle (INT)

**Examples:**
```moo
ctime = file_last_change("/data/file.txt");  // => 1234567890
ctime = file_last_change(handle);            // => 1234567890
```

---

## 6. Directory Operations

### 6.1 file_list

**Signature:** `file_list(path [, details]) → LIST`

**Description:** Lists directory contents.

**Parameters:**
- `path`: Directory path (STR)
- `details`: If true, include detailed file info (optional)

**Returns:**
- Simple mode: List of filenames (strings)
- Detailed mode: List of lists with file information

**Detailed entry format:**
```moo
{filename, file_type, file_mode, file_size}
```

| Position | Type | Description |
|----------|------|-------------|
| 1 | STR | Filename |
| 2 | STR | File type ("reg", "dir", etc.) |
| 3 | STR | File mode (octal string) |
| 4 | INT | File size in bytes |

**Examples:**
```moo
files = file_list("/data");
// => {"file1.txt", "file2.txt", "subdir"}

files = file_list("/data", 1);
// => {{"file1.txt", "reg", "644", 1024},
//     {"file2.txt", "reg", "644", 512},
//     {"subdir", "dir", "755", 0}}
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

**Parameters:**
- `path`: File path (STR)
- `mode`: Permission mode as octal string (STR)

**Examples:**
```moo
file_chmod("/data/file.txt", "644");   // rw-r--r--
file_chmod("/data/script.sh", "755");  // rwxr-xr-x
file_chmod("/data/private.txt", "600"); // rw-------
```

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
