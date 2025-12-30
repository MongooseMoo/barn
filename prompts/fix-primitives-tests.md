# Fix Primitives Tests (4 failures)

## Objective
Fix conformance test failures for primitives (queued_tasks, callers, prototypes).

## Failing Tests
```
primitives::queued_tasks_includes_this_map
primitives::callers_includes_this_list
primitives::inheritance_with_prototypes
primitives::pass_works_with_prototypes
```

## Context
- Server: Barn Go MOO server at `C:\Users\Q\code\barn`
- Tests: `C:\Users\Q\code\cow_py\tests\conformance\builtins\primitives.yaml`
- Reference: ToastStunt at `C:\Users\Q\src\toaststunt`
- Relevant files: `builtins/system.go`, `vm/eval.go`

## Key Issues

### queued_tasks_includes_this_map
queued_tasks() should return a list of maps (not lists) with task info.

### callers_includes_this_list
callers() should return list with proper format for current call stack.

### inheritance_with_prototypes / pass_works_with_prototypes
Prototype objects have special inheritance behavior. pass() should work with prototypes.

## Workflow
1. Read the failing test definitions in the YAML file
2. Test expected behavior with toast_oracle
3. Fix queued_tasks() to return maps
4. Fix callers() format
5. Check prototype inheritance in verb calls
6. Build: `go build -o barn_test.exe ./cmd/barn/`
7. Start server: `./barn_test.exe -db Test.db -port 9308 &`
8. Test: `cd ~/code/cow_py && uv run pytest tests/conformance/ -k "primitives::" --transport socket --moo-port 9308 -v`
9. Commit each fix individually

## Requirements
- NO stubbing - real implementations only
- Commit after EACH fix
- Write report to `reports/fix-primitives-tests.md`
