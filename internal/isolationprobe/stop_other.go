//go:build elle && !linux

// Only Linux can direct SIGSTOP at the calling thread (tgkill); a
// process-directed stop elsewhere lets the stopping thread run on.

package isolationprobe

import "errors"

const canStopSelf = false

func stopSelf() error { return errors.ErrUnsupported }
