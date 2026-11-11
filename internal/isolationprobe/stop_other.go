//go:build elle && !unix

package isolationprobe

import "errors"

const canStopSelf = false

func stopSelf() error { return errors.ErrUnsupported }
