# Fix Properties Tests (12 failures)

## Objective
Fix conformance test failures in the properties category.

## Failing Tests
```
properties::add_property_invalid_owner
properties::add_property_invalid_perms
properties::add_property_builtin_name
properties::add_property_defined_on_descendant
properties::add_property_not_owner
properties::is_clear_property_works
properties::is_clear_property_builtin
properties::is_clear_property_with_read_permission
properties::is_clear_property_wizard_bypasses_read
properties::clear_property_builtin
properties::clear_property_on_definer
```

## Context
- Server: Barn Go MOO server at `C:\Users\Q\code\barn`
- Tests: `C:\Users\Q\code\cow_py\tests\conformance\builtins\properties.yaml`
- Reference: ToastStunt at `C:\Users\Q\src\toaststunt`
- Toast oracle: `./toast_oracle.exe 'expression'` to test expected behavior

## Workflow
1. Read the failing test definitions in the YAML file
2. Test expected behavior with toast_oracle
3. Find the relevant code in `builtins/properties.go`
4. Compare with ToastStunt implementation
5. Fix the code
6. Build: `go build -o barn_test.exe ./cmd/barn/`
7. Start server: `./barn_test.exe -db Test.db -port 9300 &`
8. Test: `cd ~/code/cow_py && uv run pytest tests/conformance/ -k "properties::" --transport socket --moo-port 9300 -v`
9. Commit each fix individually with descriptive message

## Requirements
- NO stubbing - real implementations only
- Commit after EACH fix
- Write report to `reports/fix-properties-tests.md`
