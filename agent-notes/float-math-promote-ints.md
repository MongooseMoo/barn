# Float math builtins accept integers under PROMOTE_NUMBERS

## Goal

Under `--promote-numbers` / `PROMOTE_NUMBERS = 1`, the float math builtins
accept INT arguments and convert them to floats, as the Mongoose ToastStunt
fork does (`mongoosemoo/mongoose`, `src/numbers.cc`, builtin float promotion).
Without the option, behaviour stays as it is. Nothing else.

## Fork behaviour (read-only, `src/numbers.cc` on `mongoosemoo/mongoose`)

- Compile-time `PROMOTE_NUMBERS` (`src/include/options.h:502`).
- Promoting: sqrt cbrt sin cos tan asin acos sinh cosh tanh acosh asinh atanh
  exp log log10 ceil floor (promoting `MATH_FUNC`, 900-926), atan (both args,
  995-1048), atan2 (both, 1050-), trunc (970-993), round (1248-1262).
  Registered `TYPE_NUMERIC` (1421-1481), so `function_info` reports -2.
- Not promoting: frandom, floatstr, relative_heading.
- An INT goes through the same code as the equal FLOAT, so the result and the
  error mapping are those of the float.

## State

- Conformance candidate test
  `tests/mongoose-conformance/builtins/float_math_integer_promotion.yaml`:
  strict half passes on stock WSL Toast; promote half skips there.
- Toast-first proof of the promote half is missing: the managed Mongoose
  Toast (`profiles/toast/mongoose-wsl-mongoose.json`, ref 72e3c7f9) predates
  the fork's promotion commits, and its manifest is for the private Mongoose
  database, so no oracle pairing with Test.db exists.

## Decisions and why

- Fork registers these as TYPE_NUMERIC only under the option. Barn's registry
  is static, so the signature carries a flag and admission/function_info read
  `RuntimeOptions.PromoteNumbers` (same pattern as `OutboundNetwork` checks).

## Next step

See State.
