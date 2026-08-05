package task

import (
	"fmt"
	"testing"

	"barn/types"
)

func TestSuspensionTransitionLinearizesCallbacks(t *testing.T) {
	for _, operation := range []string{"resume", "complete_exec", "kill"} {
		for _, timing := range []string{"before", "during", "after"} {
			name := fmt.Sprintf("%s/%s", operation, timing)
			t.Run(name, func(t *testing.T) {
				tk := NewTask(71001, 2, 1000, 10)
				tk.SetState(TaskRunning)
				if operation == "complete_exec" {
					if !tk.RequestExecSuspend(-1, nil, "test-exec") {
						t.Fatal("RequestExecSuspend rejected running task")
					}
				} else if !tk.RequestSuspend(-1) {
					t.Fatal("RequestSuspend rejected running task")
				}

				wantValue := types.NewStr(operation + "-value")
				invoke := func() bool {
					switch operation {
					case "resume":
						return tk.Resume(wantValue)
					case "complete_exec":
						return tk.CompleteExec(wantValue)
					case "kill":
						tk.Kill()
						return true
					default:
						t.Fatalf("unknown operation %q", operation)
						return false
					}
				}

				if timing == "before" && !invoke() {
					t.Fatalf("%s callback before publication was rejected", operation)
				}
				callbackDone := make(chan bool, 1)
				during := func() {}
				if timing == "during" {
					started := make(chan struct{})
					during = func() {
						go func() {
							close(started)
							callbackDone <- invoke()
						}()
						<-started
						select {
						case <-callbackDone:
							t.Fatal("callback completed while suspension publication held the task lock")
						default:
						}
					}
				}

				machine := &struct{ marker string }{marker: "published"}
				tk.PublishRequestedSuspension(machine, during, nil)
				if timing == "during" && !<-callbackDone {
					t.Fatalf("%s callback during publication was rejected", operation)
				}
				if timing == "after" && !invoke() {
					t.Fatalf("%s callback after publication was rejected", operation)
				}

				if operation == "kill" {
					if got := tk.GetState(); got != TaskKilled {
						t.Fatalf("state after Kill = %s, want killed", got)
					}
					return
				}
				if got := tk.GetState(); got != TaskQueued {
					t.Fatalf("state after %s = %s, want queued", operation, got)
				}
				if !tk.WakeValue.Equal(wantValue) {
					t.Fatalf("wake value after %s = %v, want %v", operation, tk.WakeValue, wantValue)
				}
				if tk.BytecodeVMValue() != machine {
					t.Fatalf("saved VM after %s = %T, want published machine", operation, tk.BytecodeVMValue())
				}
				if invoke() {
					t.Fatalf("duplicate %s callback was accepted", operation)
				}
			})
		}
	}
}

func TestZeroDelaySuspensionQueuesOnce(t *testing.T) {
	tk := NewTask(71004, 2, 1000, 10)
	tk.SetState(TaskRunning)
	if !tk.RequestSuspend(0) {
		t.Fatal("RequestSuspend rejected running task")
	}
	machine := &struct{}{}
	tk.PublishRequestedSuspension(machine, nil, nil)
	if got := tk.GetState(); got != TaskQueued {
		t.Fatalf("zero-delay suspension state = %s, want queued", got)
	}
	if !tk.WakeValue.Equal(types.NewInt(0)) {
		t.Fatalf("zero-delay wake value = %v, want 0", tk.WakeValue)
	}
	if tk.Resume(types.NewInt(1)) {
		t.Fatal("callback after completed zero-delay suspension was accepted")
	}
}

func TestSuspensionCallbacksRejectOrdinaryRunningTask(t *testing.T) {
	tk := NewTask(71002, 2, 1000, 10)
	tk.SetState(TaskRunning)
	if tk.Resume(types.NewInt(1)) {
		t.Fatal("Resume accepted ordinary running task")
	}
	if tk.CompleteExec(types.NewInt(2)) {
		t.Fatal("CompleteExec accepted ordinary running task")
	}
}

func TestKillBeforeSuspendRequestCannotBeOverwritten(t *testing.T) {
	tk := NewTask(71003, 2, 1000, 10)
	tk.SetState(TaskRunning)
	tk.Kill()
	if tk.RequestSuspend(-1) {
		t.Fatal("RequestSuspend accepted killed task")
	}
	tk.PublishRequestedSuspension(&struct{}{}, nil, nil)
	if got := tk.GetState(); got != TaskKilled {
		t.Fatalf("state = %s, want killed", got)
	}
	if got := tk.BytecodeVMValue(); got != nil {
		t.Fatalf("killed task saved VM = %T, want nil", got)
	}
}
