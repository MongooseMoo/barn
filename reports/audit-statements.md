# Blind Implementor Audit: Statements Feature

## Executive Summary

Audited control flow statements (if, for, while, break/continue) from spec/statements.md. Found **31 specification gaps** where an implementor would need to guess, assume, or consult reference implementations. Critical gaps include loop variable scoping after termination, mutation visibility during iteration, label shadowing rules, and error condition precedence.

## Audit Scope

Focused on:
- Loop variable scoping
- Break/continue with labels
- Nested loop behavior
- Empty body handling
- Range iteration semantics

## Gaps Found

---

### GAP-001: Loop Variable After Normal Completion

**Feature:** for loops
**Spec File:** spec/statements.md
**Spec Section:** 4.1 List Iteration
**Gap Type:** guess

**Question:**
What value does the loop variable have after the loop completes normally (iterating all elements)? The spec says the variable is "bound to each element" but doesn't specify its value after iteration ends.

**Impact:**
An implementor might: (a) leave it as the last element value, (b) set to 0/null, (c) make it undefined, (d) delete the variable. Code like:

```moo
for x in ({1, 2, 3})
  // loop
endfor
return x;  // What is x?
```

...would produce different results depending on implementation choice.

**Suggested Addition:**
"After loop completion, the loop variable retains the value of the last element iterated. If the list was empty, the variable remains unmodified if it existed prior to the loop, or is set to 0 if it did not exist."

---

### GAP-002: Loop Variable After Break

**Feature:** for loops with break
**Spec File:** spec/statements.md
**Spec Section:** 4.1 List Iteration, 6.1 Break
**Gap Type:** guess

**Question:**
When a for loop exits via `break`, what value does the loop variable have? The spec doesn't address break interaction with loop variable state.

**Impact:**
Code that breaks early may expect the variable to hold the current element at break time, or may expect undefined behavior. This affects debugging and intentional early-exit patterns.

**Suggested Addition:**
"When a for loop exits via break, the loop variable retains the value it held at the moment break was executed."

---

### GAP-003: Loop Variable After Continue in Last Iteration

**Feature:** for loops with continue
**Spec File:** spec/statements.md
**Spec Section:** 4.1 List Iteration, 6.2 Continue
**Gap Type:** test

**Question:**
If the last iteration executes `continue`, what is the loop variable's value after the loop? Does the variable get bound to the "next" element (which doesn't exist) or remain as the last element?

**Impact:**
Edge case that could expose off-by-one errors or undefined behavior.

**Suggested Addition:**
"If continue is executed during the final iteration, the loop variable retains the value of that final element."

---

### GAP-004: Empty List Loop Variable Initialization

**Feature:** for loops with empty list
**Spec File:** spec/statements.md
**Spec Section:** 4.1 List Iteration
**Gap Type:** guess

**Question:**
When iterating an empty list, is the loop variable created and initialized to a default value, or does it remain untouched? If the variable didn't exist before the loop, does it exist after?

**Impact:**
Code like:

```moo
for x in ({})
endfor
return x;  // Does x exist? What's its value?
```

Different implementors might: (a) create x=0, (b) leave x undefined (E_VARNF), (c) not create x at all.

**Suggested Addition:**
"When iterating an empty list, the loop variable is not modified. If it did not exist prior to the loop, it remains undefined after the loop."

---

### GAP-005: Loop Variable Scoping with Nested Loops

**Feature:** for loops (nested)
**Spec File:** spec/statements.md
**Spec Section:** 4.1 List Iteration
**Gap Type:** test

**Question:**
In nested loops using the same variable name, what is the scoping behavior?

```moo
for i in ({1, 2})
  for i in ({10, 20})
    // Inner i shadows outer i?
  endfor
  // Is i from outer loop or inner loop?
endfor
```

Does MOO allow variable shadowing, use the same binding, or is this an error?

**Impact:**
Major correctness issue. If shadowing isn't supported, the inner loop clobbers the outer variable and breaks outer iteration. If shadowing is supported, implementor needs to track scope depth.

**Suggested Addition:**
"Loop variables do not create a new scope. Using the same variable name in nested loops causes the inner loop to overwrite the outer loop's variable, making the outer loop's iteration state inaccessible within the inner loop."

---

### GAP-006: For-Range with Start > End

**Feature:** for range
**Spec File:** spec/statements.md
**Spec Section:** 4.4 Range Iteration
**Gap Type:** assume

**Question:**
The spec says "If start > end, body never executes." What is the loop variable's value after a zero-iteration range loop?

```moo
for i in [5..1]
  // never executes
endfor
return i;  // What is i?
```

**Impact:**
Related to GAP-004 but for ranges specifically. Could be start, end, 0, or undefined.

**Suggested Addition:**
"When start > end, the loop body does not execute and the loop variable is not modified from its value before the loop (or remains undefined if it did not exist)."

---

### GAP-007: For-Range Overflow Behavior

**Feature:** for range
**Spec File:** spec/statements.md
**Spec Section:** 4.4 Range Iteration
**Gap Type:** test

**Question:**
What happens with ranges that would overflow INT64?

```moo
for i in [9223372036854775800..9223372036854775807]
  // Does this iterate 8 times or raise E_RANGE or overflow?
endfor
```

If iteration increments i, and i+1 overflows, is that checked?

**Impact:**
Could cause infinite loops, crashes, or silent corruption if overflow wraps. types.md says integer overflow is "undefined" but doesn't say if it's checked in range iteration.

**Suggested Addition:**
"Range iteration increments the loop variable using checked arithmetic. If incrementing the variable would overflow INT range, E_RANGE is raised."

---

### GAP-008: For-Range Negative Ranges

**Feature:** for range
**Spec File:** spec/statements.md
**Spec Section:** 4.4 Range Iteration
**Gap Type:** assume

**Question:**
Are negative ranges supported for counting down?

```moo
for i in [5..1]   // Already covered: doesn't execute
for i in [1..-5]  // What about this?
```

Spec says start > end means no iteration, implying downward iteration isn't supported, but doesn't explicitly forbid negative endpoints.

**Impact:**
Some implementors might assume Python-like step semantics or expect an error for negative end.

**Suggested Addition:**
"Ranges iterate upward only (incrementing by 1). If start > end, the loop does not execute. Negative values are allowed for both start and end (e.g., [-5..-1] iterates -5, -4, -3, -2, -1)."

---

### GAP-009: Range Special Marker $ in Non-List Context

**Feature:** for range with $
**Spec File:** spec/statements.md
**Spec Section:** 4.4 Range Iteration
**Gap Type:** ask

**Question:**
Section 4.4 mentions "$" as a special marker that requires "list context." What exactly is "list context" for a range?

```moo
for i in [1..$]  // Error? What list?
  // ???
endfor
```

Is this syntax valid only inside a larger expression involving a list, or is it never valid in a for range? The spec is ambiguous.

**Impact:**
Implementor wouldn't know if this is a syntax error, a runtime error (E_TYPE?), or requires a surrounding list binding.

**Suggested Addition:**
"The $ marker in range syntax [start..$] is only valid when the range appears as a list slicing or indexing expression (e.g., list[1..$]). Using $ in a for-range (for i in [1..$]) is a syntax error."

---

### GAP-010: Map Iteration Order

**Feature:** for loops over maps
**Spec File:** spec/statements.md
**Spec Section:** 4.3 Map Iteration
**Gap Type:** assume

**Question:**
Section 4.3 says maps are "iterated over." The types.md spec says maps are "unordered (implementation-defined iteration order)." Does this mean:
- (a) The order is arbitrary but stable within one iteration?
- (b) The order can change between iterations over the same map?
- (c) The order is deterministic but not specified?

**Impact:**
If iteration order changes mid-loop (because the map is mutated or for other reasons), iterating behavior is unpredictable. Implementors need to know if they should snapshot keys at loop start or iterate the live map.

**Suggested Addition:**
"Map iteration order is implementation-defined but stable: iterating the same unmodified map multiple times yields the same order. If the map is mutated during iteration (see GAP-011), order guarantees do not apply."

---

### GAP-011: Mutation Visibility During List Iteration

**Feature:** for loops (mutation)
**Spec File:** spec/statements.md
**Spec Section:** 4.1 List Iteration
**Gap Type:** test

**Question:**
If the list being iterated is modified during iteration, is the change visible to the loop?

```moo
mylist = {1, 2, 3};
for x in (mylist)
  mylist = {@mylist, x + 10};  // Append to the list being iterated
endfor
// Does the loop iterate 3 times or infinitely?
```

Copy-on-write semantics (types.md 5.4) suggest the iteration is over a snapshot, but the spec doesn't explicitly state this for loop behavior.

**Impact:**
If modifications are visible, loops can become infinite or skip elements. If not visible, the iteration is safe but that behavior must be specified.

**Suggested Addition:**
"The list expression is evaluated once before loop execution begins. Modifications to the list variable during iteration do not affect the loop's iteration sequence."

---

### GAP-012: Mutation Visibility During Map Iteration

**Feature:** for loops over maps (mutation)
**Spec File:** spec/statements.md
**Spec Section:** 4.3 Map Iteration
**Gap Type:** test

**Question:**
Similar to GAP-011 but for maps. If the map is mutated during iteration (keys added/removed), what happens?

```moo
m = ["a" -> 1];
for v, k in (m)
  m["b"] -> 2;  // Add entry during iteration
endfor
// Does the loop see the new entry?
```

**Impact:**
Implementors might snapshot keys at loop start (safe), or iterate live (unpredictable). Without specification, implementations will diverge.

**Suggested Addition:**
"The map expression is evaluated once before loop execution begins. Modifications to the map during iteration (adding, removing, or changing entries) do not affect the current iteration sequence."

---

### GAP-013: Loop Label Scoping and Shadowing

**Feature:** named loops
**Spec File:** spec/statements.md
**Spec Section:** 4.5 Named For Loops, 5.2 Named While
**Gap Type:** ask

**Question:**
Can loop labels be shadowed in nested loops?

```moo
for outer x in ({1, 2})
  for outer y in ({10, 20})  // Same label name
    break outer;  // Which loop does this exit?
  endfor
endfor
```

Is this a syntax error, or does the inner label shadow the outer? If shadowing is allowed, `break outer` would exit the inner loop, which might be confusing.

**Impact:**
Label shadowing could lead to subtle bugs. Implementors need clear scoping rules.

**Suggested Addition:**
"Loop labels must be unique within nested loop scopes. Using the same label name for a nested loop is a syntax error."

---

### GAP-014: Break/Continue with Nonexistent Label

**Feature:** break/continue with labels
**Spec File:** spec/statements.md
**Spec Section:** 6.1 Break, 6.2 Continue
**Gap Type:** assume

**Question:**
What error is raised when break/continue references a label that doesn't exist?

```moo
for x in ({1, 2})
  break nonexistent_label;
endfor
```

Spec says labeled break/continue "exits the named loop" but doesn't specify the error for non-existent labels. Is it a compile-time error or runtime?

**Impact:**
Likely compile-time, but implementor would have to guess or check reference code.

**Suggested Addition:**
"Referencing a non-existent loop label in break or continue is a compile-time error."

---

### GAP-015: Break/Continue Across Try/Except Boundary

**Feature:** break/continue in try/except
**Spec File:** spec/statements.md
**Spec Section:** 6.1 Break, 6.2 Continue, 9.1 Try/Except
**Gap Type:** test

**Question:**
Can break or continue be used inside a try/except block to exit an enclosing loop?

```moo
for x in ({1, 2, 3})
  try
    if (x == 2)
      break;  // Valid?
    endif
  except (ANY)
  endtry
endfor
```

Spec says break/continue are "invalid outside loop" but doesn't address try/except boundaries.

**Impact:**
Implementors need to know if try/except creates a control flow barrier. Most languages allow this, but MOO might differ.

**Suggested Addition:**
"Break and continue are valid inside try/except/finally blocks when enclosed by a loop. They exit the loop normally, executing any finally block before exiting."

---

### GAP-016: Break/Continue Across Fork Boundary

**Feature:** break/continue in fork
**Spec File:** spec/statements.md
**Spec Section:** 6.1 Break, 6.2 Continue, 7.1 Fork
**Gap Type:** ask

**Question:**
Can break or continue be used inside a fork block to affect an enclosing loop?

```moo
for x in ({1, 2, 3})
  fork (0)
    break;  // Does this break the outer loop?
  endfork
endfor
```

Forked tasks execute asynchronously, so this likely doesn't make sense, but the spec doesn't forbid it.

**Impact:**
Should be a compile-time error or runtime error, but which?

**Suggested Addition:**
"Break and continue are invalid inside fork blocks, even if the fork is syntactically enclosed by a loop. Attempting to use them inside a fork is a compile-time error."

---

### GAP-017: While Loop Condition Evaluation Order

**Feature:** while loops
**Spec File:** spec/statements.md
**Spec Section:** 5.1 Basic While
**Gap Type:** assume

**Question:**
The spec says the condition is evaluated, then the body executes, then "Go to step 1." Is the condition evaluated *before* the first iteration, or is this a do-while that executes body first?

Semantics section implies condition is checked first (like most languages), but the wording could be clearer.

**Impact:**
Do-while vs while semantics are fundamentally different. An empty condition would never execute body (while) vs always execute once (do-while).

**Suggested Addition:**
"While loops evaluate the condition before each iteration, including the first. If the condition is initially falsy, the body never executes."

---

### GAP-018: While Loop with Empty Condition Expression

**Feature:** while loops
**Spec File:** spec/statements.md
**Spec Section:** 5.1 Basic While
**Gap Type:** test

**Question:**
What happens if the while condition is an empty expression or missing?

```moo
while ()
  // ???
endwhile
```

Is this a syntax error?

**Impact:**
Likely a parse error, but spec doesn't have syntax grammar for error cases.

**Suggested Addition:**
"The while condition must be a valid expression. Omitting the condition or providing an empty expression is a syntax error."

---

### GAP-019: If/ElseIf/Else Empty Body Behavior

**Feature:** if statements
**Spec File:** spec/statements.md
**Spec Section:** 3.1 Syntax, 3.2 Semantics
**Gap Type:** assume

**Question:**
Are empty statement bodies allowed in if/elseif/else?

```moo
if (x > 0)
  // Empty - no statements
elseif (x < 0)
  ; // Explicit empty statement
else
endif
```

Spec syntax uses `statements` (plural) which might imply one or more, or might allow zero.

**Impact:**
Empty bodies are common during development or for readability. If disallowed, implementor needs to require at least `;`.

**Suggested Addition:**
"If, elseif, and else bodies may be empty (contain zero statements). Empty bodies are no-ops."

---

### GAP-020: For Loop Empty Body

**Feature:** for loops
**Spec File:** spec/statements.md
**Spec Section:** 4.1 List Iteration
**Gap Type:** assume

**Question:**
Is an empty for loop body allowed?

```moo
for x in ({1, 2, 3})
  // Empty
endfor
```

This might be useful for side effects in the iteration expression or for loop variable assignments, but the spec doesn't explicitly allow or forbid it.

**Impact:**
Minor, but affects parser implementation.

**Suggested Addition:**
"For loop bodies may be empty. An empty body is a no-op but the loop variable is still bound for each iteration."

---

### GAP-021: While Loop Empty Body

**Feature:** while loops
**Spec File:** spec/statements.md
**Spec Section:** 5.1 Basic While
**Gap Type:** assume

**Question:**
Is an empty while loop body allowed?

```moo
while (consume_input())
  // Side effect in condition
endwhile
```

**Impact:**
Similar to GAP-020.

**Suggested Addition:**
"While loop bodies may be empty. An empty body causes the loop to repeatedly evaluate the condition until it becomes falsy."

---

### GAP-022: Try Block Empty Body

**Feature:** try/except
**Spec File:** spec/statements.md
**Spec Section:** 9.1 Try/Except
**Gap Type:** assume

**Question:**
Can the try block body be empty?

```moo
try
  // Nothing
except (E_TYPE)
  handle_error();
endtry
```

Seems pointless but might occur during refactoring.

**Impact:**
Minor.

**Suggested Addition:**
"Try, except, and finally blocks may be empty. An empty try block is a no-op."

---

### GAP-023: Scattering Assignment in For Loop Context

**Feature:** for loops, scattering
**Spec File:** spec/statements.md
**Spec Section:** 4.1 List Iteration, 11 Scattering Assignment
**Gap Type:** ask

**Question:**
Can scattering assignment be used as the loop variable in a for loop?

```moo
for {a, b} in ({{1, 2}, {3, 4}})
  // a=1, b=2 on first iteration?
endfor
```

Section 11 describes scattering assignment as a statement, not as a for-loop binding. Spec doesn't mention if this syntax is supported.

**Impact:**
This would be very useful for iterating over lists of tuples. If supported, implementor needs to handle scattering in loop binding. If not, should be explicitly forbidden.

**Suggested Addition:**
"For loop variables are simple identifiers. Scattering assignment syntax is not supported in for loop variable bindings."

(Alternative if supported: "For loop variables may use scattering assignment syntax to destructure list elements during iteration.")

---

### GAP-024: Multiple Loop Variables with Map Iteration (Order)

**Feature:** for loops over maps
**Spec File:** spec/statements.md
**Spec Section:** 4.3 Map Iteration
**Gap Type:** assume

**Question:**
Section 4.3 shows:

```moo
for value, key in (map_expression)
```

Note the order: value first, then key. Is this intentional or a typo? Most languages use (key, value) for map iteration. The example shows this order too:

```moo
for age, name in (ages)
```

Where `ages = ["Alice" -> 30, "Bob" -> 25]`, so `age` is the value and `name` is the key. Is this the intended convention?

**Impact:**
This is backwards from common conventions. Implementors might "fix" it to (key, value) assuming it's a spec error.

**Suggested Addition:**
"Map iteration with two variables binds them in (value, key) order: the first variable receives the value, the second receives the key. This convention matches the map literal syntax (key -> value) reading right-to-left."

(Alternatively, if this was an error: fix to `for key, value in (map)` throughout.)

---

### GAP-025: Exception Handler Error Variable Scope

**Feature:** try/except
**Spec File:** spec/statements.md
**Spec Section:** 9.4 Error Variable
**Gap Type:** test

**Question:**
What is the scope of the error variable in an except clause?

```moo
try
  risky();
except e (E_TYPE)
  handle(e);
endtry
// Is e still accessible here?
```

Does `e` remain bound after the except block, or is it scoped only to the handler?

**Impact:**
Variable scoping. If `e` remains bound, it could shadow outer variables. If scoped, implementor needs block-local variables.

**Suggested Addition:**
"The error variable in an except clause is scoped to that except block only. It is not accessible after the endtry."

---

### GAP-026: Try/Finally with Return Value

**Feature:** try/finally
**Spec File:** spec/statements.md
**Spec Section:** 10 Try/Finally
**Gap Type:** test

**Question:**
What happens if both the try block and finally block return values?

```moo
try
  return 1;
finally
  return 2;
endtry
// What is returned?
```

Spec says finally "ALWAYS executes" after return, but doesn't specify if finally's return overrides try's return.

**Impact:**
Most languages let finally override return values, but some don't. This is a critical semantic.

**Suggested Addition:**
"If both the try block and finally block execute return statements, the return value from the finally block is used."

---

### GAP-027: Try/Finally with Break/Continue

**Feature:** try/finally with break/continue
**Spec File:** spec/statements.md
**Spec Section:** 10 Try/Finally
**Gap Type:** test

**Question:**
Spec says finally executes "after break/continue." What if finally also has a break or continue?

```moo
for x in ({1, 2, 3})
  try
    break;
  finally
    continue;  // Override the break?
  endtry
endfor
```

Does the finally's continue override the try's break?

**Impact:**
Control flow precedence rules.

**Suggested Addition:**
"If the try block executes break or continue, and the finally block also executes break or continue, the finally block's control flow statement takes precedence."

---

### GAP-028: Try/Finally Error Re-Raising

**Feature:** try/finally
**Spec File:** spec/statements.md
**Spec Section:** 10 Try/Finally
**Gap Type:** assume

**Question:**
Spec says "If error occurred, re-raised after finally." What if the finally block also raises an error?

```moo
try
  error_a();  // Raises E_TYPE
finally
  error_b();  // Raises E_RANGE
endtry
// Which error propagates?
```

**Impact:**
Error precedence. Most languages propagate the finally error and lose the try error.

**Suggested Addition:**
"If both the try block and finally block raise errors, the error from the finally block is propagated and the try block's error is discarded."

---

### GAP-029: For-Range Type Checking Timing

**Feature:** for range
**Spec File:** spec/statements.md
**Spec Section:** 4.4 Range Iteration
**Gap Type:** test

**Question:**
Spec says "start and end must be INT." When is this checked - at loop entry or when evaluating the range expression?

```moo
x = 1.5;
for i in [x..5]  // Error when? At for statement or when x is evaluated?
endfor
```

If `x` is a variable, the type check happens at runtime. But is it before the first iteration or during range construction?

**Impact:**
Error timing affects whether the loop body ever executes.

**Suggested Addition:**
"Range start and end expressions are evaluated and type-checked before the first iteration. If either is not INT, E_TYPE is raised before the loop body executes."

---

### GAP-030: For-List Type Checking Timing

**Feature:** for list iteration
**Spec File:** spec/statements.md
**Spec Section:** 4.1 List Iteration
**Gap Type:** test

**Question:**
Spec says list_expression "must be LIST." When is this checked?

```moo
x = 42;
for i in (x)  // E_TYPE immediately, or after evaluating x?
endfor
```

**Impact:**
Similar to GAP-029. Does the loop attempt any iterations before failing?

**Suggested Addition:**
"The list expression is evaluated and type-checked before the first iteration. If it is not a LIST, E_TYPE is raised before the loop body executes."

---

### GAP-031: For-Map Single Variable Iteration Value

**Feature:** for loops over maps (single variable)
**Spec File:** spec/statements.md
**Spec Section:** 4.3 Map Iteration
**Gap Type:** assume

**Question:**
Section 4.3 says single-variable map iteration "receives values." But are these the values from the map entries, or are they [key, value] pairs, or something else?

```moo
m = ["a" -> 1, "b" -> 2];
for x in (m)
  // Is x = 1 (the value), or x = {"a", 1} (a pair), or x = "a" (the key)?
endfor
```

The wording "receives values" suggests x = 1, 2, ... but this isn't 100% explicit.

**Impact:**
If implementor assumes "values" means [key, value] pairs (common in some languages), the implementation will diverge.

**Suggested Addition:**
"Single-variable map iteration binds the variable to the value of each map entry, ignoring the keys. To access both key and value, use the two-variable form."

---

## Summary Statistics

- **Total gaps found:** 31
- **Guess:** 7 (implementor must choose with no guidance)
- **Assume:** 13 (implied but not stated clearly)
- **Ask:** 5 (ambiguous, need clarification)
- **Test:** 6 (need to check reference implementation)

## High-Priority Gaps

Critical gaps that would cause implementation divergence or incorrectness:

1. **GAP-005** (nested loop variable scoping) - Major correctness issue
2. **GAP-011** (list mutation during iteration) - Could cause infinite loops
3. **GAP-013** (label shadowing) - Could cause wrong loop exits
4. **GAP-024** (map iteration order: value-key vs key-value) - API convention
5. **GAP-026** (try/finally return precedence) - Control flow semantics
6. **GAP-001** (loop variable after completion) - Common usage pattern

## Recommendations

1. **Add explicit variable scoping rules** for loop variables, including:
   - Post-loop value
   - Nested loop shadowing behavior
   - Empty-collection handling

2. **Specify mutation visibility** during iteration for lists and maps

3. **Clarify control flow precedence** when multiple flow-altering statements occur (return/break/continue in try/finally)

4. **Document error-checking timing** for type checks in loop constructs

5. **Add edge case examples** for:
   - Empty collections
   - Single-element collections
   - Break/continue in first/last iterations
   - Nested loops with same variable names

6. **Standardize terminology**: "value, key" vs "key, value" for maps
