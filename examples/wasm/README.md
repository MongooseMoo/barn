# WIT builtin example

See [the authoring guide](../../docs/wasm-builtins.md) for build commands, MOO
calls, type mappings and admission limits. Edit `wit/builtins.wit`, implement
the generated `Guest` trait in `src/lib.rs`, then build a standard component.

The `spin` and `trap` exports exercise execution limits in Barn's tests. Remove
them when adapting this example for your own library. The `counter` export
demonstrates fresh guest state: it returns `1` on every call.
