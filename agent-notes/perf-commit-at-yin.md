# perf/commit-at-yin

## Goal
Q-approved plan: when YIN_COMMIT_TICKS (experiment knob, removed before merge)
is K > 0, a yin() on the task's root VM (ctx.CanSuspend) after at least K ticks
in the current slice yields for 0 seconds, so long yin()-calling tasks
(#1:forbids, #2700:_handlers) commit there instead of staying one huge
optimistic transaction. Toast's yin() can always yield, so yielding more often
is Toast-legal. The low-budget branch of builtinYin is unchanged.

## State
Implemented and committed on perf/commit-at-yin (not pushed, no PR):
- config: Options.YinCommitTicks, YIN_COMMIT_TICKS (integer >= 0, Validate
  rejects negatives), TestYinCommitTicksConfiguration.
- builtins/tasks.go builtinYin: no-yield branch yields 0 seconds when
  K > 0 && ctx.CanSuspend && t.TicksLimit-ctx.TicksRemaining >= K.
- engine/yin_commit_test.go: six tests (yield above K, none below K, none
  with K=0, none in a nested VM, pre-yin write published while queued,
  conflict after the yin re-runs only from the yin).
- engine/mongoose_real_bench_test.go: BARN_MONGOOSE_YIN_COMMIT_TICKS sets K;
  printed in the GOMAXPROCS/warmup log line.
Mutations of each guard (k > 0, CanSuspend, >= k, slice-ticks measure,
config validation, parser assignment) were each caught by a test.

## Decisions and why
- Ticks in slice = t.TicksLimit - ctx.TicksRemaining. The root VM's TickLimit
  is always set from t.TicksLimit (engine/task_runtime.go launchSlice) and
  vm.Ticks is 0 at a slice's start (reset at resume, task_runtime.go ~350);
  ctx.TicksRemaining = vm.TickLimit - vm.Ticks (vm/vm.go syncContextTicks),
  refreshed before every generic-path instruction, which includes builtin calls.
- Nested-VM test uses a test builtin calling ctx.Session.CallVerb, the real
  nested-VM path; create()'s :initialize now runs on the calling VM
  (vm.pushInitialize), so it no longer exercises a nested VM.

## Surprises
- The brief's fixtures newReadCheckpointFixture/directWrite/read_checkpoint_test.go
  do not exist; used bumpReadValueLiveOnce and testBuiltinSlot instead.
- Without the CanSuspend guard, a suspend from the nested VM does not just get
  dropped: the outer task ended its slice queued.

## Next step
Gate run of TestMongooseRealWorkload with BARN_MONGOOSE_YIN_COMMIT_TICKS
values (team lead's decision); remove the knob before merge.
