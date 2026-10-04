package trace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/MongooseMoo/barn/types"
)

// Tracer records execution metadata. MOO payloads never cross this boundary.
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
	var patterns []string
	for _, filter := range filters {
		if pattern := strings.TrimSpace(filter); pattern != "" {
			patterns = append(patterns, pattern)
		}
	}
	globalTracer = &Tracer{
		enabled: enabled,
		filters: patterns,
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
func (t *Tracer) VerbCall(objID types.ObjID, verbName string, argCount int, player types.ObjID, caller types.ObjID) {
	if !t.enabled || !t.matchesFilter(verbName) {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	fmt.Fprintf(t.writer, "[TRACE] CALL #%d:%q argc=%d player=#%d caller=#%d\n",
		objID, verbName, argCount, player, caller)
}

// VerbReturn logs the type of a verb result, never its value.
func (t *Tracer) VerbReturn(objID types.ObjID, verbName string, resultType types.TypeCode) {
	if !t.enabled || !t.matchesFilter(verbName) {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	fmt.Fprintf(t.writer, "[TRACE] RETURN #%d:%q type=%s\n",
		objID, verbName, resultType.String())
}

// Exception logs an exception
func (t *Tracer) Exception(objID types.ObjID, verbName string, err types.ErrorCode) {
	if !t.enabled || !t.matchesFilter(verbName) {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	fmt.Fprintf(t.writer, "[TRACE] EXCEPTION #%d:%q %s\n",
		objID, verbName, err.String())
}

// Notify logs a notify() call
func (t *Tracer) Notify(player types.ObjID) {
	if !t.enabled {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	fmt.Fprintf(t.writer, "[TRACE]   NOTIFY #%d\n", player)
}

// Connection logs a connection event
func (t *Tracer) Connection(event string, connID int64, player types.ObjID) {
	if !t.enabled {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	fmt.Fprintf(t.writer, "[TRACE] CONN %s conn=%d player=#%d\n",
		event, connID, player)
}

// Global convenience functions

// VerbCall logs a verb call using the global tracer
func VerbCall(objID types.ObjID, verbName string, argCount int, player types.ObjID, caller types.ObjID) {
	if globalTracer != nil {
		globalTracer.VerbCall(objID, verbName, argCount, player, caller)
	}
}

// VerbReturn logs a verb return using the global tracer
func VerbReturn(objID types.ObjID, verbName string, resultType types.TypeCode) {
	if globalTracer != nil {
		globalTracer.VerbReturn(objID, verbName, resultType)
	}
}

// Exception logs an exception using the global tracer
func Exception(objID types.ObjID, verbName string, err types.ErrorCode) {
	if globalTracer != nil {
		globalTracer.Exception(objID, verbName, err)
	}
}

// Notify logs a notify() call using the global tracer
func Notify(player types.ObjID) {
	if globalTracer != nil {
		globalTracer.Notify(player)
	}
}

// Connection logs a connection event using the global tracer
func Connection(event string, connID int64, player types.ObjID) {
	if globalTracer != nil {
		globalTracer.Connection(event, connID, player)
	}
}
