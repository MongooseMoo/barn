# Fix Misc Tests (9 failures)

## Objective
Fix remaining conformance test failures across multiple categories.

## Failing Tests
```
waif::nested_waif_map_indexes
waif::deeply_nested_waif_map_indexes
index_and_range::range_list_single
index_and_range::decompile_with_index_operators
recycle::recycle_invalid_already_recycled_object
recycle::recycle_invalid_already_recycled_anonymous
anonymous::recycle_invalid_anonymous_no_crash
caller_perms::caller_perms_top_level_eval
math::random_in_valid_range_64bit
```

## Context
- Server: Barn Go MOO server at `C:\Users\Q\code\barn`
- Tests: Various YAML files in `C:\Users\Q\code\cow_py\tests\conformance\builtins\`
- Reference: ToastStunt at `C:\Users\Q\src\toaststunt`
- Toast oracle: `./toast_oracle.exe 'expression'`

## Key Issues by Category

### waif (2)
Nested waif map indexing: `waif.prop["key"]` chains not working.

### index_and_range (2)
- range_list_single: Single element range extraction
- decompile_with_index_operators: Decompiler output format

### recycle (2)
Already recycled objects should return specific error.

### anonymous (1)
Recycling anonymous objects shouldn't crash.

### caller_perms (1)
caller_perms() at top level of eval() should return appropriate value.

### math (1)
random() with 64-bit range should work correctly.

## Workflow
1. Read each failing test definition
2. Test expected behavior with toast_oracle
3. Fix each issue
4. Build: `go build -o barn_test.exe ./cmd/barn/`
5. Start server: `./barn_test.exe -db Test.db -port 9309 &`
6. Test each category
7. Commit each fix individually

## Requirements
- NO stubbing - real implementations only
- Commit after EACH fix
- Write report to `reports/fix-misc-tests.md`
