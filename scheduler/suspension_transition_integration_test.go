package scheduler

import (
	"fmt"
	"testing"

	"barn/config"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
)

func TestSchedulerSuspensionTransitionBarriers(t *testing.T) {
	for _, operation := range []string{"resume", "complete_exec", "kill"} {
		for _, timing := range []string{"before", "during", "after"} {
			t.Run(fmt.Sprintf("%s/%s", operation, timing), func(t *testing.T) {
				store := dbstore.NewStore()
				addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
				s := newSchedulerWithWorkerCount(store, config.Options{}, 1)
				t.Cleanup(s.Stop)

				builtinName := "transition_suspend"
				s.registry.Register(builtinName, func(ctx *kernel.TaskContext, _ []types.Value) types.Result {
					tk := ctx.Task.(*task.Task)
					if operation == "complete_exec" {
						if !tk.RequestExecSuspend(-1, nil, "immediate-test") {
							return types.Err(types.E_INVARG)
						}
					} else if !tk.RequestSuspend(-1) {
						return types.Err(types.E_INVARG)
					}
					return types.Suspend(-1)
				})

				tk := task.NewTaskFull(94001, 0, compileTestProgram(t, s.registry, "return "+builtinName+"();"), 1<<50, 1e9)
				tk.Context.IsWizard = true
				want := types.NewStr(operation + "-value")
				invoke := func() bool {
					switch operation {
					case "resume":
						return tk.Resume(want)
					case "complete_exec":
						return tk.CompleteExec(want)
					case "kill":
						tk.Kill()
						return true
					default:
						return false
					}
				}

				var callbackDone chan bool
				s.taskLifecycleObserver = func(stage string, _ *task.Task) {
					if stage == "suspend_before_publish" && timing == "before" {
						if !invoke() {
							t.Errorf("%s callback rejected before publication", operation)
						}
					}
					if stage == "suspend_during_publish" && timing == "during" {
						started := make(chan struct{})
						callbackDone = make(chan bool, 1)
						go func() {
							close(started)
							callbackDone <- invoke()
						}()
						<-started
						select {
						case <-callbackDone:
							t.Error("callback crossed the task-locked publication barrier")
						default:
						}
					}
					if stage == "suspend_after_publish" && timing == "after" {
						if !invoke() {
							t.Errorf("%s callback rejected after publication", operation)
						}
					}
				}

				if err := s.runTask(tk); err != nil {
					t.Fatalf("runTask: %v", err)
				}
				if callbackDone != nil && !<-callbackDone {
					t.Fatalf("%s callback during publication was rejected", operation)
				}
				if operation == "kill" {
					if got := tk.GetState(); got != task.TaskKilled {
						t.Fatalf("state = %s, want killed", got)
					}
					return
				}
				if got := tk.GetState(); got != task.TaskQueued {
					t.Fatalf("state after callback = %s, want queued", got)
				}
				if invoke() {
					t.Fatal("duplicate callback was accepted")
				}
				if err := s.runTask(tk); err != nil {
					t.Fatalf("resume runTask: %v", err)
				}
				if got := tk.GetState(); got != task.TaskCompleted {
					t.Fatalf("resumed state = %s, want completed", got)
				}
				if !tk.Result.Val.Equal(want) {
					t.Fatalf("resumed result = %v, want %v", tk.Result.Val, want)
				}
			})
		}
	}
}
