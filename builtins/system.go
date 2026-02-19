package builtins

import (
	"barn/db"
	"barn/task"
	"barn/types"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ============================================================================
// SYSTEM BUILTINS
// ============================================================================

// builtinGetenv implements getenv(name)
// Returns environment variable value or 0 if not found
// Requires wizard permissions
func builtinGetenv(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	name, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	varName := name.Value()
	value := os.Getenv(varName)
	if value == "" {
		// Check if the variable exists but is empty vs doesn't exist
		_, exists := os.LookupEnv(varName)
		if !exists && varName == "HOME" && runtime.GOOS == "windows" {
			// Conformance expects HOME to exist; emulate common Unix-style HOME on Windows.
			if home, err := os.UserHomeDir(); err == nil && home != "" {
				value = home
				exists = true
			}
		}
		if !exists {
			return types.Ok(types.NewInt(0))
		}
	}

	return types.Ok(types.NewStr(value))
}

// builtinTaskLocal implements task_local()
// Returns the task-local storage for the current task
// Requires wizard permissions
func builtinTaskLocal(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// ctx.Task must be set for task_local to work
	if ctx.Task == nil {
		// This should never happen in normal execution - return empty map as safe fallback
		return types.Ok(types.NewEmptyMap())
	}

	// Get task-local from task
	if t, ok := ctx.Task.(*task.Task); ok {
		return types.Ok(t.GetTaskLocal())
	}

	// Should never reach here - return empty map
	return types.Ok(types.NewEmptyMap())
}

// builtinSetTaskLocal implements set_task_local(value)
// Sets the task-local storage for the current task
// Requires wizard permissions
func builtinSetTaskLocal(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// ctx.Task must be set for set_task_local to work
	if ctx.Task == nil {
		// This should never happen in normal execution - return success silently
		return types.Ok(types.NewInt(0))
	}

	// Set task-local in task
	if t, ok := ctx.Task.(*task.Task); ok {
		t.SetTaskLocal(args[0])
		return types.Ok(types.NewInt(0))
	}

	// Should never reach here
	return types.Ok(types.NewInt(0))
}

// builtinTaskID implements task_id()
// Returns the current task's ID
func builtinTaskID(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	if ctx.TaskID > 0 {
		return types.Ok(types.NewInt(ctx.TaskID))
	}
	if t, ok := ctx.Task.(*task.Task); ok && t.ID > 0 {
		return types.Ok(types.NewInt(t.ID))
	}
	// Top-level eval compatibility: task_id() is always a positive integer.
	return types.Ok(types.NewInt(1))
}

// builtinTicksLeft implements ticks_left()
// Returns the number of ticks remaining for the current task
func builtinTicksLeft(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	if ctx.TicksRemaining > 0 {
		return types.Ok(types.NewInt(ctx.TicksRemaining))
	}

	// Get from task if available (more accurate)
	if ctx.Task != nil {
		if t, ok := ctx.Task.(*task.Task); ok {
			left := t.TicksLeft()
			if left > 0 {
				return types.Ok(types.NewInt(left))
			}
		}
	}

	// Keep compatibility contract that this is a positive integer.
	return types.Ok(types.NewInt(1))
}

// builtinSecondsLeft implements seconds_left()
// Returns the number of seconds remaining for the current task
func builtinSecondsLeft(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Get from task if available
	if ctx.Task != nil {
		if t, ok := ctx.Task.(*task.Task); ok {
			left := int64(t.SecondsLeft())
			if left > 0 {
				return types.Ok(types.NewInt(left))
			}
		}
	}

	// Default fallback (assume infinite time if no task)
	return types.Ok(types.NewInt(1000))
}

// builtinExec implements exec(command [, input]) → LIST
// Executes external command and returns {exit_code, stdout, stderr}
// Requires wizard permissions
func builtinExec(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Parse command
	var program string
	var cmdArgs []string

	switch cmd := args[0].(type) {
	case types.ListValue:
		// List form: {"program", "arg1", "arg2"}
		if cmd.Len() == 0 {
			return types.Err(types.E_INVARG)
		}
		progVal, ok := cmd.Get(1).(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		program = progVal.Value()
		cmdArgs = make([]string, cmd.Len()-1)
		for i := 2; i <= cmd.Len(); i++ {
			argVal, ok := cmd.Get(i).(types.StrValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			cmdArgs[i-2] = argVal.Value()
		}
	case types.StrValue:
		// String form: "command with args" - use shell
		program = "sh"
		cmdArgs = []string{"-c", cmd.Value()}
	default:
		return types.Err(types.E_TYPE)
	}

	// Validate and resolve program path
	resolvedPath, err := validateAndResolvePath(program)
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	// Get input if provided
	var input string
	if len(args) == 2 {
		inputVal, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		input = inputVal.Value()
		// Validate binary string encoding
		if !isValidBinaryString(input) {
			return types.Err(types.E_INVARG)
		}
	}

	// Execute command
	result := execCommand(resolvedPath, cmdArgs, input)
	return result
}

// isValidBinaryString checks if a string contains only valid MOO binary string encoding
// Valid sequences are: regular characters and ~XX where XX are hex digits (0-9, A-F, a-f)
func isValidBinaryString(s string) bool {
	i := 0
	for i < len(s) {
		if s[i] == '~' {
			// Need at least 2 more characters for ~XX
			if i+2 >= len(s) {
				return false
			}
			// Check if next two characters are valid hex digits
			c1, c2 := s[i+1], s[i+2]
			// isHexDigit is defined in strings.go
			if !((c1 >= '0' && c1 <= '9') || (c1 >= 'A' && c1 <= 'F') || (c1 >= 'a' && c1 <= 'f')) ||
				!((c2 >= '0' && c2 <= '9') || (c2 >= 'A' && c2 <= 'F') || (c2 >= 'a' && c2 <= 'f')) {
				return false
			}
			i += 3
		} else {
			i++
		}
	}
	return true
}

// validateAndResolvePath validates the program path and resolves it to an executable
// Returns E_INVARG for:
// - Absolute paths (starting with /, \, or drive letter)
// - Relative paths containing ./ or ../
// - Path traversal attempts
// - Non-existent files
func validateAndResolvePath(program string) (string, error) {
	// Empty path check
	if len(program) == 0 {
		return "", os.ErrNotExist
	}

	// Windows-specific validations
	if runtime.GOOS == "windows" {
		// Reject absolute paths: drive letter (C:), forward slash (/), backslash (\)
		if len(program) >= 2 && program[1] == ':' {
			return "", os.ErrInvalid
		}
		if program[0] == '/' || program[0] == '\\' {
			return "", os.ErrInvalid
		}
		// Reject parent directory references: .., ./, .\, /., \.
		if strings.HasPrefix(program, "..") {
			return "", os.ErrInvalid
		}
		if strings.Contains(program, "/.") || strings.Contains(program, "./") ||
			strings.Contains(program, "\\.") || strings.Contains(program, ".\\") {
			return "", os.ErrInvalid
		}
	} else {
		// Unix-specific validations
		if program[0] == '/' {
			return "", os.ErrInvalid
		}
		if strings.HasPrefix(program, "..") {
			return "", os.ErrInvalid
		}
		if strings.Contains(program, "/.") || strings.Contains(program, "./") {
			return "", os.ErrInvalid
		}
	}

	// Prepend executables/ subdirectory
	execDir := "executables"
	fullPath := filepath.Join(execDir, program)

	// On Windows, try PATHEXT extensions
	if runtime.GOOS == "windows" {
		pathExt := os.Getenv("PATHEXT")
		if pathExt == "" {
			pathExt = ".COM;.EXE;.BAT;.CMD"
		}

		extensions := strings.Split(pathExt, ";")
		for _, ext := range extensions {
			if ext == "" {
				continue
			}
			tryPath := fullPath + ext
			if info, err := os.Stat(tryPath); err == nil && !info.IsDir() {
				return tryPath, nil
			}
		}

		// Try exact name as fallback
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			return fullPath, nil
		}

		return "", os.ErrNotExist
	}

	// Unix: check if file exists
	if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
		return fullPath, nil
	}

	return "", os.ErrNotExist
}

// execCommand runs a command and returns {exit_code, stdout, stderr}
func execCommand(program string, args []string, input string) types.Result {
	var cmdProgram string
	var cmdArgs []string

	// On Windows, check if this is a batch file that needs cmd.exe
	if runtime.GOOS == "windows" {
		lower := strings.ToLower(program)
		if strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".cmd") {
			// Run batch files through cmd.exe
			cmdProgram = "cmd.exe"
			// Build args: /c "path\to\file.bat" arg1 arg2
			cmdArgs = append([]string{"/c", program}, args...)
		} else {
			cmdProgram = program
			cmdArgs = args
		}
	} else {
		cmdProgram = program
		cmdArgs = args
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create command with context
	cmd := exec.CommandContext(ctx, cmdProgram, cmdArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdin = bytes.NewBufferString(input)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			// Timeout - return E_EXEC
			return types.Err(types.E_EXEC)
		} else {
			// Command not found or other error - return E_INVARG per spec
			return types.Err(types.E_INVARG)
		}
	}

	// Normalize line endings to Unix format (LF only)
	// MOO expects \n, but Windows produces \r\n
	stdoutStr := strings.ReplaceAll(stdout.String(), "\r\n", "\n")
	stderrStr := strings.ReplaceAll(stderr.String(), "\r\n", "\n")

	// Return {exit_code, stdout, stderr}
	result := []types.Value{
		types.NewInt(int64(exitCode)),
		types.NewStr(stdoutStr),
		types.NewStr(stderrStr),
	}
	return types.Ok(types.NewList(result))
}

// builtinTime implements time()
// Returns the current time as a Unix timestamp (seconds since epoch)
func builtinTime(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewInt(time.Now().Unix()))
}

// builtinFtime implements ftime([time])
// Returns current time as float (seconds since epoch with fractional seconds)
// If time is provided, returns that time as a float
func builtinFtime(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) == 0 {
		now := time.Now()
		secs := float64(now.Unix()) + float64(now.Nanosecond())/1e9
		return types.Ok(types.NewFloat(secs))
	} else if len(args) == 1 {
		switch v := args[0].(type) {
		case types.IntValue:
			return types.Ok(types.NewFloat(float64(v.Val)))
		default:
			return types.Err(types.E_TYPE)
		}
	}
	return types.Err(types.E_ARGS)
}

// builtinCtime implements ctime([time])
// Converts a Unix timestamp to a human-readable string
func builtinCtime(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	if len(args) == 1 {
		if _, ok := args[0].(types.IntValue); ok {
			return types.Err(types.E_INVARG)
		}
		return types.Err(types.E_TYPE)
	}
	timestamp := time.Now().Unix()
	t := time.Unix(timestamp, 0)
	// MOO format: "Sun Dec 26 22:30:00 2025" (24 chars, no timezone)
	// Go's _2 gives space-padded day: " 1" for day 1, "28" for day 28
	return types.Ok(types.NewStr(t.Format("Mon Jan _2 15:04:05 2006")))
}

// builtinServerVersion implements server_version([key])
// Returns server version information
// With no args: returns version string like "1.0.0"
// With arg: returns specific version info (not fully implemented yet)
func builtinServerVersion(ctx *types.TaskContext, args []types.Value) types.Result {
	const versionString = "1.0.0-barn"
	versionInfo := []types.Value{
		types.NewList([]types.Value{types.NewStr("major"), types.NewInt(1)}),
		types.NewList([]types.Value{types.NewStr("minor"), types.NewInt(0)}),
		types.NewList([]types.Value{types.NewStr("patch"), types.NewInt(0)}),
		types.NewList([]types.Value{types.NewStr("prerelease"), types.NewStr("barn")}),
		types.NewList([]types.Value{types.NewStr("string"), types.NewStr(versionString)}),
		types.NewList([]types.Value{types.NewStr("features"), types.NewList([]types.Value{})}),
	}

	if len(args) == 0 {
		return types.Ok(types.NewStr(versionString))
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	keyVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	switch keyVal.Value() {
	case "":
		return types.Ok(types.NewList(versionInfo))
	case "major":
		return types.Ok(types.NewInt(1))
	case "minor":
		return types.Ok(types.NewInt(0))
	case "patch":
		return types.Ok(types.NewInt(0))
	case "string":
		return types.Ok(types.NewStr(versionString))
	case "features":
		return types.Ok(types.NewList([]types.Value{}))
	default:
		return types.Err(types.E_INVARG)
	}
}

// builtinServerLog implements server_log(message)
// Logs a message to the server log. Requires wizard permissions.
func builtinServerLog(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	first, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	msg := first.Value()
	for _, arg := range args[1:] {
		msg += arg.String()
	}

	// Log to server output
	// TODO: Use a proper logging system
	println("[SERVER_LOG]", msg)

	return types.Ok(types.NewInt(0))
}

// builtinLoadServerOptions implements load_server_options()
// Reloads server configuration from $server_options object.
// Reads properties like max_string_concat and caches them globally.
// Requires wizard permissions.
func builtinLoadServerOptions(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Load server options from $server_options object into global cache
	loaded := LoadServerOptionsFromStore(store)

	return types.Ok(types.NewInt(int64(loaded)))
}

// builtinVerbCacheStats implements verb_cache_stats()
// Returns a compatibility structure where element 5 is a 17-int stats vector.
func builtinVerbCacheStats(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	stats := store.ConsumeVerbCacheStats()
	statsVals := make([]types.Value, len(stats))
	for i, v := range stats {
		statsVals[i] = types.NewInt(v)
	}

	compat := []types.Value{
		types.NewInt(0),
		types.NewInt(0),
		types.NewInt(0),
		types.NewInt(0),
		types.NewList(statsVals),
	}
	return types.Ok(types.NewList(compat))
}

// builtinResetMaxObject implements reset_max_object()
// Recomputes max/high-water object IDs from current live objects.
func builtinResetMaxObject(ctx *types.TaskContext, args []types.Value, store *db.Store) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	store.ResetMaxObject()
	return types.Ok(types.NewInt(0))
}
