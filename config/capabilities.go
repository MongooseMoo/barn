package config

import (
	"fmt"
	"strings"
)

// Capabilities selects compiled-in builtin families, independently of runtime
// permission options. Zero enables no builtins.
type Capabilities uint64

const (
	Core Capabilities = 1 << iota
	BarnExtensions
	BackgroundTasks
	AllocatorStats
	ProcessStdin
	Network
	SQLite
)

// DefaultCapabilities preserves Barn's callable builtin surface. Optional
// Barn implementations are not claims about a stock Toast build.
func DefaultCapabilities() Capabilities {
	return Core | BarnExtensions | BackgroundTasks | AllocatorStats | ProcessStdin | Network | SQLite
}

func ParseCapabilities(value string) (Capabilities, error) {
	if value == "none" {
		return 0, nil
	}
	var result Capabilities
	for _, name := range strings.Split(value, ",") {
		var capability Capabilities
		switch strings.TrimSpace(name) {
		case "core":
			capability = Core
		case "barn-extensions":
			capability = BarnExtensions
		case "background-tasks":
			capability = BackgroundTasks
		case "allocator-stats":
			capability = AllocatorStats
		case "process-stdin":
			capability = ProcessStdin
		case "network":
			capability = Network
		case "sqlite":
			capability = SQLite
		default:
			return 0, fmt.Errorf("unknown builtin capability %q", name)
		}
		if result&capability != 0 {
			return 0, fmt.Errorf("duplicate builtin capability %q", name)
		}
		result |= capability
	}
	return result, nil
}
