# WIT builtins

Write a normal WebAssembly Component Model library. Its exported WIT functions
become callable from MOO through one stable builtin:

```moo
wasm_call("example.add", 20, 22);       /* 42 */
wasm_call("example.greet", "MOO");     /* "Hello, MOO!" */
```

The component carries its own type information. There is no Barn manifest,
handwritten pointer ABI, or guest SDK. Standard `wit-bindgen` generates the
Canonical ABI glue. Barn parses the component's WIT at load time, then caches
the value conversion plan and compiled code. Native builtin dispatch is unchanged.

```mermaid
flowchart LR
    W[WIT contract + implementation] --> C[Standard component.wasm]
    C --> L[Load and validate]
    L --> G[Published generation]
    G --> F[Fresh guest for each call]
    F --> M[MOO result]
```

## Write and build

The complete [Rust example](../examples/wasm) exports arithmetic, strings,
records, enums, variants, options and results. Its contract starts with:

```wit
package barn:example@1.0.0;

world builtins {
    export add: func(a: s64, b: s64) -> s64;
    export greet: func(name: string) -> string;
}
```

Use Rust with `wasm32-unknown-unknown` and the standard authoring tool
[`wasm-tools`](https://github.com/bytecodealliance/wasm-tools). From
`examples/wasm`:

```sh
rustup target add wasm32-unknown-unknown
cargo build --locked --release --target wasm32-unknown-unknown
wasm-tools component new target/wasm32-unknown-unknown/release/barn_wasm_example.wasm -o example.wasm
wasm-tools component wit example.wasm
```

Copy `example.wasm` into your operator directory. A module name is its basename;
it must match `[a-z][a-z0-9_-]{0,63}`. Export names retain their WIT spelling, so
`record-echo` is called as `wasm_call("example.record-echo", value)`.

## Start, inspect, replace, unload

```sh
barn --db /srv/barn/world.db --wasm-dir /srv/barn/builtins
```

The same directory can be set with `WASM_DIRECTORY = /srv/barn/builtins` in a
Barn config file; `--wasm-dir` overrides it. It also works with `--eval` and
`--eval-file`. With no directory, loading is disabled and `wasm_modules()` is empty.

Startup admits every `.wasm` file in sorted order and fails if any component is
invalid. Files must be regular files, not symlinks. Names resolve beneath an
`os.Root`; MOO supplies a module name, never a filesystem path. The directory and
its contents are controlled by the server operator.

```moo
wasm_modules();                /* sorted metadata for the current generations */
wasm_load("example");          /* wizard: atomically load or replace */
wasm_unload("example");        /* wizard: 1 if removed, 0 if absent */
```

Metadata contains `name`, `sha256`, `generation`, and `exports`. Each export
contains its name, named parameter types, and result type. A load error raises
`E_INVARG` with the admission diagnostic. Replacing the file and calling
`wasm_load` publishes a new generation only after successful validation. Failed
replacement leaves the old generation callable.

Unload removes the module immediately from lookup. Calls that already acquired
the old generation finish within their deadline; the last call frees its code.
Replacement follows the same rule. Session shutdown seals admission, waits for
bounded active calls, and closes resources. Sessions never share modules or state.

Load and unload cross the task's irreversible-effect boundary after the wizard
check. Pure calls remain transactional: a conflict retry may resolve a newer
generation. Every call gets a fresh guest, so globals and memory cannot become
hidden persistent task state. Unloading changes memory, not the operator's files;
the directory is loaded again on restart.

All four functions belong to the `barn-extensions` capability and use the usual
protected-builtin mechanism. Components do not register new native builtin names,
change bytecode IDs, or mutate a registry fingerprint.

## Values

| WIT | MOO |
| --- | --- |
| `s8`…`s64`, `u8`…`u64` | Integer; range checked. `u64` results above signed 64-bit range raise `E_RANGE`. |
| `f32`, `f64` | Float |
| `bool` | Boolean |
| `string` | Valid UTF-8 string |
| `char` | String containing exactly one Unicode scalar |
| `list<primitive>` | List; includes `list<u8>`, with integer elements |
| `tuple<T…>` | List in declaration order |
| `record` | Map with exact, case-sensitive field names |
| `enum` | Case name string |
| `option<T>` | `{}` for none; `{value}` for some |
| `variant` | `{case_name, payload}`; payload is `0` for a payload-free case |
| `result<T,E>` | `{"ok", value}` or `{"err", value}`; absent payload is `0` |
| No result | `0` |

A WIT result's `err` arm is an ordinary domain value. Traps raise `E_INVARG`.
Wrong argument count raises `E_ARGS`; wrong MOO types raise `E_TYPE`; out-of-range
integers raise `E_RANGE`; time, concurrency and value limits raise `E_QUOTA`.
Existing task string/list/map limits also apply to returned values.

## Admission profile and limits

Barn currently accepts synchronous world-level functions, up to 64 exports and
16 parameters per export. The component must contain exactly one embedded core
module, at most eight core instances, and no nested components. It can define
types but cannot import functions, interfaces, WASI, or resources. No host
filesystem, network, clock, randomness or MOO callback is provided.

Each guest memory is limited to 64 pages (4 MiB). A core table must be a single
`funcref` table with an explicit maximum no larger than 16,384. Core declaration
vectors are limited to 4,096 entries, function types to 64 parameters/results,
and locals to 4,096 per function and 65,536 per module. These bounds are checked
before runtime decoding can allocate storage from untrusted declarations. Components are
limited to 16 MiB each and 32 active module names per session. One load and eight
calls can run concurrently, with at most two calls per player. Saturation returns
`E_QUOTA` immediately so a task does not occupy a worker waiting for a guest slot.
Calls have a 250 ms deadline including instantiation; each call charges 1,000 MOO
ticks. Guest execution observes the task's exact remaining time. Load validation
uses a five-second context deadline and also respects the calling task's remaining
time. Native code compilation is not preemptible by this deadline: registration
can overrun it while compiling, then fail when the expired context is observed.
File and declaration limits bound what the compiler accepts.

Each conversion is limited to 65,536 nodes and 1 MiB of string data. Types have
at most 16 levels, 64 fields/cases, 1,024 expanded schema nodes, and four dynamic
leaves per value. Lists support fixed-width primitives only; `list<string>`,
lists of records, and nested lists are rejected at load time. Flags, resources,
futures, streams, async functions, and nested options are also rejected. Only
synchronous canonical lifts with UTF-8, memory, realloc and post-return options
are accepted. Non-finite float values raise `E_FLOAT` in either direction.

The host is pure Go: [Wazy](https://github.com/samyfodil/wazy) executes the
Component Model, and [Bytecode Alliance's Go WIT package](https://github.com/bytecodealliance/go-modules)
decodes the schema extracted by bundled wasm-tools on wazero. Barn owns the
parser runner so it can close instances and bound execution to five seconds,
128 MiB of memory, 4 MiB of output, and 4 KiB of diagnostics. The shared code cache
stays in memory. The trusted parser is compiled once during session setup, before
task load deadlines begin. Parser artifact provenance and its license are recorded in
[THIRD_PARTY.md](../internal/wasmbuiltins/THIRD_PARTY.md). No native runtime or external
parser executable is required on the server. Wazy's Component Model API is young;
the restricted profile fences known composite-list lifting hazards rather than
promising support for every component. The runtime dependency is pinned.

Recovered guest/runtime faults increment `barn.wasm_faults_recovered` and write a
diagnostic with a stack trace. They return a MOO error without terminating the server.

For components with one core instance and no imports, Barn appends an unobserved
standard inline core instance that re-exports an existing core function. This
selects Wazy's cached, anonymous graph instantiation instead of its uncached
path with shared module names. Original function indices and the WIT world stay
unchanged; metadata hashes the original file. The normalization is isolated in
`compile.go` and can be removed when the runtime fixes its fast path.

## Design review and tests

Claude reviewed the descriptor split and proposed the stable gateway, fresh
guests, atomic generations, and standard WIT contract. A separate implementation
review checks the lifecycle and ABI boundaries.

Tests use a checked-in component built from the Rust example and independent WAT
fixtures that return hostile ABI spans. Rebuild instructions and hashes are in
[the fixture record](../internal/wasmbuiltins/testdata/README.md). Go tests need
neither Rust nor wasm-tools. The engine evaluation test exercises startup,
compilation, invocation, unload, and reload through MOO code.
