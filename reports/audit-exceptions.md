# Exception Handling Feature Audit

**Feature:** Try/Except, Try/Finally, Catch Expression
**Auditor:** Blind Implementor (spec-only analysis)
**Date:** 2025-12-24
**Spec Files Reviewed:**
- `spec/statements.md` (sections 9-10)
- `spec/operators.md` (section 4)
- `spec/grammar.md` (section 3)
- `spec/errors.md` (all sections)

---

## Executive Summary

The exception handling specification has **33 significant gaps** across syntax, semantics, edge cases, and implementation details. Key areas of concern:

1. **No raise() builtin specification** - referenced but not documented
2. **Unclear catch expression matching semantics** - OR vs AND behavior
3. **Missing nested try/except interaction rules**
4. **Undefined behavior for exceptions during finally blocks**
5. **Incomplete error variable binding semantics**

**Impact:** An implementor would need to make 20+ design decisions based on guessing, testing, or tribal knowledge.

---

## Gap Analysis

### CRITICAL GAPS (Blocker Issues)

---

- id: GAP-EXC-001
  feature: "exception handling"
  spec_file: "spec/statements.md"
  spec_section: "9.4 Error Variable"
  gap_type: guess
  question: |
    What is the scope of the error variable bound in except clauses?

    The spec says "variable bound to the error that was raised" but doesn't specify:
    - Is it local to the except block only?
    - Does it shadow outer variables of the same name?
    - Does it persist after the except block ends?
    - Can it be reassigned within the except block?

    Example:
    ```moo
    e = "outer";
    try
      risky();
    except e (E_TYPE)
      // e is now E_TYPE
      e = "modified";
    endtry
    // What is e here? "modified", E_TYPE, or "outer"?
    ```
  impact: |
    Critical for scoping rules. Different choices:
    (a) Block-scoped (like Python) - e is local to except, outer e unchanged
    (b) Function-scoped (like JavaScript var) - e persists and can shadow
    (c) Overwrites existing variable - e permanently changed

    These produce completely different behavior.
  suggested_addition: |
    Add to section 9.4:
    "The error variable is scoped to the except clause body only. It shadows
    any outer variable with the same name, and the shadow ends when the except
    block completes. The variable can be read and written within the except block."

---

- id: GAP-EXC-002
  feature: "exception handling"
  spec_file: "spec/statements.md"
  spec_section: "9.1 Basic Try/Except"
  gap_type: guess
  question: |
    What happens if no except clause matches the raised error?

    The spec says "If error doesn't match, propagate to outer handler" but doesn't clarify:
    - What if there is NO outer handler? Does the task abort?
    - Is there a stack trace?
    - What error message is shown?
    - Can this be observed/logged?
  impact: |
    Fundamental to error handling model. Implementor must choose:
    (a) Silent task termination
    (b) Task abort with error code returned to caller
    (c) Error message to player/log
    (d) Stack trace generation
  suggested_addition: |
    Add to section 9.1:
    "If an error is not caught by any except clause and propagates to the top level,
    the task aborts with that error code. The error is returned to the calling verb
    (if any) or displayed to the player if at top-level."

---

- id: GAP-EXC-003
  feature: "exception handling"
  spec_file: "spec/statements.md"
  spec_section: "None - missing entirely"
  gap_type: ask
  question: |
    Is there a raise() builtin function, and if so, what is its signature and behavior?

    The spec references raise() in examples:
    - statements.md:451: "raise(e);"
    - statements.md:565: "| `raise()` | Propagate error |"
    - builtins/exec.md:216: "raise(E_EXEC, err);"

    But there is NO specification for this function. The implementor doesn't know:
    - Signature: raise(error_code) or raise(error_code, message)?
    - Can you raise with a custom message?
    - Can you raise with arbitrary data?
    - What happens if you call raise() outside a try block?
    - Can you raise from a finally block?
  impact: |
    CRITICAL. Cannot implement exception handling without knowing how to raise errors.
    The examples suggest two different signatures:
    - raise(e) - single argument
    - raise(E_EXEC, err) - two arguments

    Are these the same function? Different forms?
  suggested_addition: |
    Add new section to spec/builtins/tasks.md:

    "### raise

    **Signature:** `raise(error_code [, message [, value]]) → none`

    **Description:** Raises an error that can be caught by try/except or propagates to caller.

    **Parameters:**
    - error_code: ERR - One of E_TYPE, E_RANGE, etc.
    - message: STR (optional) - Custom error message
    - value: VALUE (optional) - Additional error data

    **Examples:**
    ```moo
    raise(E_INVARG);                    // Simple raise
    raise(E_INVARG, "Invalid user");    // With message
    ```

    **Errors:**
    - E_TYPE: error_code is not ERR type"

---

- id: GAP-EXC-004
  feature: "catch expression"
  spec_file: "spec/operators.md"
  spec_section: "4. Catch Expression"
  gap_type: guess
  question: |
    For catch expressions with multiple error codes like `expr ! E_DIV, E_TYPE`,
    are the codes OR'd or AND'd?

    The spec says "Specific errors (comma = OR, not AND)" but this parenthetical
    is unclear. Does it mean:
    - "Comma means OR" (catches if EITHER error occurs)
    - "Comma is OR, NOT and" (emphasizing it's not intersection)

    Also, can you write `expr ! E_DIV && E_TYPE` to mean AND?
  impact: |
    The parenthetical note suggests OR, but it's stated as a clarification
    rather than the main semantics. An implementor might read "E_DIV, E_TYPE"
    as "both must match" without the note.

    More importantly: can you express AND semantics? Is `&&` supported in
    error code lists?
  suggested_addition: |
    Rewrite the "Error code forms" table in section 4:

    | Form | Meaning |
    |------|---------|
    | `E_TYPE` | Catch E_TYPE only |
    | `E_TYPE, E_RANGE` | Catch E_TYPE OR E_RANGE (comma = OR) |
    | `ANY` | Catch all errors |
    | `@list_var` | Catch any error in the list (OR semantics) |

    Add: "There is no AND operator for error codes. To handle multiple specific
    errors differently, use nested try blocks or check the error variable."

---

- id: GAP-EXC-005
  feature: "catch expression"
  spec_file: "spec/operators.md"
  spec_section: "4. Catch Expression"
  gap_type: guess
  question: |
    When a catch expression has no default (`expr ! E_TYPE`), what value is returned
    if the error matches?

    The spec says:
    - "If error matches error_codes, return error (or default)"

    But what is "return error"? Is it:
    (a) The error code value (E_TYPE)
    (b) The number 0
    (c) The ERR type value
    (d) Some error object with message/traceback
  impact: |
    Example: `result = `5 / 0 ! E_DIV`;`

    If E_DIV matches, what is result?
    - Option (a): result = E_DIV (the error code constant)
    - Option (b): result = 0 (some default)
    - Option (c): result is an ERR type value equal to E_DIV

    These are different types and values.
  suggested_addition: |
    In section 4, replace:
    "If error matches error_codes, return error (or default)"

    With:
    "If error matches error_codes and a default is provided, return the default.
    If no default is provided, return the error code (ERR type) that was raised."

    Add example:
    ```moo
    `5 / 0 ! E_DIV`          => E_DIV (error code returned)
    `5 / 0 ! E_DIV => 999`   => 999 (default returned)
    `5 / 0 ! E_TYPE`         => Propagates E_DIV (no match)
    ```

---

- id: GAP-EXC-006
  feature: "try/finally"
  spec_file: "spec/statements.md"
  spec_section: "10. Try/Finally Statement"
  gap_type: guess
  question: |
    What happens if an error is raised DURING the finally block?

    The spec says "If error occurred, re-raised after finally" but doesn't address:
    - What if the finally block itself raises an error?
    - Does the original error get lost?
    - Are both errors preserved?
    - Is there a "suppressed exception" mechanism?

    Example:
    ```moo
    try
      raise(E_TYPE);
    finally
      raise(E_RANGE);  // New error in finally
    endtry
    // Which error propagates: E_TYPE or E_RANGE?
    ```
  impact: |
    Critical for cleanup code reliability. Options:
    (a) Finally error replaces original (loses E_TYPE, propagates E_RANGE)
    (b) Finally error is ignored (preserves E_TYPE, loses E_RANGE) - dangerous!
    (c) Both errors preserved somehow
    (d) Runtime error about "exception in finally"

    Python: finally error replaces original
    Java: finally error replaces original
    Go: can preserve both with defer/recover
  suggested_addition: |
    Add to section 10:
    "If the finally block raises an error, that error replaces any error that was
    being propagated from the try block. The original error is lost. This is why
    finally blocks should avoid operations that can fail.

    Example:
    ```moo
    try
      raise(E_TYPE);
    finally
      raise(E_RANGE);  // Replaces E_TYPE
    endtry
    // E_RANGE propagates, E_TYPE is lost
    ```"

---

- id: GAP-EXC-007
  feature: "try/finally"
  spec_file: "spec/statements.md"
  spec_section: "10. Try/Finally Statement"
  gap_type: test
  question: |
    Does finally execute when there's a return statement in the try block?

    The spec says finally "ALWAYS executes" and lists "After return" but doesn't
    clarify the execution order or return value behavior.

    Example:
    ```moo
    function test()
      try
        return 1;
      finally
        return 2;
      endtry
    endfunction
    // What is returned: 1 or 2?
    ```

    And:
    ```moo
    function test()
      x = 0;
      try
        return x;
      finally
        x = 5;
      endtry
    endfunction
    // Does this return 0 or 5?
    ```
  impact: |
    Affects understanding of "return" semantics. Options:
    (a) Finally executes, but original return value is preserved (return 1)
    (b) Finally can override return value (return 2)
    (c) Return in finally replaces try-block return
    (d) Multiple returns are an error

    Most languages (Python/Java): finally can override return value
    This is often considered a misfeature but is the standard behavior.
  suggested_addition: |
    Add to section 10:
    "If the try block executes a return statement, the finally block still executes
    before the function returns. If the finally block also executes a return statement,
    the finally block's return value replaces the try block's return value.

    Example:
    ```moo
    try
      return 1;
    finally
      return 2;
    endtry
    // Returns 2 (finally overrides)
    ```"

---

- id: GAP-EXC-008
  feature: "try/finally"
  spec_file: "spec/statements.md"
  spec_section: "10. Try/Finally Statement"
  gap_type: guess
  question: |
    Does finally execute for break/continue statements?

    The spec says "After break/continue" but doesn't clarify:
    - Does finally execute before the break/continue takes effect?
    - Can finally prevent the break/continue?
    - What if finally has its own break/continue?

    Example:
    ```moo
    for i in [1..10]
      try
        if (i == 5)
          break;
        endif
      finally
        log("cleanup for i=" + tostr(i));
      endtry
    endfor
    // Does finally run for i=5 before breaking?
    ```
  impact: |
    Affects loop cleanup guarantees. Implementor must decide:
    (a) Finally executes, then break happens
    (b) Break happens immediately, finally skipped
    (c) Break is queued, finally executes, then break processes
  suggested_addition: |
    Add to section 10:
    "When a break or continue statement is executed in the try block, the finally
    block executes first, then the break/continue takes effect.

    Example:
    ```moo
    for i in [1..3]
      try
        if (i == 2)
          break;  // Will exit loop...
        endif
      finally
        log("i=" + tostr(i));  // ...but this runs first
      endtry
    endfor
    // Logs: i=1, i=2, then breaks
    ```"

---

- id: GAP-EXC-009
  feature: "try/except"
  spec_file: "spec/statements.md"
  spec_section: "9.2 Multiple Except Clauses"
  gap_type: assume
  question: |
    What does "Maximum 255 except clauses" mean?

    The spec states this as a hard limit but doesn't explain:
    - Is this 255 except clauses per try block?
    - Is this 255 except clauses total in a verb?
    - Why exactly 255? (Suggests a single-byte limit somewhere)
    - What error occurs if you exceed this? Compile error? Runtime?
    - Is this limit per except block or cumulative with nested tries?
  impact: |
    Unlikely to matter in practice (who writes 255 except clauses?), but
    the specificity suggests implementation details leaking into the spec.
    An implementor might wonder if this is a fundamental language limit
    or just a current implementation constraint.
  suggested_addition: |
    Either:
    (a) Remove this limit if it's not fundamental to the language
    (b) Clarify: "A single try statement can have at most 255 except clauses.
        This is a compile-time limit. Exceeding it results in a compilation error."

---

- id: GAP-EXC-010
  feature: "try/except"
  spec_file: "spec/statements.md"
  spec_section: "9.3 Error Codes"
  gap_type: guess
  question: |
    For the `@list_var` form of error codes, when is the list evaluated?

    The spec shows `@list_var` as a way to dynamically specify error codes, but doesn't say:
    - Is the list evaluated once when the try is entered?
    - Is it evaluated each time an exception is raised?
    - Is it evaluated at compile time (making it useless)?

    Example:
    ```moo
    errors = {E_TYPE};
    try
      errors = {E_RANGE};  // Change the list during try
      raise(E_RANGE);
    except (@errors)
      // Does this catch E_RANGE?
    endtry
    ```
  impact: |
    Affects whether error handling can be dynamically reconfigured during execution.
    Options:
    (a) Evaluated at try entry - errors is captured as {E_TYPE}, E_RANGE not caught
    (b) Evaluated at exception time - errors is {E_RANGE}, E_RANGE is caught
    (c) Evaluated at except clause consideration - somewhere in between
  suggested_addition: |
    Add to section 9.3:
    "The `@list_var` form evaluates the expression when the try statement is entered,
    before any statements in the try block execute. The evaluated list is captured and
    used for all exceptions raised during that try block. Subsequent modifications to
    the variable do not affect the except matching."

---

### HIGH PRIORITY GAPS (Significant Ambiguity)

---

- id: GAP-EXC-011
  feature: "try/except/finally"
  spec_file: "spec/statements.md"
  spec_section: "10.1 Try/Except/Finally"
  gap_type: test
  question: |
    What is the execution order for try/except/finally when an exception is caught?

    The spec says:
    1. Try block
    2. If error, matching except handler
    3. Finally block (always)
    4. If unhandled error, propagate

    But this doesn't clarify:
    - Does finally run AFTER the except handler completes?
    - If except handler returns, does finally run before the return?
    - If except handler raises a new error, does finally run before propagating?

    Example:
    ```moo
    try
      raise(E_TYPE);
    except (E_TYPE)
      log("handling");
      return 1;
    finally
      log("cleanup");
    endtry
    log("after");
    ```
    What is logged and in what order?
  impact: |
    Affects understanding of cleanup guarantees. Most languages:
    - Finally runs after except completes (or during return/raise)
    - Finally always runs before any exit from the try/except block

    But the spec's numbered list could be read as:
    - Try → except → finally → normal execution
    Which might suggest finally runs even after a return in except.
  suggested_addition: |
    Clarify section 10.1 execution order:
    "Execution order for try/except/finally:
    1. Execute try block
    2. If exception raised and matches an except clause:
       a. Execute that except handler
       b. Execute finally block
       c. If except handler raised new error, propagate after finally
       d. If except handler returned, return after finally
       e. Otherwise, continue to step 4
    3. If exception raised and no match:
       a. Execute finally block
       b. Propagate exception
    4. If no exception:
       a. Execute finally block
       b. Continue normal execution"

---

- id: GAP-EXC-012
  feature: "nested try/except"
  spec_file: "spec/statements.md"
  spec_section: "None - missing"
  gap_type: test
  question: |
    How do nested try/except blocks interact?

    The spec doesn't address nested exception handlers at all. Specifically:
    - If inner except doesn't match, does it propagate to outer except?
    - If inner except matches and handles, does outer except see anything?
    - Can you re-raise an exception to outer handler?

    Example:
    ```moo
    try
      try
        raise(E_TYPE);
      except (E_RANGE)
        // Doesn't match
      endtry
    except (E_TYPE)
      // Does this catch it?
    endtry
    ```
  impact: |
    Nested exception handling is common. Without specification:
    - Implementor might think exceptions can only be caught at one level
    - Propagation semantics unclear
    - Re-raising semantics undefined
  suggested_addition: |
    Add new section 9.5 "Nested Exception Handling":

    "Try/except blocks can be nested. When an exception is raised:
    1. The innermost enclosing try block's except clauses are checked first
    2. If no match, the exception propagates to the next outer try block
    3. This continues until either a match is found or the exception reaches top level

    Example:
    ```moo
    try
      try
        raise(E_TYPE);
      except (E_RANGE)
        // Doesn't match, propagates to outer
      endtry
    except (E_TYPE)
      // This catches it
    endtry
    ```"

---

- id: GAP-EXC-013
  feature: "catch expression"
  spec_file: "spec/operators.md"
  spec_section: "4. Catch Expression"
  gap_type: guess
  question: |
    Can catch expressions be nested, and if so, how do they interact?

    Example:
    ```moo
    result = ``x / y ! E_DIV => 0` ! E_TYPE => -1`;
    ```

    If x/y raises E_TYPE, does:
    (a) Inner catch propagate E_TYPE to outer catch (result = -1)
    (b) Inner catch not match, raise propagates without outer catch seeing it
    (c) Syntax error - can't nest catch expressions
  impact: |
    Common pattern for multi-level error handling. Need to know if this works.
  suggested_addition: |
    Add to section 4:
    "Catch expressions can be nested. The inner catch expression is evaluated first.
    If the inner catch does not match the error, the error propagates and the outer
    catch expression can handle it.

    Example:
    ```moo
    ``inner ! E_TYPE => 1` ! E_RANGE => 2`
    // If inner raises E_RANGE, inner doesn't match, outer catches it (=> 2)
    ```"

---

- id: GAP-EXC-014
  feature: "catch expression"
  spec_file: "spec/operators.md"
  spec_section: "4. Catch Expression"
  gap_type: test
  question: |
    Can the default expression in a catch also raise an error?

    Example:
    ```moo
    result = `x / y ! E_DIV => risky_fallback()`;
    ```

    If risky_fallback() raises E_TYPE:
    (a) E_TYPE propagates out of the catch expression
    (b) The catch expression re-catches it (infinite loop?)
    (c) It's an error to raise in a catch default
  impact: |
    Affects composability of error handling. Need to know if fallbacks can fail.
  suggested_addition: |
    Add to section 4:
    "The default expression is evaluated in normal context. If it raises an error,
    that error propagates normally (the catch does not apply to the default expression).

    Example:
    ```moo
    `x / y ! E_DIV => risky()`
    // If risky() raises E_TYPE, E_TYPE propagates (not caught by this catch)
    ```"

---

- id: GAP-EXC-015
  feature: "catch expression"
  spec_file: "spec/grammar.md"
  spec_section: "3.1 Try/Except"
  gap_type: assume
  question: |
    Can you use the catch expression syntax in except clauses?

    The grammar shows:
    ```ebnf
    exception_code ::= identifier | "error" | string_literal
    ```

    But the operators.md examples show:
    ```moo
    `obj.prop ! E_PROPNF`
    ```

    Can you write:
    ```moo
    except (``subexpr ! E_TYPE` ! E_RANGE)
    ```

    Or is the exception_codes grammar limited to identifiers/literals?
  impact: |
    Syntactic limitation vs. orthogonality. If catch can be used anywhere
    expressions are allowed, except clauses should allow it. But the grammar
    suggests exception_codes is not a full expression.
  suggested_addition: |
    Clarify in grammar.md section 3.1:
    "The exception_codes in except clauses are limited to the forms shown.
    Full expressions (including catch expressions) are not allowed. To use
    dynamic or computed error codes, use the `@expression` form where expression
    evaluates to a list of error codes."

---

- id: GAP-EXC-016
  feature: "try/except"
  spec_file: "spec/statements.md"
  spec_section: "9.2 Multiple Except Clauses"
  gap_type: test
  question: |
    Can the same error code appear in multiple except clauses?

    Example:
    ```moo
    try
      risky();
    except e (E_TYPE)
      handle_type_error_case_1();
    except e (E_TYPE, E_RANGE)
      handle_type_error_case_2();
    endtry
    ```

    If E_TYPE is raised:
    (a) First matching clause handles it (clause 1)
    (b) All matching clauses execute (both)
    (c) It's a compile error (duplicate case)
    (d) Last matching clause wins (clause 2)
  impact: |
    Determines whether overlapping handlers are allowed. The spec says
    "First matching clause handles the error" which suggests (a), but this
    should be explicit about duplicates.
  suggested_addition: |
    Add to section 9.2:
    "Multiple except clauses may specify overlapping error codes. The first except
    clause (in source order) that matches the raised error handles it. Subsequent
    except clauses are not checked once a match is found.

    Example:
    ```moo
    except (E_TYPE)      // Matches E_TYPE
      handle_specific();
    except (E_TYPE, ANY) // Never reached for E_TYPE
      handle_general();
    endexcept
    ```"

---

- id: GAP-EXC-017
  feature: "error codes"
  spec_file: "spec/errors.md"
  spec_section: "1. Error Code Table"
  gap_type: test
  question: |
    Are error codes comparable for ordering (< > <= >=)?

    The spec shows error codes have numeric values (E_TYPE = 1, E_DIV = 2, etc.)
    but doesn't say whether you can compare them:

    ```moo
    if (error_code < E_RANGE)
      // Is this valid?
    endif
    ```

    Can you:
    - Compare error codes with < > <= >= ?
    - Use error codes in arithmetic (E_TYPE + 1)?
    - Index arrays with error codes?
  impact: |
    Determines whether ERR is a distinct type or just an INT with special meaning.
    If ERR is its own type, comparisons should fail with E_TYPE.
    If ERR is an INT alias, numeric operations work.
  suggested_addition: |
    Add to types.md ERR type section:
    "Error codes are a distinct ERR type. They can be compared for equality (== !=)
    but not for ordering (< > <= >=). Ordering comparisons raise E_TYPE.
    Arithmetic operations on error codes raise E_TYPE.

    Example:
    ```moo
    E_TYPE == E_TYPE     => 1 (valid)
    E_TYPE == 1          => 0 (different types)
    E_TYPE < E_RANGE     => E_TYPE (error: can't compare)
    E_TYPE + 1           => E_TYPE (error: can't do arithmetic)
    ```"

---

- id: GAP-EXC-018
  feature: "error codes"
  spec_file: "spec/errors.md"
  spec_section: "None - missing"
  gap_type: ask
  question: |
    Can you define custom error codes, or are you limited to the 18 standard ones?

    Some languages allow user-defined exceptions. MOO's error codes are predefined,
    but can you:
    - Create new error code constants?
    - Use integers as error codes (raise(42))?
    - Extend the error code enumeration?
  impact: |
    Affects extensibility for application-specific errors. If limited to 18 standard
    errors, some use cases might abuse E_INVARG for everything custom.
  suggested_addition: |
    Add to errors.md:
    "MOO has 18 predefined error codes. User-defined error codes are not supported.
    For application-specific errors, use E_INVARG with a descriptive message."

---

### MEDIUM PRIORITY GAPS (Documentation/Clarity Issues)

---

- id: GAP-EXC-019
  feature: "try/finally"
  spec_file: "spec/statements.md"
  spec_section: "10. Try/Finally Statement"
  gap_type: test
  question: |
    Can you have try/finally without except, and try/except without finally?

    The spec shows:
    - Section 9: try/except (no finally shown)
    - Section 10: try/finally (no except shown)
    - Section 10.1: try/except/finally (both)

    But it's unclear:
    - Is try/finally alone valid?
    - Is try/except alone valid?
    - Must you have both except and finally, or are they independent?
  impact: |
    The organization into separate sections suggests they're independent features,
    but this isn't stated explicitly. An implementor might require both.
  suggested_addition: |
    Add to section 9:
    "Try/except blocks do not require a finally clause. The finally clause is optional."

    Add to section 10:
    "Try/finally blocks do not require except clauses. The except clause is optional."

    Clarify in section 10.1:
    "A try statement may have except clauses, a finally clause, or both. At least
    one of except or finally must be present."

---

- id: GAP-EXC-020
  feature: "catch expression"
  spec_file: "spec/operators.md"
  spec_section: "4. Catch Expression"
  gap_type: test
  question: |
    What is the precedence of the catch expression?

    The precedence table shows catch at level 3, but the syntax uses backticks
    which could be confusing:

    ```moo
    x = `a + b ! E_TYPE` + c
    ```

    Is this:
    (a) x = (`a + b ! E_TYPE`) + c  (catch has high precedence, wraps a+b)
    (b) x = `(a + b ! E_TYPE) + c`  (catch wraps the whole right side)
    (c) Syntax error (backticks not closed)
  impact: |
    The backtick-quote syntax suggests explicit delimiters (like parens), which
    would make precedence irrelevant. But if backticks are just operators, then
    precedence matters.

    The grammar shows: `catch_expr ::= "`" expression "!" exception_codes ["=>" expression] "'"`
    This suggests backtick and quote are delimiters, not operators.
  suggested_addition: |
    Clarify in section 4:
    "Catch expressions use explicit delimiters (backtick to open, single-quote to close).
    The expression between backtick and `!` is evaluated first, then the error codes
    are checked, then the optional default (after `=>`). The entire catch expression
    evaluates to a single value.

    Example:
    ```moo
    x = `a + b ! E_TYPE` + c   // catch wraps (a+b), result added to c
    x = `a + b ! E_TYPE => 0` + c  // catch wraps (a+b), result added to c
    ```"

---

- id: GAP-EXC-021
  feature: "try/except"
  spec_file: "spec/statements.md"
  spec_section: "9.3 Error Codes"
  gap_type: test
  question: |
    What is the "error" literal keyword in the exception_code grammar?

    The grammar shows:
    ```ebnf
    exception_code ::= identifier | "error" | string_literal
    ```

    But the spec doesn't explain what "error" means as an exception code.
    Is it:
    - A synonym for ANY?
    - A specific error code (like E_ERROR)?
    - A literal string "error"?
    - An unused grammar artifact?
  impact: |
    The grammar includes it but no examples use it. An implementor doesn't know
    whether to support it or what it means.
  suggested_addition: |
    Either:
    (a) Remove "error" from the grammar if it's unused
    (b) Document it: "The keyword `error` is a synonym for ANY and catches all errors"
    (c) Document it: "The keyword `error` is deprecated; use ANY instead"

---

- id: GAP-EXC-022
  feature: "try/except"
  spec_file: "spec/statements.md"
  spec_section: "9.3 Error Codes"
  gap_type: test
  question: |
    Can you use string literals as error codes in except clauses?

    The grammar shows:
    ```ebnf
    exception_code ::= identifier | "error" | string_literal
    ```

    But no examples show string literals. What would this mean:
    ```moo
    except ("my_error")
      // What does this catch?
    endtry
    ```

    Is this:
    - A way to catch custom string-based errors?
    - Invalid (grammar artifact)?
    - Equivalent to a variable lookup?
  impact: |
    Grammar suggests feature that isn't documented. Implementor doesn't know
    whether to support it or error on it.
  suggested_addition: |
    Either:
    (a) Remove string_literal from exception_code grammar if unsupported
    (b) Document: "String literals in except clauses are treated as variable names
        and looked up at runtime. Use identifier form instead."

---

- id: GAP-EXC-023
  feature: "try/except"
  spec_file: "spec/statements.md"
  spec_section: "9.3 Error Codes"
  gap_type: test
  question: |
    For `@list_expr` exception codes, what happens if the list contains non-error values?

    Example:
    ```moo
    codes = {E_TYPE, 42, "not an error", #123};
    try
      risky();
    except (@codes)
      // What if E_TYPE is raised? Does 42 prevent matching?
    endtry
    ```

    Does it:
    (a) Filter to only ERR types, ignore others
    (b) Raise E_TYPE when building the catch list
    (c) Match any error if the list contains ANY valid error code
    (d) Require all list elements to be ERR type
  impact: |
    Determines validation semantics. Need to know when/how the list is validated.
  suggested_addition: |
    Add to section 9.3:
    "When using `@list_expr`, the expression must evaluate to a list of error codes
    (ERR type). If the list contains non-error values, E_TYPE is raised when the
    try statement is entered (before the try block executes). Empty lists are valid
    and match no errors."

---

- id: GAP-EXC-024
  feature: "error handling"
  spec_file: "spec/errors.md"
  spec_section: "3.2 Catch Expression"
  gap_type: test
  question: |
    Can you catch the same error that is executing (re-entrant catch)?

    Example:
    ```moo
    function recurse(n)
      `recurse(n - 1) ! E_MAXREC => "base case"`;
    endfunction
    ```

    When the recursion limit is hit:
    (a) E_MAXREC is raised, catch handles it, returns "base case"
    (b) E_MAXREC prevents catch execution (stack unwinding started)
    (c) Catch sees E_MAXREC but re-raises it anyway
  impact: |
    E_MAXREC is special because the stack is in a bad state. Can you catch it?
    What about E_QUOTA or other resource errors?
  suggested_addition: |
    Add to errors.md section 2.10 (E_MAXREC):
    "E_MAXREC can be caught by try/except or catch expressions. When caught,
    the call stack is unwound to the catch site, allowing recovery. However,
    calling another verb from the error handler may immediately raise E_MAXREC
    again if the stack is still near the limit."

---

- id: GAP-EXC-025
  feature: "error handling"
  spec_file: "spec/statements.md"
  spec_section: "9.4 Error Variable"
  gap_type: test
  question: |
    Can you bind the same variable name in multiple except clauses?

    Example:
    ```moo
    try
      risky();
    except e (E_TYPE)
      log(e);
    except e (E_RANGE)
      log(e);
    endtry
    ```

    Is this:
    (a) Valid - same variable name, different scope per except
    (b) Invalid - duplicate variable declaration
    (c) Valid but the variable shadows outer scope
  impact: |
    Determines whether variable names are scoped per except-clause or shared
    across all except clauses in a try block.
  suggested_addition: |
    Add to section 9.4:
    "The error variable can have the same name in different except clauses within
    the same try statement. Each except clause has its own binding for the variable,
    scoped to that except clause only."

---

- id: GAP-EXC-026
  feature: "error handling"
  spec_file: "spec/statements.md"
  spec_section: "9.1 Basic Try/Except"
  gap_type: test
  question: |
    What happens if you have an except clause with no statements?

    Example:
    ```moo
    try
      risky();
    except (E_TYPE)
      // Empty - no statements
    endtry
    ```

    Is this:
    (a) Valid - catches and silently ignores the error
    (b) Invalid - syntax error (empty block)
    (c) Valid but generates a warning
  impact: |
    Common pattern for "ignore this error". Need to know if it's legal.
  suggested_addition: |
    Add to section 9.1:
    "An except clause may have an empty body (no statements). This catches and
    silently ignores the error, allowing execution to continue after the try block.

    Example:
    ```moo
    try
      optional_operation();
    except (E_PROPNF)
      // Ignore if property doesn't exist
    endtry
    ```"

---

- id: GAP-EXC-027
  feature: "catch expression"
  spec_file: "spec/operators.md"
  spec_section: "4. Catch Expression"
  gap_type: test
  question: |
    Can catch expressions appear in statement position, or only as expressions?

    Example:
    ```moo
    `risky_operation() ! E_TYPE`;  // Statement - discard result
    ```

    Is this:
    (a) Valid - catch can be used as statement (like function calls)
    (b) Invalid - catch must be part of larger expression
    (c) Valid but the result must be used
  impact: |
    Affects whether catch is orthogonal (can be used anywhere expressions can)
    or special-cased for value-returning contexts only.
  suggested_addition: |
    Add to section 4:
    "Catch expressions can be used anywhere expressions are allowed, including
    statement position (where the result is discarded).

    Example:
    ```moo
    `risky() ! E_TYPE`;  // Valid - catches error, discards result
    x = `risky() ! E_TYPE => 0`;  // Valid - uses result
    ```"

---

- id: GAP-EXC-028
  feature: "try/finally"
  spec_file: "spec/statements.md"
  spec_section: "13.3 Try/Finally Guarantee"
  gap_type: test
  question: |
    What happens if a finally block contains a return/break/continue?

    The Go implementation shows:
    ```go
    defer func() {
        finallyErr := t.Finally.Execute(vm)
        if err == nil {
            err = finallyErr
        }
    }()
    ```

    This suggests errors from finally can replace try-block errors, but what about
    control flow statements?

    Example:
    ```moo
    for i in [1..10]
      try
        if (i == 5) break;
      finally
        continue;  // Override the break?
      endtry
    endfor
    ```
  impact: |
    Go's defer allows the deferred function to override return values, but
    MOO's control flow is statement-based, not expression-based. Need to define
    what happens when finally has control flow.
  suggested_addition: |
    Add to section 10:
    "If a finally block executes a return, break, or continue statement, that
    control flow statement overrides any return/break/continue from the try or
    except block. This is generally considered bad practice but is allowed.

    Example:
    ```moo
    try
      return 1;
    finally
      return 2;  // Overrides - function returns 2
    endtry
    ```"

---

### LOW PRIORITY GAPS (Minor Clarifications)

---

- id: GAP-EXC-029
  feature: "error codes"
  spec_file: "spec/errors.md"
  spec_section: "1. Error Code Table"
  gap_type: assume
  question: |
    Are the error code names reserved words in the language?

    Can you use E_TYPE as a variable name?
    ```moo
    E_TYPE = 42;  // Valid or syntax error?
    ```

    If error codes are predefined constants, they might be keywords, or they
    might just be variables in a global namespace.
  impact: |
    Affects lexer/parser implementation. If E_TYPE is a keyword, it can't be
    a variable. If it's a global constant, it shadows any local variable.
  suggested_addition: |
    Add to errors.md section 1:
    "Error code names (E_NONE, E_TYPE, etc.) are predefined constants in the
    global namespace. They are not reserved words and can be shadowed by local
    variables, but doing so is strongly discouraged."

---

- id: GAP-EXC-030
  feature: "catch expression"
  spec_file: "spec/operators.md"
  spec_section: "4. Catch Expression"
  gap_type: test
  question: |
    Does the catch expression short-circuit if the expression doesn't raise an error?

    Example:
    ```moo
    x = `safe_value ! E_TYPE => expensive_fallback()`;
    ```

    If safe_value doesn't raise an error:
    (a) expensive_fallback() is NOT evaluated (short-circuit)
    (b) expensive_fallback() IS evaluated anyway
    (c) Depends on optimization level
  impact: |
    Affects whether catch can be used for lazy evaluation. Most likely (a), but
    should be explicit.
  suggested_addition: |
    Add to section 4:
    "Catch expressions short-circuit: the default expression (after `=>`) is only
    evaluated if the main expression raises a matching error. If no error is raised,
    the default is never evaluated.

    Example:
    ```moo
    `safe_value ! E_TYPE => expensive()`  // expensive() not called if safe
    ```"

---

- id: GAP-EXC-031
  feature: "try/except"
  spec_file: "spec/statements.md"
  spec_section: "9.3 Error Codes"
  gap_type: test
  question: |
    Is `ANY` case-sensitive?

    Can you write:
    - `except (any)`
    - `except (Any)`
    - `except (aNy)`

    Or must it be exactly `ANY`?
  impact: |
    Minor, but affects language consistency. Most keywords in MOO are case-sensitive.
  suggested_addition: |
    Add to section 9.3:
    "The `ANY` keyword is case-sensitive and must be written in uppercase. `any`,
    `Any`, etc. are not recognized as error code wildcards and will be treated as
    variable names."

---

- id: GAP-EXC-032
  feature: "error handling"
  spec_file: "spec/errors.md"
  spec_section: "3. Error Handling"
  gap_type: assume
  question: |
    What is the performance cost of try/except blocks?

    Should implementors optimize for:
    (a) The no-error case (zero-cost try/except when no error raised)
    (b) The error case (fast exception handling when errors occur)
    (c) Balanced (moderate cost for both)

    This affects implementation strategy (table-based vs. flag-checking).
  impact: |
    Not critical for correctness, but affects performance expectations.
    If try is "free" (zero-cost), programmers might use it liberally.
    If try is expensive, programmers should avoid it.
  suggested_addition: |
    Add to errors.md section 3:
    "Exception handling is designed for exceptional cases. Try/except blocks
    should be used for error handling, not regular control flow. While the
    performance impact of a try block with no error is minimal, frequent
    exception raising and catching may have performance costs."

---

- id: GAP-EXC-033
  feature: "error messages"
  spec_file: "spec/errors.md"
  spec_section: "6. Error Message Guidelines"
  gap_type: ask
  question: |
    When an error is caught by an except clause, does the error variable contain
    just the error code, or does it include a message, traceback, or other metadata?

    Example:
    ```moo
    try
      risky();
    except e (E_TYPE)
      // What is e? Just E_TYPE, or an error object?
      log(tostr(e));  // What gets logged?
    endtry
    ```

    Is `e`:
    (a) Just the error code (ERR type with value E_TYPE)
    (b) A structured error object {code, message, traceback}
    (c) A string message
    (d) Implementation-defined
  impact: |
    Critical for error inspection and logging. If `e` is just the code, you
    can't get the error message or stack trace. If it's an object, you need
    to know its structure.

    The spec says "variable bound to the error that was raised" which suggests
    the error code itself, but this should be explicit.
  suggested_addition: |
    Add to section 9.4:
    "The error variable is bound to the error code (ERR type) that was raised.
    It does not include error messages, stack traces, or other metadata. To get
    the full error context, use the callers() builtin within the except handler.

    Example:
    ```moo
    try
      risky();
    except e (E_TYPE)
      // e is E_TYPE (the error code)
      // e == E_TYPE is true
      stack = callers();  // Get stack trace if needed
    endtry
    ```"

---

## Summary Statistics

- **Total gaps found:** 33
- **Critical (GUESS/ASK):** 10
- **High priority (TEST/ASSUME):** 8
- **Medium priority:** 10
- **Low priority:** 5

## Recommended Actions

1. **Immediate:** Document the raise() builtin (GAP-EXC-003)
2. **High priority:** Clarify error variable scope (GAP-EXC-001)
3. **High priority:** Define finally block error semantics (GAP-EXC-006)
4. **High priority:** Document catch expression matching (GAP-EXC-004, GAP-EXC-005)
5. **Medium priority:** Add nested try/except section (GAP-EXC-012)
6. **Polish:** Review grammar for unused productions (GAP-EXC-021, GAP-EXC-022)

## Conformance Test Implications

To cover these gaps, conformance tests should include:

1. Error variable scoping across except blocks
2. Finally execution with return/break/continue
3. Nested try/except propagation
4. Catch expression with/without defaults
5. Multiple except clauses with overlapping codes
6. Exception during finally block
7. Empty except bodies
8. Dynamic error code lists (@list_var)
9. Error code list validation
10. Re-raising errors (via raise() builtin)

**Estimated test coverage needed:** 40+ tests for comprehensive exception handling coverage.
