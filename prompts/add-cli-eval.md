# Task: Add -eval flag to CLI for evaluating MOO expressions

## Context

The barn CLI needs a way to evaluate MOO expressions directly without starting the server. This is essential for debugging and testing.

## Objective

Add an `-eval` flag that evaluates a MOO expression and prints the result.

Example usage:
```bash
barn.exe -db toastcore.db -eval "verb_args(#10, 3)"
barn.exe -db toastcore.db -eval "1 + 2"
barn.exe -db toastcore.db -eval "verbs(#10)"
barn.exe -db toastcore.db -eval "{1, 2, 3}[2]"
```

## Files to Modify

- `cmd/barn/main.go` - add the `-eval` flag and evaluation logic

## Implementation

1. Add `-eval string` flag
2. When set:
   - Load the database (like other inspection flags do)
   - Create an evaluator
   - Parse and evaluate the expression
   - Print the result (use the value's String() method for MOO literal format)
   - Exit

Look at how the existing `-verb-code` and `-obj-info` flags work for the pattern.

The evaluator setup should be similar to how `server.NewServer` does it, but simpler since we just need to eval one expression.

## Key files to reference

- `vm/eval.go` - the Evaluator
- `db/store.go` - the Store
- `parser/parser.go` - parsing expressions

## Output format

Print the result as a MOO literal:
```
=> {1, 2, 3}
=> #10
=> "hello"
=> E_VERBNF
```

If there's an error, print it:
```
Error: E_TYPE
```

## Output

Write results to `./reports/add-cli-eval.md`

## CRITICAL: File Modified Error Workaround

If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
