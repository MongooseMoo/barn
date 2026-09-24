package task

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func TestInputReceiptFencesOnlyYieldingSource(t *testing.T) {
	m := NewManager()
	source := NewTask(1, 0, 100, 1)
	m.RegisterTask(source)
	complete := source.NewInputReceipt()
	source.SetState(TaskQueued)
	source.PrepareYieldRequeue(1, time.Now())
	if !source.ReadyDeadline(time.Now()).IsZero() || source.ReserveAdmission() {
		t.Fatal("yield continuation overtook its injected input")
	}
	other := NewTask(2, 0, 100, 1)
	other.SetState(TaskQueued)
	if !other.ReserveAdmission() {
		t.Fatal("input receipt blocked unrelated work")
	}
	<-m.ScheduleChanged()
	complete()
	complete() // cancellation and normal completion may race
	select {
	case <-m.ScheduleChanged():
	default:
		t.Fatal("last input completion lost its scheduler wake")
	}
	if source.ReadyDeadline(time.Now()).IsZero() || !source.ReserveAdmission() {
		t.Fatal("input completion did not release continuation")
	}
}

func TestInputReceiptDoesNotBlockItsOwnReadDelivery(t *testing.T) {
	source := NewTask(1, 0, 100, 1)
	complete := source.NewInputReceipt()
	defer complete()
	source.SuspendIndefinite()
	source.SetReadingPlayer(0)
	if !source.ResumeAndClaim(types.NewStr("injected")) {
		t.Fatal("read delivery waited for the input event delivering it")
	}
}
