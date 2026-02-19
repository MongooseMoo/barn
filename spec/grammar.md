# MOO Language Grammar Specification

## Overview

This document defines the formal grammar of the MOO programming language as implemented in ToastStunt. The grammar is presented in Extended Backus-Naur Form (EBNF).

**Source:** Derived from `moo_interp/parser.lark` with ToastStunt extensions.

---

## Notation

```
::=     Definition
|       Alternative
[ ]     Optional (0 or 1)
{ }     Repetition (0 or more)
( )     Grouping
" "     Terminal (literal)
' '     Terminal (literal, alternative)
/**/    Comment
```

---

## 1. Program Structure

```ebnf
program         ::= { statement }

statement       ::= single_statement
                  | if_statement
                  | for_statement
                  | while_statement
                  | fork_statement
                  | try_except_statement
                  | try_finally_statement

single_statement ::= [ expression | return_statement | break_statement | continue_statement ] ";"

body            ::= { statement }
```

---

## 2. Control Flow Statements

### 2.1 Conditional (if/elseif/else)

```ebnf
if_statement    ::= if_clause { elseif_clause } [ else_clause ] "endif"

if_clause       ::= "if" "(" expression ")" body
elseif_clause   ::= "elseif" "(" expression ")" body
else_clause     ::= "else" body
```

### 2.2 For Loop

```ebnf
for_statement   ::= for_clause body "endfor"

for_clause      ::= "for" identifier [ "," identifier ] "in" "(" expression ")"
                  | "for" identifier [ "," identifier ] "in" "[" expression ".." range_end "]"

range_end       ::= expression | "$"
```

**Semantics:**
- First form: iterate over list elements
- Second form: iterate over numeric range (inclusive)
- Optional second identifier captures index (lists) or key (maps)

### 2.3 While Loop

```ebnf
while_statement ::= while_clause body "endwhile"

while_clause    ::= "while" [ identifier ] "(" expression ")"
```

**Semantics:**
- Optional identifier names the loop for break/continue targeting

### 2.4 Fork Statement

```ebnf
fork_statement  ::= fork_clause { statement } "endfork"

fork_clause     ::= "fork" [ identifier ] "(" expression ")"
```

**Semantics:**
- Expression is delay in seconds (int or float)
- Optional identifier receives task ID
- Body executes asynchronously after delay

### 2.5 Return/Break/Continue

```ebnf
return_statement   ::= "return" [ expression ]
break_statement    ::= "break" [ identifier ]
continue_statement ::= "continue" [ identifier ]
```

**Semantics:**
- Optional identifier targets named loop

---

## 3. Exception Handling

### 3.1 Try/Except

```ebnf
try_except_statement ::= "try" { statement } { except_clause } "endtry"

except_clause   ::= "except" [ identifier ] "(" exception_codes ")" { statement }

exception_codes ::= "any"
                  | "@" expression
                  | exception_code { "," exception_code }

exception_code  ::= identifier          /* E_TYPE, E_RANGE, etc. */
                  | "error"             /* literal keyword */
                  | string_literal
```

**Semantics:**
- `any` catches all errors
- `@expr` evaluates to list of error codes
- Optional identifier binds the caught error

### 3.2 Try/Finally

```ebnf
try_finally_statement ::= "try" { statement } finally_clause "endtry"

finally_clause  ::= "finally" { statement }
```

**Semantics:**
- Finally block always executes (normal exit, error, or return)

---

## 4. Expression Precedence

Expressions are listed from **lowest to highest** precedence:

| Level | Operator(s) | Associativity | Description |
|-------|-------------|---------------|-------------|
| 1 | `=` | Right | Assignment |
| 2 | `? \|` | Right | Ternary conditional |
| 3 | `` ` ! ' `` | N/A | Catch expression |
| 4 | `@` | Right | Splice |
| 5 | `{ } =` | Right | Scatter assignment |
| 6 | `\|\|` | Left | Logical OR |
| 7 | `&&` | Left | Logical AND |
| 8 | `\|.` | Left | Bitwise OR |
| 9 | `^.` | Left | Bitwise XOR |
| 10 | `&.` | Left | Bitwise AND |
| 11 | `== != < <= > >= in` | Left | Comparison |
| 12 | `<< >>` | Left | Bit shift |
| 13 | `+ -` | Left | Addition/Subtraction |
| 14 | `* / %` | Left | Multiplication/Division/Modulo |
| 15 | `^` | Right | Exponentiation |
| 16 | `! ~ -` | Right | Unary (not, bitwise not, negate) |
| 17 | `. : [ ]` | Left | Postfix (property, verb, index) |

---

## 5. Expression Grammar

```ebnf
expression      ::= assignment
                  | ternary
                  | catch_expr
                  | splicer
                  | scatter
                  | logical_or

/* Level 1: Assignment (right-associative) */
assignment      ::= postfix "=" expression

/* Level 2: Ternary (right-associative) */
ternary         ::= logical_or "?" expression "|" expression

/* Level 3: Catch expression */
catch_expr      ::= "`" expression "!" exception_codes [ "=>" expression ] "'"

/* Level 4: Splice */
splicer         ::= "@" expression

/* Level 5: Scatter assignment */
scatter         ::= "{" scattering_target "}" "=" expression

scattering_target ::= scatter_item { "," scatter_item }
scatter_item    ::= identifier
                  | "?" identifier [ "=" expression ]
                  | "@" identifier

/* Level 6: Logical OR (left-associative, short-circuit) */
logical_or      ::= logical_and { "||" logical_and }

/* Level 7: Logical AND (left-associative, short-circuit) */
logical_and     ::= bitwise_or { "&&" bitwise_or }

/* Level 8: Bitwise OR (left-associative) */
bitwise_or      ::= bitwise_xor { "|." bitwise_xor }

/* Level 9: Bitwise XOR (left-associative) */
bitwise_xor     ::= bitwise_and { "^." bitwise_and }

/* Level 10: Bitwise AND (left-associative) */
bitwise_and     ::= comparison { "&." comparison }

/* Level 11: Comparison (left-associative) */
comparison      ::= shift { comparison_op shift }
comparison_op   ::= "==" | "!=" | "<" | "<=" | ">" | ">=" | "in"

/* Level 12: Shift (left-associative) */
shift           ::= additive { shift_op additive }
shift_op        ::= "<<" | ">>"

/* Level 13: Additive (left-associative) */
additive        ::= multiplicative { add_op multiplicative }
add_op          ::= "+" | "-"

/* Level 14: Multiplicative (left-associative) */
multiplicative  ::= power { mul_op power }
mul_op          ::= "*" | "/" | "%"

/* Level 15: Power (right-associative) */
power           ::= unary [ "^" power ]

/* Level 16: Unary (right-associative) */
unary           ::= unary_op unary
                  | postfix
unary_op        ::= "!" | "~" | "-"

/* Level 17: Postfix (left-associative) */
postfix         ::= atom { postfix_op }
postfix_op      ::= "." property_name                           /* property access */
                  | ".:" identifier                              /* waif property */
                  | "[" index_expr "]"                           /* indexing */
                  | "[" range_start ".." range_end "]"           /* range */
                  | ":" verb_name "(" [ arguments ] ")"          /* verb call */

property_name   ::= identifier | "(" expression ")"
verb_name       ::= identifier | "(" expression ")"
```

---

## 6. Atoms (Primary Expressions)

```ebnf
atom            ::= "(" expression ")"
                  | catch_expr
                  | literal
                  | function_call
                  | dollar_property
                  | dollar_verb_call
                  | dollar
                  | identifier

function_call   ::= identifier "(" [ arguments ] ")"

dollar_property ::= "$" identifier                    /* equivalent to #0.identifier */

dollar_verb_call ::= "$" verb_name "(" [ arguments ] ")"  /* equivalent to #0:verb() */

dollar          ::= "$"                               /* last index marker in ranges */

arguments       ::= expression { "," expression }
```

---

## 7. Indexing and Ranges

```ebnf
index_expr      ::= "^"                               /* first element (index 1) */
                  | "$"                               /* last element */
                  | expression

range_start     ::= "^"                               /* first element */
                  | expression

range_end       ::= "$"                               /* last element */
                  | expression
```

**Semantics:**
- `^` represents index 1 (first element)
- `$` represents `length(collection)` (last element)
- Ranges are inclusive on both ends
- 1-based indexing throughout

---

## 8. Literals

```ebnf
literal         ::= integer_literal
                  | float_literal
                  | string_literal
                  | object_literal
                  | boolean_literal
                  | list_literal
                  | map_literal
                  | error_literal

integer_literal ::= [ "-" ] digit { digit }

float_literal   ::= [ "-" ] { digit } "." { digit } [ exponent ]
                  | [ "-" ] digit { digit } exponent
exponent        ::= ( "e" | "E" ) [ "+" | "-" ] digit { digit }

string_literal  ::= '"' { string_char } '"'
string_char     ::= any_char_except_quote_or_backslash
                  | escape_sequence
escape_sequence ::= "\\" | '\"' | "\n" | "\t" | "\r"
                  | "\x" hex_digit hex_digit

object_literal  ::= "#" integer_literal

boolean_literal ::= "true" | "false"

list_literal    ::= "{" [ expression { "," expression } ] "}"

map_literal     ::= "[" [ map_entry { "," map_entry } ] "]"
map_entry       ::= expression "->" expression

error_literal   ::= "error"                           /* raw error keyword */
```

---

## 9. Lexical Elements

### 9.1 Identifiers

```ebnf
identifier      ::= ( letter | "_" ) { letter | digit | "_" }
letter          ::= "a".."z" | "A".."Z"
digit           ::= "0".."9"
hex_digit       ::= digit | "a".."f" | "A".."F"
```

**Reserved words:**
```
if elseif else endif
for endfor in
while endwhile
fork endfork
try except finally endtry
return break continue
any error true false
```

### 9.2 Comments

```ebnf
comment         ::= block_comment | line_comment
block_comment   ::= "/*" { any_char } "*/"
line_comment    ::= "//" { any_char_except_newline } newline
```

### 9.3 Whitespace

```ebnf
whitespace      ::= " " | "\t" | "\n" | "\r" | "\f"
```

Whitespace is ignored except as token separator.

---

## 10. Operator Tokens

| Token | Name | Category |
|-------|------|----------|
| `=` | Assign | Assignment |
| `?` | Question | Ternary |
| `\|` | Pipe | Ternary |
| `` ` `` | Backtick | Catch open |
| `'` | Quote | Catch close |
| `!` | Bang | Catch/Unary |
| `=>` | Arrow | Catch default |
| `@` | At | Splice/Scatter |
| `\|\|` | Or | Logical |
| `&&` | And | Logical |
| `\|.` | BitOr | Bitwise |
| `^.` | BitXor | Bitwise |
| `&.` | BitAnd | Bitwise |
| `==` | Equal | Comparison |
| `!=` | NotEqual | Comparison |
| `<` | Less | Comparison |
| `<=` | LessEq | Comparison |
| `>` | Greater | Comparison |
| `>=` | GreaterEq | Comparison |
| `in` | In | Comparison |
| `<<` | ShiftLeft | Shift |
| `>>` | ShiftRight | Shift |
| `+` | Plus | Additive |
| `-` | Minus | Additive/Unary |
| `*` | Star | Multiplicative |
| `/` | Slash | Multiplicative |
| `%` | Percent | Multiplicative |
| `^` | Caret | Power/First |
| `~` | Tilde | Unary |
| `.` | Dot | Property |
| `.:` | DotColon | Waif property |
| `:` | Colon | Verb call |
| `[` | LBracket | Index/Range |
| `]` | RBracket | Index/Range |
| `..` | DotDot | Range |
| `{` | LBrace | List/Scatter |
| `}` | RBrace | List/Scatter |
| `(` | LParen | Grouping |
| `)` | RParen | Grouping |
| `,` | Comma | Separator |
| `;` | Semi | Statement end |
| `->` | MapArrow | Map entry |
| `#` | Hash | Object |
| `$` | Dollar | System/Last |

---

## 11. Semantic Notes

### 11.1 Short-Circuit Evaluation

- `||` evaluates right operand only if left is false
- `&&` evaluates right operand only if left is true

### 11.2 Catch Expression

The catch expression `` `expr ! codes => default' `` evaluates `expr`. If it raises an error matching `codes`, returns `default` (or the error if no default). Otherwise returns `expr` result.

### 11.3 Scatter Assignment

```moo
{a, ?b = 0, @rest} = {1, 2, 3, 4};
// a = 1, b = 2, rest = {3, 4}
```

- Required: `a` must have a value
- Optional: `?b` uses default if missing
- Rest: `@rest` collects remaining elements

### 11.4 Dollar Notation

- `$prop` is equivalent to `#0.prop`
- `$verb(args)` is equivalent to `#0:verb(args)`
- `$` in index context means "last element"

---

## 12. Grammar Validation

This grammar should parse all valid MOO code in the conformance test suite. Validation checkpoint:

```bash
# Run parser tests
go build -o barn.exe ./cmd/barn/
uv run --project ..\moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}" -k moocode_parsing
```

All 62 parsing tests must pass.
