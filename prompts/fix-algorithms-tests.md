# Fix Algorithms Tests (6 failures)

## Objective
Fix conformance test failures for hash binary output format.

## Failing Tests
```
algorithms::string_hash_binary_output_md5
algorithms::binary_hash_binary_output_sha256
algorithms::value_hash_binary_output_sha1
algorithms::string_hmac_binary_output_sha256
algorithms::binary_hmac_binary_output_sha1
algorithms::value_hmac_binary_output_sha256
```

## Context
- Server: Barn Go MOO server at `C:\Users\Q\code\barn`
- Tests: `C:\Users\Q\code\cow_py\tests\conformance\builtins\algorithms.yaml`
- Reference: ToastStunt at `C:\Users\Q\src\toaststunt`
- Relevant files: `builtins/crypto.go`
- Toast oracle: `./toast_oracle.exe 'expression'`

## Key Issue
These tests check hash functions with binary output mode.
The hash functions (string_hash, binary_hash, value_hash, *_hmac) take an optional
binary flag that changes output format from hex string to binary string.

Example: `string_hash("MD5", "test", 1)` should return binary, not hex.

## Workflow
1. Read the failing test definitions in the YAML file
2. Test expected behavior with toast_oracle
3. Check hash functions in `builtins/crypto.go`
4. Compare with ToastStunt's implementation
5. Fix binary output handling
6. Build: `go build -o barn_test.exe ./cmd/barn/`
7. Start server: `./barn_test.exe -db Test.db -port 9300 &`
8. Test: `cd ~/code/cow_py && uv run pytest tests/conformance/ -k "algorithms::" --transport socket --moo-port 9300 -v`
9. Commit each fix individually

## Requirements
- NO stubbing - real implementations only
- Commit after EACH fix
- Write report to `reports/fix-algorithms-tests.md`
