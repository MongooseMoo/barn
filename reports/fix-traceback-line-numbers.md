# Fix Traceback Line Numbers to Skip Comment-Only Lines

## Problem Statement

Barn's traceback shows incorrect line numbers. When an error occurs in a verb, the traceback reports "line 1" even when line 1 is just a documentation string. For example:

```moo
"Usage: news [contents] [articles]";   // line 1 - doc string
"";                                      // line 2 - doc string
"Common uses:";                          // line 3 - doc string
...
set_task_perms(caller_perms());          // actual code starts here
```

When an error occurs, the traceback shows `#6:news line 1` but line 1 is just a doc string, not executable code.

## Root Cause Analysis

### Current Implementation

The line number tracking issue has two components:

1. **Line numbers are hardcoded when frames are created**
   - `vm/verbs.go:225` - `LineNumber: 1` is hardcoded when pushing activation frames
   - `vm/verbs.go:126` - `LineNumber: 0` with TODO comment: "Track line numbers during execution"
   - `vm/builtin_pass.go:126` - `LineNumber: 0` hardcoded

2. **Line numbers are never updated during execution**
   - The VM evaluates statements in `vm/eval_stmt.go:EvalStmt()`
   - There is no mechanism to update the top frame's LineNumber as execution progresses
   - When errors occur, the traceback shows whatever line number was set when the frame was created (usually 1)

### Key Code Locations

**Frame Creation (hardcoded line numbers):**
- `vm/verbs.go:118-127` - `EvalVerb()` pushes frame with LineNumber: 0
- `vm/verbs.go:217-226` - `CallVerb()` pushes frame with LineNumber: 1
- `vm/builtin_pass.go:118-127` - `pass()` builtin pushes frame with LineNumber: 0

**Statement Evaluation (no line tracking):**
- `vm/eval_stmt.go:32-66` - `EvalStmt()` dispatches statements but doesn't update line numbers
- `vm/eval_stmt.go:13-29` - `EvalStatements()` iterates through statements without tracking position

**Traceback Generation:**
- `task/traceback.go:14-61` - `FormatTraceback()` reads LineNumber from frames as-is
- `task/task.go:239-246` - `GetTopFrame()` returns the current frame for reading

## How Reference Implementations Handle This

### ToastStunt (Bytecode VM)

ToastStunt uses a bytecode VM with a program counter (PC):

1. Each bytecode instruction has an associated source line number
2. The VM maintains a PC that points to the current instruction
3. `find_line_number(prog, vector, pc)` in `src/decompile.cc` maps PC back to source lines
4. When building the call stack, it uses the current PC to determine line numbers

```c
// From ToastStunt execute.cc line 180
find_line_number(activ_stack[t].prog, ...)
```

### Barn's AST Interpreter Advantage

Unlike bytecode VMs, Barn is an AST interpreter where every statement node already has Position information:

- All statement types implement `Position() Position` (defined in `parser/ast.go:6-8`)
- Position includes `Line int` (from `parser/token.go:98`)
- Examples: `ExprStmt`, `IfStmt`, `WhileStmt`, `ForStmt`, etc. all have `Pos Position` fields

This means we already have line numbers available at evaluation time - we just need to use them.

## Proposed Solution

### Approach: Update Line Numbers During Execution

Add a mechanism to update the top frame's line number as statements are evaluated:

1. **Add UpdateLineNumber method to Task** (`task/task.go`)
   ```go
   // UpdateLineNumber updates the line number of the top frame
   func (t *Task) UpdateLineNumber(line int) {
       t.mu.Lock()
       defer t.mu.Unlock()
       if len(t.CallStack) > 0 {
           t.CallStack[len(t.CallStack)-1].LineNumber = line
       }
   }
   ```

2. **Update line number at start of each statement** (`vm/eval_stmt.go`)
   ```go
   func (e *Evaluator) EvalStmt(stmt parser.Stmt, ctx *types.TaskContext) types.Result {
       // Update line number for traceback
       if ctx.Task != nil {
           if t, ok := ctx.Task.(*task.Task); ok {
               pos := stmt.Position()
               t.UpdateLineNumber(pos.Line)
           }
       }

       // Tick counting
       if !ctx.ConsumeTick() {
           return types.Err(types.E_MAXREC)
       }

       // ... rest of function unchanged
   }
   ```

3. **Initialize frames with line 1** (already done in `vm/verbs.go:225`)
   - When a verb is first called, the frame starts at line 1
   - As statements execute, the line number gets updated to reflect actual position

### Why This Works

1. **Every statement has line information**: All statement nodes implement `Position()` which returns the line number
2. **Natural update points**: We already iterate through statements in `EvalStmt()`, so we can update line numbers there
3. **Correct for control flow**: If/while/for statements will show the correct line for the statement being executed
4. **Handles nested calls**: Each frame tracks its own line number independently

### Edge Cases Handled

1. **Doc strings as statements**: Lines like `"usage info";` are `ExprStmt` nodes with their own line numbers, so they will be reflected if an error occurs during their evaluation
2. **Nested expressions**: Only statements update line numbers; errors in expressions show the line of the containing statement
3. **Builtin calls**: When calling builtins, the line number reflects where the builtin was called from
4. **Verb calls**: When calling another verb, a new frame is pushed with its own line tracking

## Alternative Approaches Considered

### 1. Skip Doc-String-Only Lines (Rejected)

Initially, the problem was framed as "skip lines that are just string literals". This approach has problems:

- **Arbitrary filtering**: Who decides what's a "doc string" vs. legitimate code?
- **False positives**: `x = "config"; y = 1;` - is this a doc string or code?
- **Doesn't solve root cause**: The real issue is that line numbers aren't tracked during execution
- **Toast doesn't do this**: Reference implementations show actual line numbers, not filtered ones

### 2. Start Counting from First Non-String (Rejected)

Adjust line numbers to skip leading doc strings:

- **Breaks semantics**: Line 10 in the verb wouldn't match line 10 in traceback
- **Confusing for developers**: Editor shows line X, traceback shows line Y
- **Fragile**: What if code is added/removed?

### 3. Track Line Numbers Only on Errors (Rejected)

Walk the AST backward when an error occurs to find the line:

- **Performance**: Requires AST traversal on every error
- **Complexity**: Doesn't help with `callers()` builtin which needs line numbers for normal calls
- **Incomplete**: Doesn't solve the fundamental tracking problem

## Implementation Checklist

- [ ] Add `UpdateLineNumber(line int)` method to `task/task.go`
- [ ] Update `vm/eval_stmt.go:EvalStmt()` to call `UpdateLineNumber()` at start of each statement
- [ ] Remove hardcoded `LineNumber: 1` from `vm/verbs.go:225` (or document that it's the initial value)
- [ ] Update the TODO comment in `vm/verbs.go:126` to reference this solution
- [ ] Test with verbs that have leading doc strings to verify correct line numbers
- [ ] Test with nested verb calls to ensure each frame tracks independently
- [ ] Test that `callers()` builtin returns correct line numbers

## Test Cases

### 1. Error in Verb with Doc Strings

```moo
"Usage: test";        // line 1
"";                   // line 2
x = 1;                // line 3
y = 1 / 0;            // line 4 - error should show this line
```

Expected traceback: `#N:test (this == #N), line 4: Division by zero`

### 2. Error in Nested Call

```moo
// #1:foo
"Outer verb";         // line 1
#2:bar();             // line 2
```

```moo
// #2:bar
"Inner verb";         // line 1
x = 1 / 0;            // line 2 - error here
```

Expected traceback:
```
#P <- #2:bar (this == #2), line 2:  Division by zero
#P <- ... called from #1:foo (this == #1), line 2
#P <- (End of traceback)
```

### 3. Using callers() Builtin

```moo
"Doc string";         // line 1
stack = callers();    // line 2
return stack[1][6];   // Should return 2, not 1
```

## Additional Notes

### Why "Line 1" Happens

The current code in `vm/verbs.go:225` explicitly sets `LineNumber: 1` when a verb is called. This was likely intended as a reasonable default, but it never gets updated as execution proceeds through the verb. The TODO comment in line 126 confirms this was a known limitation.

### Thread Safety

The `UpdateLineNumber()` method uses the existing mutex pattern (`t.mu.Lock()`) that's already used for `PushFrame()` and `PopFrame()`. This ensures thread-safe updates to the call stack.

### Performance Impact

Minimal - we're adding one method call per statement evaluation, which is negligible compared to the actual statement execution cost. The `Position()` method is just a field access, not a computation.

## Conclusion

The fix is straightforward:
1. AST nodes already have line numbers via `Position()`
2. Add a method to update the top frame's line number
3. Call it at the start of each statement evaluation

This gives accurate tracebacks that match the source code, without any special handling for doc strings or other heuristics.
