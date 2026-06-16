# Parser Package Rules

The `parser` package owns MOO syntax only.

Hard boundaries:
- Do not import `barn/types` or any other Barn runtime package from `parser`.
- Do not construct runtime values in `parser`.
- Do not encode truthiness, map-key validity, VM behavior, database behavior, server behavior, or builtin behavior here.
- Keep `ANY` and error names as syntax. Runtime packages decide what they mean.
- Keep literals as parser-owned syntax nodes. Runtime packages lower them into `types.Value`.
- Do not add compatibility adapters, senders, helpers, or wrapper APIs to preserve deleted runtime-shaped parser APIs.

Deletion-first rule:
- When an old parser API exposes runtime concepts, move callers to the real runtime owner and delete the old parser surface.
- A parser cleanup is not complete while the old runtime-shaped path still coexists with the syntax-only path.
