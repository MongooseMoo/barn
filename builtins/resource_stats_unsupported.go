//go:build !linux

package builtins

import "errors"

var errProcessStatsUnsupported = errors.New("process statistics are unsupported on this platform")

func readProcessUsage() (processUsage, error)   { return processUsage{}, errProcessStatsUnsupported }
func readProcessMemory() (processMemory, error) { return processMemory{}, errProcessStatsUnsupported }
