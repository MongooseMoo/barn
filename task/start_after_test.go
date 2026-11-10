package task

import (
	"testing"
	"time"
)

// A MOO delay longer than a time.Duration holds (about 292 years) must still
// land that many seconds in the future, as Toast's double_to_start_tv adds it
// to now; converting it through time.Duration overflowed into the past.
func TestStartAfterBeyondDurationRange(t *testing.T) {
	now := time.Unix(1_800_000_000, 250_000_000)
	cases := []struct {
		seconds float64
		want    time.Time
	}{
		{1.5, now.Add(1500 * time.Millisecond)},
		{1e10, time.Unix(1_800_000_000+10_000_000_000, 250_000_000)},
		{1e10 + 0.5, time.Unix(1_800_000_000+10_000_000_000, 750_000_000)},
		{363410236465734, time.Unix(1_800_000_000+363410236465734, 250_000_000)},
		{1e300, IndefiniteSuspendStartTime},
	}
	for _, c := range cases {
		if got := StartAfter(now, c.seconds); !got.Equal(c.want) {
			t.Errorf("StartAfter(now, %v) = %v, want %v", c.seconds, got, c.want)
		}
	}
}

func TestSuspendSecondsBeyondDurationRangeWakesInFarFuture(t *testing.T) {
	tk := NewState(1, 2, 100, 5)
	tk.SetState(TaskRunning)
	before := time.Now()
	tk.SuspendSeconds(1e10)
	if got := tk.WakeTime.Unix() - before.Unix(); got < 10_000_000_000 {
		t.Fatalf("suspend(1e10) wakes %d seconds after now, want at least 10000000000", got)
	}
}
