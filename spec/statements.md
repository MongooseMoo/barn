# MOO Statements Specification

## Overview

MOO has 8 statement types for control flow, plus expression statements and empty statements.

---

## 1. Statement Types

| Statement | Purpose |
|-----------|---------|
| Expression | Execute expression for side effects |
| If/ElseIf/Else | Conditional branching |
| For | Iteration over collections/ranges |
| While | Condition-based looping |
| Fork | Asynchronous task creation |
| Try/Except | Exception handling |
| Try/Finally | Guaranteed cleanup |
| Return/Break/Continue | Flow control |

---

## 2. Expression Statements

```moo
expression;
;
```

**Semantics:**
- Evaluate expression, discard result
- Empty statement (`;` alone) is valid

**Examples:**
```moo
x = 5;              // Assignment
obj:method();       // Verb call (result discarded)
notify(player, "hi"); // Builtin call
;                   // No-op
```

---

## 3. Conditional Statements (if/elseif/else)

### 3.1 Syntax

```moo
if (condition)
  statements
[elseif (condition)
  statements]*
[else
  statements]
endif
```

### 3.2 Semantics

1. Evaluate first `if` condition
2. If truthy, execute its body and skip to `endif`
3. Otherwise, evaluate each `elseif` condition in order
4. If an `elseif` is truthy, execute its body and skip to `endif`
5. If no conditions matched and `else` present, execute `else` body

### 3.3 Examples

```moo
if (x > 0)
  return "positive";
elseif (x < 0)
  return "negative";
else
  return "zero";
endif
```

### 3.4 Truthiness

See [types.md](types.md) for truthiness rules:
- Falsy: `0`, `0.0`, `""`, `{}`, `[]`, `false`
- Truthy: Everything else

---

## 4. For Loops

### 4.1 List Iteration

```moo
for variable in (list_expression)
  statements
endfor
```

**Semantics:**
- Evaluate `list_expression` once (must be LIST)
- **A snapshot is taken** - the list is captured before iteration begins
- For each element, bind to `variable` and execute body
- 1-based indexing for index form
- **Mutation isolation:** Subsequent mutations to the list variable do not affect the iteration

**Post-loop variable state:**
- After the loop completes (normally or via `break`), the loop variable **retains the value of the last element iterated**
- If the list was empty, the variable remains unchanged from its pre-loop value

**Loop variable scoping:**
- Loop variables do not create a new scope
- Using the same variable name in nested loops causes the inner loop to overwrite the outer loop's variable
- The outer loop's iteration state becomes inaccessible within the inner loop

**Type checking:**
- The list expression is evaluated and type-checked before the first iteration
- If the expression is not a LIST, E_TYPE is raised before the loop body executes

**Empty bodies:**
- For loop bodies may be empty (contain zero statements)
- An empty body is a no-op, but the loop variable is still bound for each iteration

**Scattering assignment:**
- For loop variables must be simple identifiers
- Scattering assignment syntax is not supported in for loop variable bindings

**Example:**
```moo
for item in ({1, 2, 3})
  notify(player, tostr(item));
endfor
// After loop: item == 3

// Mutation during iteration
list = {1, 2, 3};
for x in (list)
  list = {};  // This does NOT affect iteration
endfor
// Loop still iterates over {1, 2, 3}

// Nested loops with same variable
for i in ({1, 2})
  for i in ({10, 20})  // Inner i overwrites outer i
    // Outer i is inaccessible here
  endfor
  // i now has value from inner loop (20), breaking outer iteration
endfor
```

### 4.2 List Iteration with Index

```moo
for value, index in (list_expression)
  statements
endfor
```

**Semantics:**
- `value` receives element
- `index` receives 1-based position
- Same snapshot and mutation isolation as single-variable form

**Post-loop variable state:**
- After the loop completes, the `index` variable **retains the value of the last index iterated** (equal to `length(list)`)
- The `value` variable retains the last element value
- If the list was empty, both variables remain unchanged

**Example:**
```moo
for name, i in ({"Alice", "Bob"})
  notify(player, tostr(i) + ": " + name);
endfor
// Output: "1: Alice", "2: Bob"
// After loop: name == "Bob", i == 2
```

### 4.3 Map Iteration

```moo
for value in (map_expression)
  statements
endfor

for value, key in (map_expression)
  statements
endfor
```

**Semantics:**
- Iterates over map entries
- **Single variable:** receives the *value* only (not keys, not [key, value] pairs)
- **Two variables:** first receives value, second receives key (value, key order)
- Same snapshot and mutation isolation as list iteration
- **Map mutation:** The map expression is evaluated once before iteration begins. Modifications to the map during iteration (adding, removing, or changing entries) do not affect the current iteration sequence.
- **Iteration order:** Implementation-defined but stable within a single iteration of an unmodified map

**Post-loop variable state:**
- Loop variables retain values from last iteration
- If map was empty, variables remain unchanged

**Example:**
```moo
ages = ["Alice" -> 30, "Bob" -> 25];

// Two-variable form: (value, key) order
for age, name in (ages)
  notify(player, name + " is " + tostr(age));
endfor
// After loop: age == 30 (or 25), name == "Alice" (or "Bob")

// Single-variable form: receives values only
for age in (ages)
  notify(player, tostr(age));
endfor
// Prints: 30, 25 (keys not accessible)

// Empty map
for v in ([])
  // Never executes
endfor
// If v was undefined before, it remains undefined
```

### 4.4 Range Iteration

```moo
for variable in [start..end]
  statements
endfor
```

**Semantics:**
- `start` and `end` must be INT
- Iterates from `start` to `end` inclusive, incrementing by 1
- If `start > end`, body never executes and loop variable remains unchanged
- Negative values are allowed for both start and end (e.g., `[-5..-1]` iterates -5, -4, -3, -2, -1)
- Ranges iterate upward only (no countdown/decrement support)

**Type checking:**
- Range expressions are evaluated and type-checked before the first iteration
- If either start or end is not INT, E_TYPE is raised before the loop body executes

**Special markers:**
- `$` as end: iterate to list length (requires list indexing context like `mylist[1..$]`)
- Using `$` in a for-range without list context (e.g., `for i in [1..$]`) is a syntax error

**Example:**
```moo
for i in [1..5]
  notify(player, tostr(i));
endfor
// Output: 1, 2, 3, 4, 5

// Negative range
for i in [-3..0]
  notify(player, tostr(i));
endfor
// Output: -3, -2, -1, 0

// Empty range (start > end)
for i in [5..1]
  // Never executes
endfor
// i remains unchanged from pre-loop value
```

### 4.5 Named For Loops

```moo
for name variable in (expression)
  // Can use: break name; continue name;
endfor
```

**Semantics:**
- `name` is an identifier labeling the loop
- `break name` exits that specific loop
- `continue name` continues that specific loop

---

## 5. While Loops

### 5.1 Basic While

```moo
while (condition)
  statements
endwhile
```

**Semantics:**
1. Evaluate `condition` (must be a valid expression)
2. If falsy, exit loop
3. Execute body
4. Go to step 1

**Condition evaluation:**
- The condition is evaluated before each iteration, including the first
- If the condition is initially falsy, the body never executes
- Omitting the condition or providing an empty expression is a syntax error

**Empty bodies:**
- While loop bodies may be empty (contain zero statements)
- An empty body causes the loop to repeatedly evaluate the condition until it becomes falsy

**Example:**
```moo
i = 0;
while (i < 5)
  notify(player, tostr(i));
  i = i + 1;
endwhile

// Empty body with side-effecting condition
while (consume_input())
  // Side effect occurs in condition
endwhile
```

### 5.2 Named While

```moo
while name (condition)
  statements
endwhile
```

**Semantics:**
- `name` labels the loop for break/continue targeting

**Example:**
```moo
while outer (x > 0)
  while (y > 0)
    if (done)
      break outer;  // Exit outer loop
    endif
    y = y - 1;
  endwhile
  x = x - 1;
endwhile
```

---

## 6. Break and Continue

### 6.1 Break

```moo
break;
break loop_name;
```

**Semantics:**
- Without name: exits innermost loop
- With name: exits the named loop
- Invalid outside loop

### 6.2 Continue

```moo
continue;
continue loop_name;
```

**Semantics:**
- Without name: skips to next iteration of innermost loop
- With name: skips to next iteration of named loop
- Invalid outside loop

**Example:**
```moo
for i in [1..10]
  if (i % 2 == 0)
    continue;  // Skip even numbers
  endif
  if (i > 7)
    break;     // Stop at 7
  endif
  notify(player, tostr(i));
endfor
// Output: 1, 3, 5, 7
```

---

## 7. Fork Statement

### 7.1 Basic Fork

```moo
fork (delay)
  statements
endfork
```

**Semantics:**
- `delay` is seconds (INT or FLOAT) before execution
- Creates new background task
- Parent continues immediately
- Forked task executes `statements` after delay

**Example:**
```moo
notify(player, "Starting...");
fork (5)
  notify(player, "5 seconds later!");
endfork
notify(player, "This prints immediately");
```

### 7.2 Fork with Task ID

```moo
fork task_id (delay)
  statements
endfork
```

**Semantics:**
- `task_id` variable receives the new task's ID
- Can use for `kill_task(task_id)`

**Example:**
```moo
fork tid (10)
  expensive_operation();
endfork
// Later...
if (user_cancelled)
  kill_task(tid);
endif
```

### 7.3 Fork Delay Values

| Delay | Behavior |
|-------|----------|
| `0` | Execute as soon as possible |
| `> 0` | Execute after N seconds |
| `< 0` | E_INVARG error |
| Float | Sub-second precision |

---

## 8. Return Statement

```moo
return;
return expression;
```

**Semantics:**
- Without expression: returns `0`
- With expression: returns evaluated value
- Exits current verb/function immediately

**Example:**
```moo
if (!valid(obj))
  return E_INVARG;
endif
// ... do work ...
return result;
```

---

## 9. Try/Except Statement

### 9.1 Basic Try/Except

```moo
try
  statements
except (error_codes)
  handler_statements
endtry
```

**Semantics:**
1. Execute `try` block
2. If error raised and matches `error_codes`, execute handler
3. If error doesn't match, propagate to outer handler

### 9.2 Multiple Except Clauses

```moo
try
  risky_operation();
except e (E_TYPE)
  // Handle type errors, e = E_TYPE
except e (E_RANGE, E_INVIND)
  // Handle range/index errors
except (ANY)
  // Handle all other errors
endtry
```

**Matching order:**
- First matching clause handles the error
- Maximum 255 except clauses

### 9.3 Error Codes

| Form | Meaning |
|------|---------|
| `E_TYPE` | Specific error |
| `E_TYPE, E_RANGE` | Multiple specific errors |
| `ANY` | All errors |
| `@list_var` | Dynamic list of errors |

### 9.4 Error Variable

```moo
except variable (codes)
```

**Semantics:**
- `variable` bound to the error that was raised
- Type is ERR

**Example:**
```moo
try
  result = obj.property;
except e (E_PROPNF, E_INVIND)
  if (e == E_PROPNF)
    result = "default";
  else
    raise(e);  // Re-raise
  endif
endtry
```

---

## 10. Try/Finally Statement

```moo
try
  statements
finally
  cleanup_statements
endtry
```

**Semantics:**
- Execute `try` block
- `finally` block ALWAYS executes:
  - After normal completion
  - After error
  - After return
  - After break/continue
- If error occurred, re-raised after finally

**Example:**
```moo
file = file_open("data.txt", "r");
try
  process_file(file);
finally
  file_close(file);  // Always closes file
endtry
```

### 10.1 Try/Except/Finally

```moo
try
  statements
except (codes)
  handler
finally
  cleanup
endtry
```

**Execution order:**
1. Try block
2. If error, matching except handler
3. Finally block (always)
4. If unhandled error, propagate

---

## 11. Scattering Assignment

### 11.1 Syntax

```moo
{target, target, ...} = list_expression;
```

### 11.2 Target Types

| Syntax | Type | Behavior |
|--------|------|----------|
| `var` | Required | Must have value |
| `?var` | Optional | Uses `0` if missing |
| `?var = expr` | Optional with default | Uses `expr` if missing |
| `@var` | Rest | Collects remaining elements |

### 11.3 Examples

```moo
{a, b, c} = {1, 2, 3};
// a=1, b=2, c=3

{a, ?b} = {1};
// a=1, b=0

{a, ?b = "default"} = {1};
// a=1, b="default"

{first, @rest} = {1, 2, 3, 4};
// first=1, rest={2, 3, 4}

{a, ?b, @rest} = {1};
// a=1, b=0, rest={}
```

### 11.4 Errors

| Condition | Error |
|-----------|-------|
| Not enough values for required targets | E_ARGS |
| Right side not a list | E_TYPE |

---

## 12. Statement Execution Order

### 12.1 Sequential Execution

Statements execute top-to-bottom unless control flow changes.

### 12.2 Control Flow Priority

| Statement | Effect |
|-----------|--------|
| `return` | Exit verb immediately |
| `break` | Exit loop immediately |
| `continue` | Skip to loop next iteration |
| `raise()` | Propagate error |

### 12.3 Error Propagation

1. Error raised in statement
2. Check for enclosing try/except
3. If match found, execute handler
4. If no match, propagate to caller
5. If top-level, task aborts

---

## 13. Go Implementation Notes

### 13.1 Statement Interface

```go
type Stmt interface {
    Node
    stmtNode()
    Execute(vm *VM) error
}
```

### 13.2 Loop Labels

```go
type LoopContext struct {
    Label    string  // Optional name
    BreakTo  bool    // Break requested
    Continue bool    // Continue requested
}
```

### 13.3 Try/Finally Guarantee

```go
func (t *TryFinallyStmt) Execute(vm *VM) (err error) {
    defer func() {
        // Finally always runs
        finallyErr := t.Finally.Execute(vm)
        if err == nil {
            err = finallyErr
        }
    }()
    return t.Try.Execute(vm)
}
```
