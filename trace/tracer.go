package trace

import (
	"barn/types"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Tracer provides execution tracing for debugging
type Tracer struct {
	enabled bool
	filters []string
	writer  io.Writer
	mu      sync.Mutex
}

// Global tracer instance
var globalTracer *Tracer

// Init initializes the global tracer
func Init(enabled bool, filters []string, writer io.Writer) {
	if writer == nil {
		writer = os.Stderr
	}
	globalTracer = &Tracer{
		enabled: enabled,
		filters: filters,
		writer:  writer,
	}
}

// IsEnabled returns whether tracing is enabled
func IsEnabled() bool {
	if globalTracer == nil {
		return false
	}
	return globalTracer.enabled
}

// matchesFilter checks if a verb name matches any of the filter patterns
func (t *Tracer) matchesFilter(verbName string) bool {
	if len(t.filters) == 0 {
		return true // No filters = trace everything
	}

	for _, pattern := range t.filters {
		if matched, _ := filepath.Match(pattern, verbName); matched {
			return true
		}
	}
	return false
}

// VerbCall logs a verb call
func (t *Tracer) VerbCall(objID types.ObjID, verbName string, args []types.Value, player types.ObjID, caller types.ObjID) {
	if !t.enabled || !t.matchesFilter(verbName) {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Format args
	argStrs := make([]string, len(args))
	for i, arg := range args {
		argStrs[i] = arg.String()
	}
	argsStr := strings.Join(argStrs, ", ")

	fmt.Fprintf(t.writer, "[TRACE] CALL #%d:%s args=[%s] player=#%d caller=#%d\n",
		objID, verbName, argsStr, player, caller)
}

// VerbReturn logs a verb return value
func (t *Tracer) VerbReturn(objID types.ObjID, verbName string, result types.Value) {
	if !t.enabled || !t.matchesFilter(verbName) {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	resultStr := "0"
	if result != nil {
		resultStr = result.String()
	}

	fmt.Fprintf(t.writer, "[TRACE] RETURN #%d:%s => %s\n",
		objID, verbName, resultStr)
}

// Exception logs an exception
func (t *Tracer) Exception(objID types.ObjID, verbName string, err types.ErrorCode) {
	if !t.enabled || !t.matchesFilter(verbName) {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	errStr := types.NewErr(err).String()
	fmt.Fprintf(t.writer, "[TRACE] EXCEPTION #%d:%s %s\n",
		objID, verbName, errStr)
}

// Notify logs a notify() call
func (t *Tracer) Notify(player types.ObjID, message string) {
	if !t.enabled {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Truncate long messages for readability
	msgDisplay := message
	if len(msgDisplay) > 60 {
		msgDisplay = msgDisplay[:57] + "..."
	}

	fmt.Fprintf(t.writer, "[TRACE]   NOTIFY #%d %q\n", player, msgDisplay)
}

// Connection logs a connection event
func (t *Tracer) Connection(event string, connID int64, player types.ObjID, details string) {
	if !t.enabled {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if details != "" {
		fmt.Fprintf(t.writer, "[TRACE] CONN %s conn=%d player=#%d %s\n",
			event, connID, player, details)
	} else {
		fmt.Fprintf(t.writer, "[TRACE] CONN %s conn=%d player=#%d\n",
			event, connID, player)
	}
}

// Global convenience functions

// VerbCall logs a verb call using the global tracer
func VerbCall(objID types.ObjID, verbName string, args []types.Value, player types.ObjID, caller types.ObjID) {
	if globalTracer != nil {
		globalTracer.VerbCall(objID, verbName, args, player, caller)
	}
}

// VerbReturn logs a verb return using the global tracer
func VerbReturn(objID types.ObjID, verbName string, result types.Value) {
	if globalTracer != nil {
		globalTracer.VerbReturn(objID, verbName, result)
	}
}

// Exception logs an exception using the global tracer
func Exception(objID types.ObjID, verbName string, err types.ErrorCode) {
	if globalTracer != nil {
		globalTracer.Exception(objID, verbName, err)
	}
}

// Notify logs a notify() call using the global tracer
func Notify(player types.ObjID, message string) {
	if globalTracer != nil {
		globalTracer.Notify(player, message)
	}
}

// Connection logs a connection event using the global tracer
func Connection(event string, connID int64, player types.ObjID, details string) {
	if globalTracer != nil {
		globalTracer.Connection(event, connID, player, details)
	}
}
