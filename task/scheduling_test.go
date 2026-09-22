package task

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func TestScheduleChangesCoalesceAndCoverExternalResumes(t *testing.T) {
	m := NewManager()
	tk := NewTask(1, 0, 100, 1)
	m.RegisterTask(tk)
	<-m.ScheduleChanged()
	for _, resume := range []func() bool{
		func() bool { return tk.Resume(types.NewInt(1)) },
		func() bool { return tk.ResumeGeneration(tk.SuspendGeneration(), types.NewInt(2)) },
		func() bool { return tk.CompleteExec(types.NewInt(3)) },
	} {
		tk.SuspendIndefinite()
		<-m.ScheduleChanged()
		if !resume() {
			t.Fatal("resume failed")
		}
		select {
		case <-m.ScheduleChanged():
		default:
			t.Fatal("resume lost its wakeup")
		}
	}
	for range 1000 {
		m.NotifyScheduleChange()
	}
	<-m.ScheduleChanged()
	select {
	case <-m.ScheduleChanged():
		t.Fatal("notifications did not coalesce")
	default:
	}
}

func TestReadyDeadlineTracksTimedAndIndefiniteSuspension(t *testing.T) {
	tk := NewTask(1, 0, 100, 1)
	now := time.Now()
	tk.Suspend(time.Hour)
	_, wake, _ := tk.SchedulingSnapshot()
	if got := tk.ReadyDeadline(now); !got.Equal(wake) {
		t.Fatalf("deadline = %v, want %v", got, wake)
	}
	tk.SuspendIndefinite()
	if got := tk.ReadyDeadline(now); !got.IsZero() {
		t.Fatalf("indefinite task has deadline %v", got)
	}
	tk.CompleteExec(types.NewInt(0))
	if got := tk.ReadyDeadline(time.Now()); got.IsZero() || got.After(time.Now()) {
		t.Fatalf("completed helper is not due: %v", got)
	}
	tk.Kill()
	if got := tk.ReadyDeadline(now); !got.IsZero() {
		t.Fatalf("killed task has deadline %v", got)
	}
}
