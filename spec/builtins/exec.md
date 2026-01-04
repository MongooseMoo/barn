# MOO Process Execution Built-ins

## Overview

Functions for executing external processes. Requires exec to be enabled in server configuration.

---

## 1. Process Execution

### 1.1 exec

**Signature:** `exec(command [, input]) → LIST`

**Description:** Executes external command and returns output.

**Parameters:**
- `command`: List of command and arguments, or string
- `input`: Optional string to send to stdin

**Returns:** `{exit_code, stdout, stderr}`

**Examples:**
```moo
// Simple command
result = exec({"ls", "-la"});
{exit_code, stdout, stderr} = result;

// With input
result = exec({"cat"}, "Hello, World!");
// stdout => "Hello, World!"

// String form (parsed by shell)
result = exec("echo hello");
```

**Errors:**
- E_EXEC: Command not found or execution failed
- E_PERM: Not allowed to exec

---

### 1.2 Command Forms

**List form (recommended):**
```moo
exec({"program", "arg1", "arg2"})
// Direct execution, no shell interpretation
```

**String form (requires shell in executables directory):**
```moo
exec("program arg1 arg2")
// Passed to shell, supports pipes, redirects
// Note: Requires shell executable (e.g., "sh") in executables/ directory
```

---

## 2. Execution Options

### 2.1 exec_async() [Not Implemented]

**Signature:** `exec_async(command [, callback_obj, callback_verb]) → INT`

> **Note:** This function is documented but not implemented in ToastStunt or Barn.

**Description:** Executes command asynchronously.

**Returns:** Process ID.

**Callback:** When process completes, calls:
```moo
callback_obj:callback_verb(pid, exit_code, stdout, stderr)
```

**Examples:**
```moo
pid = exec_async({"long_running_command"}, this, "process_done");

// Later, this:process_done is called with results
```

---

### 2.2 exec_timeout() [Not Implemented]

**Signature:** `exec_timeout(command, timeout [, input]) → LIST`

> **Note:** This function is documented but not implemented in ToastStunt or Barn. The basic `exec()` function has a hardcoded 30-second timeout.

**Description:** Executes with timeout.

**Parameters:**
- `timeout`: Maximum seconds

**Errors:**
- E_EXEC: Timeout exceeded

---

## 3. Process Control

### 3.1 kill_process() [Not Implemented]

**Signature:** `kill_process(pid [, signal]) → none`

> **Note:** This function is documented but not implemented in ToastStunt or Barn.

**Description:** Sends signal to process.

**Signals:**
| Name | Number | Effect |
|------|--------|--------|
| "TERM" | 15 | Terminate |
| "KILL" | 9 | Force kill |
| "INT" | 2 | Interrupt |

---

### 3.2 wait_process() [Not Implemented]

**Signature:** `wait_process(pid) → LIST`

> **Note:** This function is documented but not implemented in ToastStunt or Barn.

**Description:** Waits for async process to complete.

**Returns:** `{exit_code, stdout, stderr}`

---

## 4. Environment

### 4.1 exec_env() [Not Implemented]

**Signature:** `exec_env(command, env [, input]) → LIST`

> **Note:** This function is documented but not implemented in ToastStunt or Barn.

**Description:** Executes with custom environment.

**Parameters:**
- `env`: Map of environment variables

**Examples:**
```moo
result = exec_env(
    {"./script.sh"},
    ["PATH" -> "/usr/bin", "HOME" -> "/tmp"]
);
```

---

## 5. Security

### 5.1 Allowed Commands

Server may restrict which commands can be executed:
- Whitelist of allowed programs
- Blacklist of forbidden programs
- Path restrictions

### 5.2 Sandboxing

Executed processes may be sandboxed:
- Limited filesystem access
- Network restrictions
- Resource limits (CPU, memory)

### 5.3 Permission Requirements

- Usually requires wizard
- May be allowed for programmers with restrictions
- Never available to regular users

---

## 6. Error Handling

| Error | Condition |
|-------|-----------|
| E_EXEC | Execution failed |
| E_PERM | Permission denied |
| E_INVARG | Invalid command |
| E_ARGS | Wrong argument count |

**Exit codes:**
- 0: Success
- Non-zero: Command-specific failure
- -1: Process killed by signal

---

## 7. Common Patterns

### 7.1 Capture Output

```moo
{code, out, err} = exec({"command"});
if (code == 0)
    process_output(out);
else
    log_error(err);
endif
```

### 7.2 Pipeline Simulation

```moo
// No direct pipe support in list form
// Use shell form for pipes:
result = exec("cat file.txt | grep pattern | wc -l");

// Or chain manually:
{_, data, _} = exec({"cat", "file.txt"});
{_, filtered, _} = exec({"grep", "pattern"}, data);
{_, count, _} = exec({"wc", "-l"}, filtered);
```

### 7.3 Error Checking

```moo
{code, out, err} = exec({"command"});
if (code != 0)
    raise(E_EXEC, err);
endif
return out;
```

---

## 8. Resource Limits

| Limit | Description |
|-------|-------------|
| Timeout | Maximum execution time |
| Output size | Maximum stdout/stderr |
| Processes | Maximum concurrent |

---

## 9. Go Implementation Notes

```go
import (
    "bytes"
    "context"
    "os/exec"
    "time"
)

func builtinExec(args []Value) (Value, error) {
    // Check permission
    if !callerIsWizard() {
        return nil, E_PERM
    }

    var cmd *exec.Cmd
    var input []byte

    // Parse command
    switch c := args[0].(type) {
    case *MOOList:
        if len(c.data) == 0 {
            return nil, E_INVARG
        }
        program := string(c.data[0].(StringValue))
        cmdArgs := make([]string, len(c.data)-1)
        for i := 1; i < len(c.data); i++ {
            cmdArgs[i-1] = string(c.data[i].(StringValue))
        }
        cmd = exec.Command(program, cmdArgs...)
    case StringValue:
        // Shell form
        cmd = exec.Command("sh", "-c", string(c))
    default:
        return nil, E_TYPE
    }

    // Input
    if len(args) > 1 {
        input = []byte(string(args[1].(StringValue)))
    }

    // Set up I/O
    var stdout, stderr bytes.Buffer
    cmd.Stdin = bytes.NewReader(input)
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    // Run with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
    cmd.Stdin = bytes.NewReader(input)
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    err := cmd.Run()

    exitCode := 0
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            exitCode = exitErr.ExitCode()
        } else if ctx.Err() == context.DeadlineExceeded {
            return nil, E_EXEC
        } else {
            return nil, E_EXEC
        }
    }

    return &MOOList{data: []Value{
        IntValue(exitCode),
        StringValue(stdout.String()),
        StringValue(stderr.String()),
    }}, nil
}

func builtinExecAsync(args []Value) (Value, error) {
    if !callerIsWizard() {
        return nil, E_PERM
    }

    // Parse command
    cmdList := args[0].(*MOOList)
    program := string(cmdList.data[0].(StringValue))
    cmdArgs := make([]string, len(cmdList.data)-1)
    for i := 1; i < len(cmdList.data); i++ {
        cmdArgs[i-1] = string(cmdList.data[i].(StringValue))
    }

    cmd := exec.Command(program, cmdArgs...)

    var callbackObj, callbackVerb string
    if len(args) > 1 {
        callbackObj = string(args[1].(ObjValue))
        callbackVerb = string(args[2].(StringValue))
    }

    // Start process
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    if err := cmd.Start(); err != nil {
        return nil, E_EXEC
    }

    pid := cmd.Process.Pid
    asyncProcesses[pid] = &AsyncProcess{
        Cmd:          cmd,
        Stdout:       &stdout,
        Stderr:       &stderr,
        CallbackObj:  callbackObj,
        CallbackVerb: callbackVerb,
    }

    // Wait in goroutine
    go func() {
        err := cmd.Wait()
        exitCode := 0
        if err != nil {
            if exitErr, ok := err.(*exec.ExitError); ok {
                exitCode = exitErr.ExitCode()
            }
        }

        // Call callback if specified
        if callbackVerb != "" {
            scheduler.QueueVerbCall(callbackObj, callbackVerb, []Value{
                IntValue(pid),
                IntValue(exitCode),
                StringValue(stdout.String()),
                StringValue(stderr.String()),
            })
        }
    }()

    return IntValue(pid), nil
}
```
