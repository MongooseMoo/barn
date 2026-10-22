package types

// ExceptionReason identifies the native guard that produced an exception.
// It is diagnostic metadata, not a MOO error code or exception tuple field.
// Zero means the origin is unknown, including errors raised explicitly by MOO.
type ExceptionReason uint8

const (
	ExceptionReasonUnknown ExceptionReason = iota
	ExceptionReasonFrameDepth
	ExceptionReasonTickBudget
	ExceptionReasonSecondsBudget
)

func (r ExceptionReason) String() string {
	switch r {
	case ExceptionReasonFrameDepth:
		return "frame_depth"
	case ExceptionReasonTickBudget:
		return "tick_budget"
	case ExceptionReasonSecondsBudget:
		return "seconds_budget"
	default:
		return "unknown"
	}
}
