# Fix Limits Tests (8 failures)

## Objective
Fix conformance test failures for max_value_bytes limit checking.

## Failing Tests
```
limits::setadd_checks_list_max_value_bytes_exceeds
limits::listinsert_checks_list_max_value_bytes
limits::listappend_checks_list_max_value_bytes
limits::listset_fails_if_value_too_large
limits::decode_binary_checks_list_max_value_bytes
limits::list_literal_checks_max_value_bytes
limits::map_literal_checks_max_value_bytes
limits::encode_binary_limit
```

## Context
- Server: Barn Go MOO server at `C:\Users\Q\code\barn`
- Tests: `C:\Users\Q\code\cow_py\tests\conformance\builtins\limits.yaml`
- Reference: ToastStunt at `C:\Users\Q\src\toaststunt`
- Relevant files: `builtins/lists.go`, `builtins/encoding.go`, `vm/eval.go`

## Key Concept
MOO has a `max_value_bytes` server option that limits how large values can get.
When operations would exceed this limit, they should return E_QUOTA.

Operations that need checking:
- setadd(), listinsert(), listappend(), listset()
- decode_binary()
- List and map literal construction in VM

## Workflow
1. Read the failing test definitions in the YAML file
2. Check how limits are loaded (load_server_options)
3. Find where max_value_bytes is checked in ToastStunt
4. Add E_QUOTA checks to list/map operations
5. Build: `go build -o barn_test.exe ./cmd/barn/`
6. Start server: `./barn_test.exe -db Test.db -port 9300 &`
7. Test: `cd ~/code/cow_py && uv run pytest tests/conformance/ -k "limits::" --transport socket --moo-port 9300 -v`
8. Commit each fix individually

## Requirements
- NO stubbing - real implementations only
- Commit after EACH fix
- Write report to `reports/fix-limits-tests.md`
