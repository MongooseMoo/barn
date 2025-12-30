# Fix Objects Tests (8 failures)

## Objective
Fix conformance test failures in the objects category related to create().

## Failing Tests
```
objects::create_invalid_owner_invarg
objects::create_invalid_owner_ambiguous
objects::create_invalid_owner_failed_match
objects::create_invalid_owner_invarg_as_programmer
objects::create_invalid_parent_ambiguous
objects::create_invalid_parent_failed_match
objects::create_list_invalid_ambiguous
objects::create_list_invalid_failed_match
```

## Context
- Server: Barn Go MOO server at `C:\Users\Q\code\barn`
- Tests: `C:\Users\Q\code\cow_py\tests\conformance\builtins\objects.yaml`
- Reference: ToastStunt at `C:\Users\Q\src\toaststunt`
- Relevant files: `builtins/objects.go`
- Toast oracle: `./toast_oracle.exe 'expression'`

## Key Issue
These tests check that create() returns appropriate errors for invalid owner/parent:
- E_INVARG for invalid object references
- Proper error handling for $ambiguous_match, $failed_match

## Workflow
1. Read the failing test definitions in the YAML file
2. Test expected behavior with toast_oracle
3. Find create() in `builtins/objects.go`
4. Check ToastStunt's bf_create in src/functions.cc
5. Fix error handling for invalid owner/parent arguments
6. Build: `go build -o barn_test.exe ./cmd/barn/`
7. Start server: `./barn_test.exe -db Test.db -port 9300 &`
8. Test: `cd ~/code/cow_py && uv run pytest tests/conformance/ -k "objects::create" --transport socket --moo-port 9300 -v`
9. Commit each fix individually

## Requirements
- NO stubbing - real implementations only
- Commit after EACH fix
- Write report to `reports/fix-objects-tests.md`
