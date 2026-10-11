# Bundled WIT parser

`wasm-tools.wasm.gz` is copied unchanged from
[`go.bytecodealliance.org` v0.7.0](https://github.com/bytecodealliance/go-modules/tree/v0.7.0/internal/wasmtools).
It is the standard wasm-tools executable compiled to WebAssembly, not a Barn
parser or a private IDL. Its compressed SHA-256 is:

```text
732779402af524ca78074b953bf11932020ac7a019c8ab9fc15a9eef7fb7e090
```

The distributor's Apache 2.0 license, including its LLVM exceptions, is reproduced as
[`wasm-tools.LICENSE`](wasm-tools.LICENSE). Upstream wasm-tools source is maintained
by the [Bytecode Alliance](https://github.com/bytecodealliance/wasm-tools).

Barn embeds this artifact directly because the distributor's `wit.DecodeWIT`
does not expose a context, close its runner, or allow a memory/output limit. The
small runner in `wit.go` adds those bounds and an in-memory compilation cache.
The schema is decoded with the distributor's `wit.DecodeJSON`; its WIT type model
and the Component Model Canonical ABI remain standard.
