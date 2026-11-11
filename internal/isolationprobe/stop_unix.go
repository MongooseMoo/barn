//go:build elle && unix

package isolationprobe

import "syscall"

const canStopSelf = true

func stopSelf() error { return syscall.Kill(syscall.Getpid(), syscall.SIGSTOP) }
