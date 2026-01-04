# MOO Process Execution Built-ins

## Overview

Functions for executing external processes. Requires exec to be enabled in server configuration.

**IMPORTANT:** exec() is an asynchronous operation that suspends the current task until the external process completes.

---

## 1. Process Execution

### 1.1 exec

**Signature:** `exec(command [, input [, environment]]) → LIST`

**Description:** Executes external command asynchronously and returns output. The task is suspended until the process completes.

**Parameters:**
- `command`: LIST of strings - first element is program name, rest are arguments
- `input`: Optional string to send to stdin
- `environment`: Optional list of strings - environment variables in "KEY=VALUE" format

**Returns:** `{exit_code, stdout, stderr}` (when task resumes)

**Examples:**
```moo
// Simple command
result = exec({"ls", "-la"});
{exit_code, stdout, stderr} = result;

// With input
result = exec({"cat"}, "Hello, World!");
// stdout => "Hello, World!"

// With environment variables
result = exec({"printenv"}, "", {"MY_VAR=test", "DEBUG=1"});
```

**Errors:**
- E_INVARG: Invalid command, path, or arguments
- E_PERM: Not a wizard (exec requires wizard permission)
- E_TYPE: Command is not a list

**CRITICAL:** The task suspends when exec() is called and resumes when the process completes. Use fork/task_id if you need non-blocking execution.

---

## 2. Security and Path Restrictions

### 2.1 Executables Directory

All commands are executed from the server's `executables/` subdirectory. Commands cannot specify absolute paths or traverse outside this directory.

**Example:**
```moo
exec({"test_io"})  // Runs executables/test_io
```

### 2.2 Path Restrictions

The following path forms are **forbidden** and raise E_INVARG:

- **Absolute paths:** `/bin/ls`, `\Windows\system32\cmd.exe`, `C:\Program Files\tool.exe`
- **Parent directory traversal:** `../../../etc/passwd`, `..`
- **Relative path markers:** `./script.sh`, `foo/../bar`

**Valid examples:**
```moo
exec({"mycommand"})           // OK: simple name
exec({"subdir/mycommand"})    // OK: subdirectory (if allowed)
```

**Invalid examples:**
```moo
exec({"/bin/ls"})             // E_INVARG: absolute path
exec({"./mycommand"})         // E_INVARG: ./ prefix
exec({"../mycommand"})        // E_INVARG: parent directory
exec({"foo/../../bar"})       // E_INVARG: path traversal
```

### 2.3 Permission Requirements

- **Requires wizard permission** - exec() raises E_PERM for non-wizards
- No exceptions - programmers and regular players cannot use exec()

### 2.4 Additional Security

Servers may implement additional restrictions:
- Whitelist of allowed programs
- Resource limits (CPU, memory, execution time)
- Network access restrictions
- Filesystem sandboxing

---

## 3. Error Handling

### 3.1 MOO Errors

| Error | Condition |
|-------|-----------|
| E_PERM | Not a wizard |
| E_INVARG | Invalid command path, arguments, or input |
| E_TYPE | Command is not a list |
| E_ARGS | Wrong argument count |

**E_INVARG conditions:**
- Empty command list
- Command path is empty string
- Absolute path (starts with /, \, or drive letter)
- Path contains parent directory (..)
- Path contains ./ or path traversal
- Command file doesn't exist in executables/ directory
- Input contains invalid binary escape (e.g., ~ZZ)
- List elements are not all strings

### 3.2 Exit Codes

The first element of the return list is the process exit code:

- **0:** Success
- **Non-zero:** Command-specific error (1-255 typically)
- **Platform-specific:** May include signal numbers on Unix

---

## 4. Common Patterns

### 4.1 Capture Output

```moo
{code, out, err} = exec({"command", "arg1", "arg2"});
if (code == 0)
    process_output(out);
else
    log_error(err);
endif
```

### 4.2 Sending Input

```moo
// Send data to command via stdin
input_data = "Hello, World!";
{code, output, errors} = exec({"process_input"}, input_data);
```

### 4.3 Environment Variables

```moo
// Set custom environment for the process
env = {"PATH=/usr/bin:/bin", "DEBUG=1", "LANG=en_US.UTF-8"};
{code, out, err} = exec({"myprogram"}, "", env);
```

### 4.4 Error Checking

```moo
{code, out, err} = exec({"command"});
if (code != 0)
    raise(E_INVARG, tostr("Command failed: ", err));
endif
return out;
```

### 4.5 Async Execution with Fork

```moo
// Run exec in background task
fork task_id (0)
    {code, out, err} = exec({"long_running_command"});
    process_results(code, out, err);
endfork
// Task continues immediately, exec runs in forked task
```

---

## 5. Resource Limits

ToastStunt implements various resource limits for exec():

| Limit | Description |
|-------|-------------|
| Timeout | Maximum execution time before termination |
| Output size | Maximum stdout/stderr buffer size |
| Concurrent processes | Maximum simultaneous exec() operations (EXEC_MAX_PROCESSES) |

These limits are server-configurable and may vary by installation.

---

## 6. Task Suspension Behavior

When exec() is called:

1. **Task suspends immediately** - The calling task enters a suspended state
2. **Process starts** - External command begins execution
3. **Server continues** - Other tasks continue running
4. **Process completes** - When the external process exits, output is captured
5. **Task resumes** - The suspended task resumes with the result list

**Key points:**
- The suspension is mandatory - exec() cannot return synchronously
- Multiple tasks can have exec() calls running concurrently (up to server limit)
- Task can be killed with kill_task() while process is running
- resume() cannot force an exec task to resume early (raises E_INVARG)
- queued_tasks() shows exec tasks in suspended state

**Example:**
```moo
// This suspends the current task
player:tell("Starting command...");
{code, out, err} = exec({"sleep", "5"});
player:tell("Command finished!");  // Executes 5 seconds later
```

---

## 7. Implementation Notes

### 7.1 Toast Implementation

ToastStunt's exec() implementation (src/exec.cc):
- Uses POSIX fork/exec on Unix, CreateProcess on Windows
- Maintains process table (EXEC_MAX_PROCESSES slots)
- Captures stdout/stderr via pipes
- Handles SIGCHLD on Unix for process completion
- Validates paths before execution
- Prepends executables/ directory to command path

### 7.2 Key Functions

- `bf_exec()` - Main builtin function, validates and starts process
- `exec_complete()` - Signal handler for child process completion
- `deal_with_child_exit()` - Resumes suspended task with results
- `exec_waiter_suspender()` - Task suspension mechanism
