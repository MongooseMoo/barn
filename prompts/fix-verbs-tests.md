# Fix Verbs Tests (10 failures)

## Objective
Fix conformance test failures in the verbs category.

## Failing Tests
```
verbs::add_verb_basic
verbs::add_verb_invalid_owner
verbs::add_verb_invalid_perms
verbs::add_verb_invalid_args
verbs::add_verb_with_write_permission
verbs::add_verb_wizard_bypasses_write
verbs::add_verb_not_owner
verbs::add_verb_is_owner
verbs::add_verb_wizard_sets_owner
verbs::verb_args_basic
```

## Context
- Server: Barn Go MOO server at `C:\Users\Q\code\barn`
- Tests: `C:\Users\Q\code\cow_py\tests\conformance\builtins\verbs.yaml`
- Reference: ToastStunt at `C:\Users\Q\src\toaststunt`
- Toast oracle: `./toast_oracle.exe 'expression'` to test expected behavior

## Workflow
1. Read the failing test definitions in the YAML file
2. Test expected behavior with toast_oracle
3. Find the relevant code in `builtins/verbs.go`
4. Compare with ToastStunt implementation (src/functions.cc, search for bf_add_verb)
5. Fix the code
6. Build: `go build -o barn_test.exe ./cmd/barn/`
7. Start server: `./barn_test.exe -db Test.db -port 9300 &`
8. Test: `cd ~/code/cow_py && uv run pytest tests/conformance/ -k "verbs::" --transport socket --moo-port 9300 -v`
9. Commit each fix individually with descriptive message

## Requirements
- NO stubbing - real implementations only
- Commit after EACH fix
- Write report to `reports/fix-verbs-tests.md`
