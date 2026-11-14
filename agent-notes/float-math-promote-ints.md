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

- Barn fix: `Signature.PromotedFloat` on the 23 math descriptors; admission
  (`validateFunctionArgs`) and `function_info` treat their FLOAT positions as
  numeric when `RuntimeOptions.PromoteNumbers` is on; the implementations read
  arguments through `numberFloat`. Promote lane: test 6 passed, 2 skipped
  (was 4 failed); strict lane unchanged at 3 passed, 5 skipped.

## Decisions and why

- Fork registers these as TYPE_NUMERIC only under the option. Barn's registry
  is static, so the signature carries a flag and admission/function_info read
  `RuntimeOptions.PromoteNumbers` (same pattern as `OutboundNetwork` checks).
- The promote lane ran without `--oracle-profile-manifest`: the profile gate
  needs a PROMOTE_NUMBERS oracle manifest for Test.db, and none exists.

## Surprises

- The managed Mongoose Toast at 72e3c7f9 lacks the promotion commits.

## Next step

- Toast-first proof of the promote half needs a managed Mongoose Toast oracle
  built from a ref containing `2a21a4e`, with a Test.db manifest. Not done.
- Full Go test suite and full conformance suite were not run here; CI on a PR.
