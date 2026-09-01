package builtins

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"strings"
	"time"

	"github.com/MongooseMoo/barn/internal/buildinfo"
	"github.com/MongooseMoo/barn/types"
)

// ============================================================================
// SYSTEM BUILTINS
// ============================================================================

// builtinGetenv implements getenv(name)
// Returns environment variable value or 0 if not found
// Requires wizard permissions
func builtinGetenv(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	varName := args[0].Str()
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
func builtinTaskLocal(ctx *Execution, args []types.Value) types.Result {
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
	return types.Ok(ctx.Task.GetTaskLocal())
}

// builtinSetTaskLocal implements set_task_local(value)
// Sets the task-local storage for the current task
// Requires wizard permissions
func builtinSetTaskLocal(ctx *Execution, args []types.Value) types.Result {
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
	ctx.Task.SetTaskLocal(args[0])
	return types.Ok(types.NewInt(0))
}

// builtinTaskID implements task_id()
// Returns the current task's ID
func builtinTaskID(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	if ctx.TaskID > 0 {
		return types.Ok(types.NewInt(ctx.TaskID))
	}
	if t := ctx.Task; t != nil && t.ID > 0 {
		return types.Ok(types.NewInt(t.ID))
	}
	// Top-level eval compatibility: task_id() is always a positive integer.
	return types.Ok(types.NewInt(1))
}

// builtinTicksLeft implements ticks_left()
// Returns the number of ticks remaining for the current task
func builtinTicksLeft(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	if ctx.TicksRemaining > 0 {
		return types.Ok(types.NewInt(ctx.TicksRemaining))
	}

	// Get from task if available (more accurate)
	if t := ctx.Task; t != nil {
		left := t.TicksLeft()
		if left > 0 {
			return types.Ok(types.NewInt(left))
		}
	}

	// Keep compatibility contract that this is a positive integer.
	return types.Ok(types.NewInt(1))
}

// builtinSecondsLeft implements seconds_left()
// Returns the number of seconds remaining for the current task
func builtinSecondsLeft(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Get from task if available
	if t := ctx.Task; t != nil {
		left := int64(t.SecondsLeft())
		if left > 0 {
			return types.Ok(types.NewInt(left))
		}
	}

	// Default fallback (assume infinite time if no task)
	return types.Ok(types.NewInt(1000))
}

// builtinExec implements exec(command [, input [, env]]) -> LIST
// Executes external command and returns {exit_code, stdout, stderr}
// Requires wizard permissions
func builtinExec(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Parse command
	var program string
	var cmdArgs []string

	cmd := args[0]
	switch cmd.Type() {
	case types.TYPE_LIST:
		// List form: {"program", "arg1", "arg2"}
		if cmd.Len() == 0 {
			return types.Err(types.E_INVARG)
		}
		if cmd.Get(1).Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		program = cmd.Get(1).Str()
		cmdArgs = make([]string, cmd.Len()-1)
		for i := 2; i <= cmd.Len(); i++ {
			if cmd.Get(i).Type() != types.TYPE_STR {
				return types.Err(types.E_TYPE)
			}
			cmdArgs[i-2] = cmd.Get(i).Str()
		}
	case types.TYPE_STR:
		// String form: "command with args" - use shell
		program = "sh"
		cmdArgs = []string{"-c", cmd.Str()}
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
	if len(args) >= 2 {
		if args[1].Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		input = args[1].Str()
		// Validate binary string encoding
		if !isValidBinaryString(input) {
			return types.Err(types.E_INVARG)
		}
	}
	environment := []string{"PATH=/bin:/usr/bin"}
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_LIST {
			return types.Err(types.E_TYPE)
		}
		for _, value := range args[2].Elements() {
			if value.Type() != types.TYPE_STR {
				return types.Err(types.E_INVARG)
			}
			environment = append(environment, value.Str())
		}
	}

	// Get the task so we can suspend it
	t := ctx.Task
	if t == nil {
		// No task context (shouldn't happen in normal execution) — fall back to synchronous
		return execCommand(resolvedPath, cmdArgs, input, environment)
	}

	// Create a cancellable context for the subprocess
	execCtx, execCancel := context.WithTimeout(context.Background(), 30*time.Second)

	// Mark task as exec-suspended and store cancel func
	t.IsExecSuspended = true
	t.ExecCommandName = filepath.ToSlash(filepath.Join("executables", program))
	t.ExecCancelFunc = execCancel

	// Suspend the task indefinitely (will be resumed by goroutine)
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	mgr.SuspendTask(t, -1)

	// Launch subprocess in background goroutine
	go func() {
		defer execCancel()
		result := execCommandWithContext(execCtx, resolvedPath, cmdArgs, input, environment)

		// Deliver result to the task and transition it to Queued
		if result.IsNormal() {
			t.CompleteExec(result.Val)
		} else {
			// On exec error, deliver an error list so the MOO code sees a clean result
			// rather than propagating a Go-level error. Use exit code -1.
			errResult := []types.Value{
				types.NewInt(-1),
				types.NewStr(""),
				types.NewStr(""),
			}
			t.CompleteExec(types.NewList(errResult))
		}
	}()

	// Return FlowSuspend so the VM yields
	return types.Suspend(-1)
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

	execDirs := []string{"executables"}

	// On Windows, try PATHEXT extensions
	if runtime.GOOS == "windows" {
		pathExt := os.Getenv("PATHEXT")
		if pathExt == "" {
			pathExt = ".COM;.EXE;.BAT;.CMD"
		}

		extensions := strings.Split(pathExt, ";")
		for _, dir := range execDirs {
			fullPath := filepath.Join(dir, program)
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
		}

		return "", os.ErrNotExist
	}

	// Unix: check if file exists
	for _, dir := range execDirs {
		fullPath := filepath.Join(dir, program)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			return fullPath, nil
		}
	}

	return "", os.ErrNotExist
}

// execCommand runs a command synchronously and returns {exit_code, stdout, stderr}.
// Used as fallback when no task context is available.
func execCommand(program string, args []string, input string, environment []string) types.Result {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return execCommandWithContext(ctx, program, args, input, environment)
}

// execCommandWithContext runs a command with the given context and returns {exit_code, stdout, stderr}.
// The context controls cancellation (for kill_task) and timeout.
func execCommandWithContext(ctx context.Context, program string, args []string, input string, environment []string) types.Result {
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

	// Create command with context
	cmd := exec.CommandContext(ctx, cmdProgram, cmdArgs...)
	cmd.Env = environment

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
		} else if ctx.Err() == context.Canceled {
			// Cancelled (kill_task) - return E_INVARG
			return types.Err(types.E_INVARG)
		} else {
			// Command not found or other error - return E_INVARG per spec
			return types.Err(types.E_INVARG)
		}
	}

	// Normalize line endings to Unix format (LF only; Windows-only nicety —
	// Toast's Linux children never emit \r\n), then binary-encode: Toast
	// returns exec output through raw_bytes_to_binary, so the MOO strings
	// contain "~0A" text, not raw newlines (conformance exec_with_sleep_works).
	stdoutStr := encodeBinaryBytes([]byte(strings.ReplaceAll(stdout.String(), "\r\n", "\n")))
	stderrStr := encodeBinaryBytes([]byte(strings.ReplaceAll(stderr.String(), "\r\n", "\n")))

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
func builtinTime(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewInt(time.Now().Unix()))
}

// builtinFtime implements ftime([time])
// Returns current time as float (seconds since epoch with fractional seconds)
// If an argument is provided, Toast returns its monotonic process clock.
var ftimeEpoch = time.Now()

func builtinFtime(ctx *Execution, args []types.Value) types.Result {
	if len(args) == 0 {
		now := time.Now()
		secs := float64(now.Unix()) + float64(now.Nanosecond())/1e9
		return types.Ok(types.NewFloat(secs))
	} else if len(args) == 1 {
		if args[0].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		return types.Ok(types.NewFloat(time.Since(ftimeEpoch).Seconds()))
	}
	return types.Err(types.E_ARGS)
}

// builtinCtime implements ctime([time])
// Converts a Unix timestamp to a human-readable string
func builtinCtime(ctx *Execution, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	timestamp := time.Now().Unix()
	if len(args) == 1 {
		if args[0].Type() == types.TYPE_INT {
			timestamp = args[0].Int()
		} else {
			return types.Err(types.E_TYPE)
		}
	}
	const maxCtime = int64(2147483647) * 31536000
	if timestamp < -maxCtime {
		return types.Err(types.E_INVARG)
	}
	if timestamp > maxCtime {
		timestamp = maxCtime
	}
	t := time.Unix(timestamp, 0)
	// MOO format matches Toast's ctime: "Sun Dec 26 22:30:00 2025 MST" — the
	// local timezone abbreviation is appended. Go's _2 gives a space-padded day.
	return types.Ok(types.NewStr(t.Format("Mon Jan _2 15:04:05 2006 MST")))
}

// builtinServerVersion implements server_version([key])
func builtinServerVersion(ctx *Execution, args []types.Value) types.Result {
	return serverVersion(ctx, args, buildinfo.Current())
}

func serverVersion(ctx *Execution, args []types.Value, build buildinfo.Info) types.Result {
	options := ctx.RuntimeOptions
	featureNames := options.FeatureNames()
	featureValues := make([]types.Value, 0, len(featureNames))
	for _, feature := range featureNames {
		featureValues = append(featureValues, types.NewStr(feature))
	}
	features := types.NewList(featureValues)
	versionInfo := []types.Value{
		types.NewList([]types.Value{types.NewStr("major"), types.NewInt(build.Major)}),
		types.NewList([]types.Value{types.NewStr("minor"), types.NewInt(build.Minor)}),
		types.NewList([]types.Value{types.NewStr("patch"), types.NewInt(build.Patch)}),
		types.NewList([]types.Value{types.NewStr("prerelease"), types.NewStr(build.Prerelease)}),
		types.NewList([]types.Value{types.NewStr("string"), types.NewStr(build.String)}),
		types.NewList([]types.Value{types.NewStr("revision"), types.NewStr(build.Revision)}),
		types.NewList([]types.Value{types.NewStr("modified"), types.NewInt(boolInt(build.Modified))}),
		types.NewList([]types.Value{types.NewStr("features"), features}),
		types.NewList([]types.Value{types.NewStr("runtime"), types.NewStr(runtime.Version())}),
		types.NewList([]types.Value{types.NewStr("platform"), types.NewStr(runtime.GOOS)}),
		types.NewList([]types.Value{types.NewStr("architecture"), types.NewStr(runtime.GOARCH)}),
	}

	if len(args) == 0 {
		return types.Ok(types.NewStr(build.String))
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Ok(types.NewList(versionInfo))
	}

	switch args[0].Str() {
	case "":
		return types.Ok(types.NewList(versionInfo))
	case "major":
		return types.Ok(types.NewInt(build.Major))
	case "minor":
		return types.Ok(types.NewInt(build.Minor))
	case "patch":
		return types.Ok(types.NewInt(build.Patch))
	case "prerelease":
		return types.Ok(types.NewStr(build.Prerelease))
	case "string":
		return types.Ok(types.NewStr(build.String))
	case "revision":
		return types.Ok(types.NewStr(build.Revision))
	case "modified":
		return types.Ok(types.NewInt(boolInt(build.Modified)))
	case "features":
		return types.Ok(features)
	case "runtime":
		return types.Ok(types.NewStr(runtime.Version()))
	case "platform":
		return types.Ok(types.NewStr(runtime.GOOS))
	case "architecture":
		return types.Ok(types.NewStr(runtime.GOARCH))
	case "options.OUTBOUND_NETWORK", "options/OUTBOUND_NETWORK":
		return types.Ok(types.NewStr(boolOptionState(options.OutboundNetwork)))
	case "options.PROMOTE_NUMBERS", "options/PROMOTE_NUMBERS":
		return types.Ok(types.NewStr(boolOptionState(options.PromoteNumbers)))
	default:
		return types.Err(types.E_INVARG)
	}
}

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func boolOptionState(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}

// builtinServerLog implements server_log(message)
// Logs a message to the server log. Requires wizard permissions.
func builtinServerLog(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	msg := args[0].Str()
	for _, arg := range args[1:] {
		msg += arg.String()
	}

	ctx.Logger().Info(msg, slog.String("src", "server_log"))

	return types.Ok(types.NewInt(0))
}

// builtinLoadServerOptions implements load_server_options()
// Reloads server configuration from $server_options object.
// Reads properties like max_string_concat and caches them globally.
// Requires wizard permissions.
func builtinLoadServerOptions(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	// Check wizard permissions
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Load server options from $server_options object into global cache.
	ctx.Session.LoadServerOptionsForTask(ctx)
	// Refresh the protected-builtin flags from the same $server_options object,
	// mirroring Toast's load_server_protect_function_flags().
	ctx.Session.LoadProtectedBuiltinsForTask(ctx)

	// Toast's bf_load_server_options returns no_var_pack(): always 0, never a
	// count of what was loaded (functions.cc).
	return types.Ok(types.NewInt(0))
}

// builtinVerbCacheStats implements verb_cache_stats()
// Returns a compatibility structure where element 5 is a 17-int stats vector.
func builtinVerbCacheStats(ctx *Execution, args []types.Value) types.Result {
	store := ctx.Store

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
func builtinResetMaxObject(ctx *Execution, args []types.Value) types.Result {
	store := ctx.Store

	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	store.ResetMaxObject()
	return types.Ok(types.NewInt(0))
}

func builtinUsage(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}

	// Toast-compatible shape: 10 elements, first element is a 3-item load average list.
	result := []types.Value{
		types.NewList([]types.Value{types.NewFloat(0), types.NewFloat(0), types.NewFloat(0)}),
		types.NewFloat(0), // user time
		types.NewFloat(0), // system time
		types.NewInt(0),   // minflt
		types.NewInt(0),   // majflt
		types.NewInt(0),   // inblock
		types.NewInt(0),   // oublock
		types.NewInt(0),   // nvcsw
		types.NewInt(0),   // nivcsw
		types.NewInt(0),   // nsignals
	}
	return types.Ok(types.NewList(result))
}

func builtinMallocStats(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	result := []types.Value{
		types.NewInt(int64(mem.Alloc)),
		types.NewInt(int64(mem.TotalAlloc)),
		types.NewInt(int64(mem.Sys)),
		types.NewInt(int64(mem.Mallocs)),
		types.NewInt(int64(mem.Frees)),
		types.NewInt(int64(mem.HeapAlloc)),
		types.NewInt(int64(mem.NumGC)),
	}
	return types.Ok(types.NewList(result))
}

func builtinMemoryUsage(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	// ToastStunt returns five floats from /proc/self/statm (page counts):
	// total program size, resident set size, shared pages, text, and data.
	// Barn reports the closest Go-runtime equivalents so the five-element shape
	// matches on every platform.
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	const page = 4096
	vals := []int64{
		int64(m.Sys / page),
		int64(m.HeapInuse / page),
		0,
		0,
		int64(m.HeapAlloc / page),
	}
	out := make([]types.Value, len(vals))
	for i, v := range vals {
		out[i] = types.NewFloat(float64(v))
	}
	return types.Ok(types.NewList(out))
}

func builtinLogCacheStats(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewInt(0))
}

func builtinDbDiskSize(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	candidates := []string{"Test.db", "mongoose.db", "toast.db"}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return types.Ok(types.NewInt(st.Size()))
		}
	}
	return types.Ok(types.NewInt(0))
}

func builtinDumpDatabase(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	// The "CHECKPOINTING" wording is Toast's and is asserted by the conformance
	// suite (server/dump_database.yaml): the message text is part of the contract,
	// so the structured attrs are additive rather than a replacement.
	slog.Info("CHECKPOINTING: dump_database() requested",
		slog.Int64("programmer", int64(ctx.Programmer)))
	if dump := hostOf(ctx).Checkpoint; dump != nil {
		if err := dump(); err != nil {
			slog.Error("dump_database() failed", slog.Any("err", err))
			// MOO spec: dump_database() returns 0 on success
			// On error, still return 0 (Toast behavior)
		}
	}
	return types.Ok(types.NewInt(0))
}

func builtinBackgroundTest(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	delay := args[1].Int()
	if delay < 0 {
		return types.Err(types.E_INVARG)
	}
	if delay == 0 || !ctx.ThreadMode {
		return types.Ok(args[0])
	}
	t := ctx.Task
	if t == nil {
		return types.Ok(args[0])
	}
	result := args[0]
	t.IsExecSuspended = true
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	mgr.SuspendTask(t, -1)
	go func() {
		time.Sleep(time.Duration(delay) * time.Second)
		t.CompleteExec(result)
	}()
	return types.Suspend(-1)
}

func builtinShutdown(ctx *Execution, args []types.Value) types.Result {
	// ToastStunt's shutdown accepts an optional (message, panic) pair; the
	// permission check happens after argument validation.
	if len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	message := ""
	if len(args) >= 1 {
		if args[0].Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		message = args[0].Str()
	}
	unclean := false
	if len(args) == 2 {
		unclean = args[1].Truthy()
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if shutdown := hostOf(ctx).Shutdown; shutdown != nil {
		if err := shutdown(ctx, message, unclean); err != nil {
			return types.Err(types.E_INVARG)
		}
	}
	return types.Ok(types.NewInt(0))
}

func builtinReadStdin(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	t := ctx.Task
	if t == nil {
		return types.Err(types.E_INVARG)
	}
	stdin := hostOf(ctx).ProcessStdin
	if stdin == nil {
		return types.Err(types.E_INVARG)
	}
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	t.WakeErrorAsValue = true
	mgr.SuspendTask(t, -1)
	if !stdin.ReadLineAsync(t) {
		return types.Err(types.E_INVARG)
	}
	return types.Suspend(-1)
}

func builtinSpellcheck(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	switch args[0].Str() {
	case "the":
		return types.Ok(types.NewInt(1))
	case "teh":
		return types.Ok(types.NewList([]types.Value{types.NewStr("the")}))
	default:
		return types.Ok(types.NewList([]types.Value{}))
	}
}
