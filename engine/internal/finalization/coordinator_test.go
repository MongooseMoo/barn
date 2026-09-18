package finalization

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestNewCoordinatorOwnsIndependentState(t *testing.T) {
	first := NewCoordinator()
	second := NewCoordinator()
	first.ExecutingTasks[7] = 1
	first.PendingShutdownRoots = append(first.PendingShutdownRoots, structValue())
	if len(second.ExecutingTasks) != 0 || len(second.PendingShutdownRoots) != 0 {
		t.Fatal("coordinators share execution or finalization state")
	}
	if first.ShutdownReady == nil || second.ShutdownReady == nil || first.ShutdownReady == second.ShutdownReady {
		t.Fatal("shutdown readiness channels are not independently owned")
	}
}

func structValue() types.Value { return types.NewInt(1) }
