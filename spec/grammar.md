# MOO Language Grammar Specification

## Overview

This document defines the formal grammar of the MOO programming language as implemented in ToastStunt. The grammar is presented in Extended Backus-Naur Form (EBNF).

**Source:** ToastStunt `src/parser.y` (grammar, precedence declarations, and the
hand-written lexer `yylex`) and `src/keywords.gperf`. Line references below are
to upstream `lisdude/toaststunt` master.

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

statement       ::= if_statement
                  | for_statement
                  | while_statement
                  | fork_statement
                  | try_statement
                  | expression ";"
                  | "break" [ identifier ] ";"
                  | "continue" [ identifier ] ";"
                  | "return" [ expression ] ";"
                  | ";"

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

**Semantic lowering:** `elseif` is concrete MOO syntax. Each clause lowers to a
semantic conditional nested in the else-body of the preceding conditional; it
does not create an `elseif` semantic type.

### 2.2 For Loop

```ebnf
for_statement   ::= for_clause body "endfor"

for_clause      ::= "for" identifier [ "," identifier ] "in" "(" expression ")"
                  | "for" identifier "in" "[" expression ".." expression "]"
```

**Semantics:**
- First form: iterate over list elements
- Second form: iterate over numeric range (inclusive)
- The optional second identifier belongs only to collection iteration and
  captures a list index or map key
- The range bounds are ordinary expressions; `^` and `$` are not legal there
  (see Section 7)

**Semantic lowering:** The collection and range productions lower to distinct
semantic loop variants. No semantic loop selects its form through nullable
fields.

### 2.3 While Loop

```ebnf
while_statement ::= while_clause body "endwhile"

while_clause    ::= "while" [ identifier ] "(" expression ")"
```

**Semantics:**
- Optional identifier names the loop for break/continue targeting

### 2.4 Fork Statement

```ebnf
fork_statement  ::= fork_clause body "endfork"

fork_clause     ::= "fork" [ identifier ] "(" expression ")"
```

**Semantics:**
- Expression is delay in seconds (int or float)
- Optional identifier receives task ID
- Body executes asynchronously after delay
- Loops enclosing the fork are not visible inside its body: `break` or
  `continue` there that would target them is a compile error

### 2.5 Return/Break/Continue

**Semantics:**
- The optional identifier on `break`/`continue` names an enclosing loop. For a
  `for` loop the loop variable (and the index variable, if present) names the
  loop; a `while` loop is named by its optional identifier.

---

## 3. Exception Handling

### 3.1 Try Statement

```ebnf
try_statement   ::= "try" body except_clause { except_clause } "endtry"
                  | "try" body "finally" body "endtry"

except_clause   ::= "except" [ identifier ] "(" codes ")" body

codes           ::= "ANY"
                  | arg_list
```

There is no form that combines `except` clauses with a `finally` clause:
`try ... except (...) ... finally ... endtry` is a syntax error. To get both,
nest a try/except inside a try/finally (`parser.y` try productions; conformance
`language/parser_try_except_finally.yaml`).

**Semantics:**
- `ANY` catches every error. It must stand alone: `ANY` is a keyword, not an
  expression, so `(E_INVARG, ANY)` or `@ANY` is a syntax error.
- Otherwise the codes are an ordinary argument list (Section 6): any
  expressions, with `@` splices. They are evaluated when the `try` statement is
  entered and the raised value is matched against the resulting list. Error
  literals are the usual codes, but any value is a valid code: `except
  ("E_CUSTOM")` catches `raise("E_CUSTOM")`, and `` `raise(1) ! 1 => 2' ``
  returns 2 (conformance `language/expression_codes_and_defaults.yaml`).
- An optional identifier binds the caught error
- A clause after an `ANY` clause is a compile error ("Unreachable EXCEPT
  clause"); more than 255 clauses is a compile error ("Too many EXCEPT clauses
  (max. 255)").
- A finally block always executes on normal exit, error, or return

**Semantic lowering:** Both concrete forms lower to one semantic try statement
containing an ordered handler list and an optional finalizer. At least one
handler or finalizer is required.

---

## 4. Expression Precedence

Expressions are listed from **lowest to highest** precedence. This is the
`%right`/`%nonassoc`/`%left` declaration block of `parser.y` (lines 104-113):

| Level | Operator(s) | Associativity | Description |
|-------|-------------|---------------|-------------|
| 1 | `=` | Right | Assignment (including scatter assignment) |
| 2 | `? \|` | Non-associative | Ternary conditional |
| 3 | `\|\| &&` | Left | Logical OR and AND (one shared level) |
| 4 | `== != < <= > >= in` | Left | Comparison |
| 5 | `\|. &. ^.` | Left | Bitwise OR, AND, XOR (one shared level) |
| 6 | `<< >>` | Left | Bit shift |
| 7 | `+ -` | Left | Addition/Subtraction |
| 8 | `* / %` | Left | Multiplication/Division/Modulo |
| 9 | `^` | Right | Exponentiation |
| 10 | `! ~ -` | Right (prefix) | Unary (not, bitwise not, negate) |
| 11 | `. : [ ]` | Left | Postfix (property, verb, index, range) |

Consequences:
- `||` and `&&` share one left-associative level, so `a || b && c` is
  `(a || b) && c` and `1 || 0 && 0` is `0` (conformance
  `language/moocode_parsing.yaml`, `precedence_or_and_share_one_left_associative_level`).
- Bitwise operators bind tighter than comparisons: `x == y |. z` is
  `x == (y |. z)`. The three bitwise operators share one level, so
  `a |. b &. c` is `(a |. b) &. c`.
- Unary operators bind tighter than `^`: `-2 ^ 2` is `(-2) ^ 2`.

The catch expression and the splice prefix are not precedence levels. A catch
expression is delimited by its backquote and quote and is an atom. `@` is only
a prefix on an argument-list entry (Section 6).

---

## 5. Expression Grammar

```ebnf
expression      ::= assignment
                  | scatter_assignment
                  | conditional
                  | binary
                  | unary
                  | postfix

assignment      ::= lvalue "=" expression
lvalue          ::= identifier
                  | postfix "." property_name
                  | lvalue "[" expression "]"
                  | lvalue "[" expression ".." expression "]"

scatter_assignment ::= "{" arg_list "}" "=" expression        /* each entry an identifier or @identifier */
                     | "{" scatter "}" "=" expression          /* contains at least one ?target */

scatter         ::= scatter_item { "," scatter_item }
scatter_item    ::= identifier
                  | "?" identifier [ "=" expression ]
                  | "@" identifier

conditional     ::= expression "?" expression "|" expression

binary          ::= expression binary_op expression
binary_op       ::= "||" | "&&"
                  | "==" | "!=" | "<" | "<=" | ">" | ">=" | "in"
                  | "|." | "&." | "^."
                  | "<<" | ">>"
                  | "+" | "-" | "*" | "/" | "%" | "^"

unary           ::= ( "!" | "~" | "-" ) expression

postfix         ::= atom
                  | postfix "." property_name                     /* property access */
                  | postfix "." ":" identifier                    /* waif property, same as .(":name") */
                  | postfix "[" expression "]"                    /* indexing */
                  | postfix "[" expression ".." expression "]"    /* range */
                  | postfix ":" verb_name "(" arg_list ")"        /* verb call */

property_name   ::= identifier | "(" expression ")"
verb_name       ::= identifier | "(" expression ")"
```

The operator productions are disambiguated by Section 4.

**Assignment targets.** The left side of `=` must be a variable, a property
reference, or an index/range chain ending in one of those. Anything else is a
compile error ("Illegal expression on left side of assignment."), for example
`1 = 2` or `{}[1] = 2`.

**Scatter assignment** is an expression, like ordinary assignment; its value is
the right-hand side. It can be parenthesized and used as an operand or a loop
condition: `v = ({a, @b} = {1, 2, 3})` sets `v` to `{1, 2, 3}`, and
`while ({?x, @todo} = todo)` drains a list (conformance
`language/scatter.yaml`, `scatter_expression_*`). A brace list followed by `=`
is a scatter only when every entry is a plain identifier or `@identifier`;
otherwise it is a compile error ("Scattering assignment targets must be simple
variables."). More than one `@` target ("More than one `@' target in
scattering assignment.") and `{} = ...` ("Empty list in scattering
assignment.") are compile errors.

Because the `{...}` form with only required and rest targets is parsed as a
list expression before the `=` is seen, `0 || {a, b} = x` groups as
`(0 || {a, b}) = x` (an illegal assignment), while a target list containing a
`?` target is its own production, so `0 || {?a, b} = {2}` is
`0 || ({?a, b} = {2})` and yields `{2}`.

**Semantic lowering:** every assignment form constructs one sealed target.
Scatter constructs the destructuring target rather than a second statement or
target family.

---

## 6. Atoms (Primary Expressions)

```ebnf
atom            ::= "(" expression ")"
                  | catch_expr
                  | literal
                  | list_literal
                  | map_literal
                  | function_call
                  | "$" identifier                                 /* #0.identifier */
                  | "$" identifier "(" arg_list ")"               /* #0:identifier(...) */
                  | "^"                                           /* first, index context only */
                  | "$"                                           /* last, index context only */
                  | identifier

catch_expr      ::= "`" expression "!" codes [ "=>" expression ] "'"

function_call   ::= identifier "(" arg_list ")"

arg_list        ::= [ arg { "," arg } ]
arg             ::= expression
                  | "@" expression

list_literal    ::= "{" arg_list "}"

map_literal     ::= "[" "]"
                  | "[" map_entry { "," map_entry } "]"
map_entry       ::= expression "->" expression
```

**Argument lists** are the only place `@` may appear. `arg_list` is shared by
list literals, builtin and verb call arguments, and except/catch codes. The
spliced operand is any expression, including an assignment: `{@ret = {1, 2}}`
sets `ret` and evaluates to `{1, 2}`. A splice is never an operand of a binary
operator: `{1/@{}}` is a syntax error (conformance
`language/parser_splice_operand.yaml`).

**No trailing commas.** A list literal, map literal, argument list, or scatter
target list cannot end with a comma: `{1, 2,}`, `{@a,}`, `["a" -> 1,]`, and
`{a, b,} = ...` are syntax errors (conformance
`language/parser_trailing_comma.yaml`).

**No ranges in list literals.** A list entry is an expression, not a range:
`{0..0}` is a syntax error. `{0, 1}[1..2]` is an ordinary range on a list
literal (conformance `language/parser_list_range.yaml`).

**Catch expression.** The codes follow the same rules as `except` codes
(Section 3.1), including lower-case or mixed-case `any`. The default after
`=>` is any expression, including an assignment: `` `1 / 0 ! ANY => x = 5' ``
assigns 5 to `x` and evaluates to 5.

**Builtin calls.** `name(args)` where `name` is not a builtin function is a
compile error in `set_verb_code()` and `eval()` ("Unknown built-in function:
name"). When the server loads a database it compiles such a call instead as
`call_function("name", @args)` and logs a warning (`parser.y`, the
`tID '(' arglist ')'` production; database.md covers loading).

---

## 7. Indexing and Ranges

`^` and `$` are legal only inside the brackets of an index or range expression,
at any depth within the bracketed expressions (`x[^ + 1..$]`). Anywhere else,
including a `for` range, they are a compile error ("Illegal context for `^'
expression." / "Illegal context for `$' expression.").

**Semantics:**
- `^` is the first index of the indexed value: `1` for a non-empty list or
  string, `0` for an empty one, and the first key of a map
- `$` is the last index: the length of a list or string, the last key of a map
- Ranges are inclusive on both ends
- 1-based indexing throughout

See types.md Section 5 for range bounds.

**Semantic lowering:** `^` and `$` lower to language-neutral first and last
boundary expressions. The concrete token spelling remains parser-owned.

---

## 8. Literals

```ebnf
literal         ::= integer_literal
                  | float_literal
                  | string_literal
                  | object_literal
                  | error_literal

integer_literal ::= digit { digit }

float_literal   ::= digit { digit } "." { digit } [ exponent ]   /* "1." only when not followed by "." */
                  | "." digit { digit } [ exponent ]
                  | digit { digit } exponent
exponent        ::= ( "e" | "E" ) [ "+" | "-" ] digit { digit }

string_literal  ::= '"' { string_char } '"'
string_char     ::= any_char_except_quote_backslash_newline
                  | "\" any_char_except_newline

object_literal  ::= "#" [ "-" ] digit { digit }

error_literal   ::= error_name                    /* E_NONE ... E_INTRPT, any letter case */
```

**Numbers.** Literals carry no sign; `-` is the unary operator. Negation of a
numeric literal is folded into the literal at compile time. An exponent with no
digits is a compile error ("Malformed floating-point literal"), as is a float
literal outside the finite double range ("Floating-point literal out of
range"; for example `1e400`).

**Float precision.** A float literal is compiled to the full double that
`strtod` reads from its spelling. Decompiled code (`verb_code()`) prints floats
with 15 significant digits, but the stored program keeps the exact value:
after `set_verb_code(o, "v", {"return 18.46744073709553614;"})`,
`o:v() == 18.46744073709553614` is true while `verb_code(o, "v")` shows
`return 18.4674407370955;` (conformance
`builtins/set_verb_code_float_literal_precision.yaml`).

**Strings.** A backslash makes the next character literal; there are no other
escape sequences (`"\n"` is the one-character string `"n"`). A string cannot
contain a newline: a literal that reaches the end of its source line, including
one whose line ends in a backslash, is a compile error ("Missing quote")
(`parser.y` yylex; conformance `language/parser_string_lines.yaml`).

**Objects.** `#` must be followed by digits, optionally after `-`; otherwise
the compile error is "Malformed object number".

**Booleans.** `true` and `false` are not literals or keywords. They are
predefined variables (see types.md Section 6.2) initialized to the Boolean
values, so `x = true = {}` compiles and assigns `{}` to both.

---

## 9. Lexical Elements

### 9.1 Identifiers and Keywords

```ebnf
identifier      ::= ( letter | "_" ) { letter | digit | "_" }
letter          ::= "a".."z" | "A".."Z"
digit           ::= "0".."9"
```

The lexer reads the whole identifier and then looks it up in the keyword table
(`keywords.gperf`), **ignoring letter case** (it is generated with
`gperf --ignore-case`). A match is a keyword; anything else is an identifier.

**Keywords (any letter case):**
```
if else elseif endif
for in endfor
fork endfork
return
while endwhile
try except finally endtry
ANY
break continue
E_NONE E_TYPE E_DIV E_PERM E_PROPNF E_VERBNF E_VARNF E_INVIND E_RECMOVE
E_MAXREC E_RANGE E_ARGS E_NACC E_INVARG E_QUOTA E_FLOAT E_FILE E_EXEC E_INTRPT
```

Consequences (conformance `language/keyword_case.yaml`,
`language/error_name_case.yaml`):
- `RETURN 5;`, `IF (1) ... ENDIF`, `For x In (...)` and `EXCEPT (Any)` compile.
- A keyword in any case cannot be a variable, property name, or verb name:
  `ANY = 3`, `x.IN`, and `x:ANY()` are syntax errors. `["ANY" -> 1]` is an
  ordinary string key.
- Error names match only as whole identifiers: `e_type` and `E_Type` are the
  error value `E_TYPE` and cannot be assigned, while `E_IN3ARG`, `E_TYPEx`,
  `E_TYPE3`, and `E_TYPE_` are ordinary variables.

Variable names are also case-insensitive: `Foo` and `foo` name the same
variable (`sym_table.cc` `find_name` uses `strcasecmp`).

### 9.2 Comments

```ebnf
comment         ::= "/*" { any_char } "*/"
```

MOO has only block comments, and they are whitespace between tokens:
- The first `*/` ends the comment; comments do not nest. `/* outer /* inner */
  return 7;` is a comment followed by `return 7;`, while `/* a /* b */ c */`
  leaves `c */` as code.
- Comments may span lines and appear between any two tokens
  (`18 /* x */ / /* y */ 3` is `18 / 3`).
- `/*` and `//` inside a string literal are string text.
- There is no `//` line comment; `// one` is a syntax error.
- A comment still open at end of program is a compile error reported on the
  line where the program ended: "Line N:  End of program while in a comment".

(`parser.y` yylex lines 889-907; conformance
`language/parser_comment_contract.yaml`.)

### 9.3 Whitespace

Whitespace (`isspace`) is ignored except as a token separator.

---

## 10. Operator Tokens

| Token | Name | Category |
|-------|------|----------|
| `=` | Assign | Assignment |
| `?` | Question | Ternary / optional scatter target |
| `\|` | Pipe | Ternary |
| `` ` `` | Backtick | Catch open |
| `'` | Quote | Catch close |
| `!` | Bang | Catch/Unary |
| `=>` | Arrow | Catch default |
| `@` | At | Splice/Scatter rest |
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
| `:` | Colon | Verb call |
| `[` | LBracket | Index/Range/Map |
| `]` | RBracket | Index/Range/Map |
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

**Lexing `^`:** `^` followed by `..` is the caret token; `^` followed by any
other `.` is the `^.` XOR token. So `x[^.y]` lexes as `x[` `^.` `y` `]` (a syntax
error); write `(^).y` or `^ .y`. Likewise `|.` and `&.` are taken whenever `|`
or `&` is followed by `.` (`parser.y` yylex operator switch).

**Waif property:** `.:` is two tokens; `obj.:name` is `obj.(":name")`.

---

## 11. Semantic Notes

### 11.1 Short-Circuit Evaluation

- `||` evaluates right operand only if left is false
- `&&` evaluates right operand only if left is true

### 11.2 Catch Expression

The catch expression `` `expr ! codes => default' `` evaluates `expr`. If it raises an error matching `codes`, returns `default` (or the error code if no default). Otherwise returns `expr` result.

### 11.3 Scatter Assignment

```moo
{a, ?b = 0, @rest} = {1, 2, 3, 4};
// a = 1, b = 2, rest = {3, 4}
```

- Required: `a` must have a value
- Optional: `?b` takes a spare value if there is one; otherwise its default is
  evaluated, and without a default the variable is left unchanged
- Rest: `@rest` collects remaining elements

See statements.md Section 11 for binding order and errors.

### 11.4 Dollar Notation

- `$prop` is equivalent to `#0.("prop")`
- `$verb(args)` is equivalent to `#0:("verb")(args)`
- `$` in index context means "last index"

---

## 12. Compilation Limits and Diagnostics

**Depth.** Programs nest at least 256 constructs deep: 256 nested unary
operators, parentheses, list literals, map values, `^` chains, assignments,
postfix indexes, `if`/`while`/`try` statements, and combinations of these all
compile (conformance `language/parser_depth_boundary.yaml`). Parentheses around
an operand do not add a separate level: `1 + (1 + (... (1 + 1)))` with 255
parenthesized operands compiles.

**Literals.** There is no limit on the number of distinct literals in one
program; programs with 257 and 600 distinct string and float literals compile
and each literal keeps its value (conformance
`language/literal_count_boundary.yaml`).

**Diagnostics.** Compile errors are strings of the form `Line N:  message` (two
spaces after the colon), returned by `set_verb_code()` and as the second
element of a failed `eval()`. A failed `set_verb_code()` leaves the verb's
previous program in place. A syntax error stops compilation and is the last
message: `{"a = ;", "break;"}` reports only `Line 1:  syntax error`. Semantic
errors found while parsing do not stop it, so several can be reported in source
order, and a syntax error after them is reported too (`{"break;", "a = ;"}`
reports the `break` error and then `Line 2:  syntax error`). Messages include:

| Message | Cause |
|---------|-------|
| `syntax error` | Grammar violation |
| ``No enclosing loop for `break' statement`` | `break` outside a loop |
| ``No enclosing loop for `continue' statement`` | `continue` outside a loop |
| ``Invalid loop name in `break' statement: NAME`` | Named `break` with no such loop |
| ``Invalid loop name in `continue' statement: NAME`` | Named `continue` with no such loop |
| `Unknown built-in function: NAME` | Call of a nonexistent builtin |
| `Floating-point literal out of range` | Float literal not finite |
| `Malformed floating-point literal` | Exponent without digits |
| `Malformed object number` | `#` not followed by digits |
| `Missing quote` | String literal reaches end of line |
| `End of program while in a comment` | Unterminated `/*` |
| ``Illegal context for `^' expression.`` | `^` outside index brackets |
| ``Illegal context for `$' expression.`` | `$` outside index brackets |
| `Illegal expression on left side of assignment.` | Bad assignment target |
| `Scattering assignment targets must be simple variables.` | Non-variable scatter target |
| ``More than one `@' target in scattering assignment.`` | Two rest targets |
| `Empty list in scattering assignment.` | `{} = ...` |
| `Unreachable EXCEPT clause` | Clause after an `ANY` clause |

(Conformance `language/parser_independent_diagnostics.yaml`,
`language/parser_comment_contract.yaml`, `language/parser_string_lines.yaml`.)

---

## 13. Grammar Validation

This grammar should parse all valid MOO code in the conformance test suite. Validation checkpoint:

```bash
# Run parser tests
go build -o barn.exe ./cmd/barn/
uv run --project ..\moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe serve --db {db} --port {port}" -k moocode_parsing
```

All 62 parsing tests must pass.
