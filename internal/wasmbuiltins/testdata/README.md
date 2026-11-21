# Component fixtures

`example.wasm` is built from the repository's `examples/wasm` source and lockfile
using `wit-bindgen` 0.62.0 and wasm-tools 1.261.0. From the repository root:

```sh
cargo build --locked --release --target wasm32-unknown-unknown --manifest-path examples/wasm/Cargo.toml
wasm-tools component new examples/wasm/target/wasm32-unknown-unknown/release/barn_wasm_example.wasm -o internal/wasmbuiltins/testdata/example.wasm
wasm-tools parse internal/wasmbuiltins/testdata/malformed.wat -o internal/wasmbuiltins/testdata/malformed.wasm
wasm-tools parse internal/wasmbuiltins/testdata/scalar.wat -o internal/wasmbuiltins/testdata/scalar.wasm
cd internal/wasmbuiltins/testdata
wasm-tools component embed stateful.wit stateful-core.wat -o stateful-embedded.wasm
wasm-tools component new stateful-embedded.wasm -o stateful.wasm
rm stateful-embedded.wasm
```

`stateful.wasm` was built with wasm-tools 1.230.0, the parser artifact embedded
as `../wasm-tools.wasm.gz`, run under wazero with the working directory mounted.
Its two exports change guest state that a reused guest would expose:
`bump-global` increments a mutable core global and `bump-memory` a word in
linear memory; each returns the new value, so a fresh guest returns 1.

`malformed.wat` independently constructs overflowing scalar-list spans and an
invalid string span. It also exports an integer function so tests can prove
that recovery leaves the admitted generation usable.

`scalar.wat` has no world-local types or host imports and keeps a core module
name. Tests exercise concurrent calls, memory/table growth bounds, non-finite
results and oversized result lists. Re-extraction verifies the normalized world.

SHA-256 of the checked-in artifacts (compiler upgrades can change the Rust binary):

```text
590044faa41ce717e31304795d00f889d19bbb70d372ae9b165fce851c380c11  example.wasm
5837a08f54d0b7d7fa841e54749eb1d15552dfb8f66671f36367990acc88fafd  malformed.wasm
f16373ded081d7361bff9a0f34f8dd77e19b287fd141e188d72351c5bac7bbb7  scalar.wasm
4ce99e1486a04b283b40286a12678fc528026cd8decb4f593c8f4651795ff590  stateful.wasm
```
