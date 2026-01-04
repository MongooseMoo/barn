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

## 2. Security

### 2.1 Allowed Commands

Server may restrict which commands can be executed:
- Whitelist of allowed programs
- Blacklist of forbidden programs
- Path restrictions

### 2.2 Sandboxing

Executed processes may be sandboxed:
- Limited filesystem access
- Network restrictions
- Resource limits (CPU, memory)

### 2.3 Permission Requirements

- Usually requires wizard
- May be allowed for programmers with restrictions
- Never available to regular users

---

## 3. Error Handling

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

## 4. Common Patterns

### 4.1 Capture Output

```moo
{code, out, err} = exec({"command"});
if (code == 0)
    process_output(out);
else
    log_error(err);
endif
```

### 4.2 Pipeline Simulation

```moo
// No direct pipe support in list form
// Use shell form for pipes:
result = exec("cat file.txt | grep pattern | wc -l");

// Or chain manually:
{_, data, _} = exec({"cat", "file.txt"});
{_, filtered, _} = exec({"grep", "pattern"}, data);
{_, count, _} = exec({"wc", "-l"}, filtered);
```

### 4.3 Error Checking

```moo
{code, out, err} = exec({"command"});
if (code != 0)
    raise(E_EXEC, err);
endif
return out;
```

---

## 5. Resource Limits

| Limit | Description |
|-------|-------------|
| Timeout | Maximum execution time |
| Output size | Maximum stdout/stderr |
| Processes | Maximum concurrent |

---

## 6. Go Implementation Notes

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
```
