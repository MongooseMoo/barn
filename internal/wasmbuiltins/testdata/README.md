# Component fixtures

`example.wasm` is built from the repository's `examples/wasm` source and lockfile
using `wit-bindgen` 0.62.0 and wasm-tools 1.261.0. From the repository root:

```sh
cargo build --locked --release --target wasm32-unknown-unknown --manifest-path examples/wasm/Cargo.toml
wasm-tools component new examples/wasm/target/wasm32-unknown-unknown/release/barn_wasm_example.wasm -o internal/wasmbuiltins/testdata/example.wasm
wasm-tools parse internal/wasmbuiltins/testdata/malformed.wat -o internal/wasmbuiltins/testdata/malformed.wasm
wasm-tools parse internal/wasmbuiltins/testdata/scalar.wat -o internal/wasmbuiltins/testdata/scalar.wasm
```

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
```
