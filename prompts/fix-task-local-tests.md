# Fix Task Local Tests (8 failures)

## Objective
Fix conformance test failures in the task_local category.

## Failing Tests
```
task_local::fork_and_suspend_case
task_local::command_verb
task_local::server_verb
task_local::across_verb_calls
task_local::across_verb_calls_with_intermediate
task_local::suspend_between_verb_calls
task_local::read_between_verb_calls
task_local::nonfunctional_kill_task
```

## Context
- Server: Barn Go MOO server at `C:\Users\Q\code\barn`
- Tests: `C:\Users\Q\code\cow_py\tests\conformance\builtins\task_local.yaml`
- Reference: ToastStunt at `C:\Users\Q\src\toaststunt`
- Relevant files: `builtins/system.go`, `server/scheduler.go`, `vm/eval.go`

## Key Concept
task_local() and set_task_local() provide per-task storage that persists:
- Across verb calls within the same task
- Through suspend/resume cycles
- Through fork operations (forked task gets copy)

## Workflow
1. Read the failing test definitions in the YAML file
2. Understand what task_local persistence requires
3. Check how TaskContext tracks task_local data
4. Ensure task_local survives verb calls, suspend, resume
5. Fix the code
6. Build: `go build -o barn_test.exe ./cmd/barn/`
7. Start server: `./barn_test.exe -db Test.db -port 9300 &`
8. Test: `cd ~/code/cow_py && uv run pytest tests/conformance/ -k "task_local::" --transport socket --moo-port 9300 -v`
9. Commit each fix individually

## Requirements
- NO stubbing - real implementations only
- Commit after EACH fix
- Write report to `reports/fix-task-local-tests.md`
