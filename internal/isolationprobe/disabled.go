//go:build !elle

// Package isolationprobe contains instrumentation for the opt-in Elle build.
// Normal server builds compile its hooks away and cannot disable validation.
package isolationprobe

const Enabled = false

func Fault(string) bool           { return false }
func SetCommitExclusionPair(bool) {}
func ActiveFault() string         { return "" }
func SetPause(func(string))       {}
func Pause(string)                {}
func Note(string)                 {}
func MarkFailed(any)              {}
func Failed(any) bool             { return false }
func Reset()                      {}
func Snapshot() map[string]uint64 { return map[string]uint64{} }
