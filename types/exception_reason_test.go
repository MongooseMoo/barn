package types

import "testing"

func TestExceptionReasonLabelsAndUnknownDefault(t *testing.T) {
	for reason, want := range map[ExceptionReason]string{
		ExceptionReasonUnknown: "unknown", ExceptionReasonFrameDepth: "frame_depth",
		ExceptionReasonTickBudget: "tick_budget", ExceptionReasonSecondsBudget: "seconds_budget",
		ExceptionReason(255): "unknown",
	} {
		if got := reason.String(); got != want {
			t.Errorf("reason %d label = %q, want %q", reason, got, want)
		}
	}
	if got := Err(E_MAXREC).Reason; got != ExceptionReasonUnknown {
		t.Fatalf("ordinary E_MAXREC reason = %s, want unknown", got)
	}
}
