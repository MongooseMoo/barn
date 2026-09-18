# Builtin descriptors and capabilities

`builtins.BaseDescriptors()` owns each base builtin's implementation, signature,
admission rules, visibility, effect policy, line synchronization, and capability.
`vm.Descriptors()` contributes complete `eval` and `pass` descriptors before
construction. `NewRegistryFromDescriptors` validates every descriptor, rejects
duplicates and layouts larger than 256 entries, and copies its inputs. The
resulting registry has no registration or replacement API.

Descriptor order defines IDs. The default order preserves the 252 IDs from
`4eacf59`; capability filtering leaves holes rather than reassigning IDs. Append
new descriptors without reordering existing ones. Compilers snapshot the lookup
map, and compiled programs retain its fingerprint through fork extraction and
checkpoint serialization. Execution and snapshot restoration reject a different
fingerprint. Older checkpoints have no fingerprint; preserving the default IDs
and leaving disabled slots empty prevents reinterpretation of those IDs.

## Default policy

Barn enables all implemented families by default, preserving existing callable
extensions. No previously callable builtin is removed by default. Availability
is independent of runtime permissions: `OUTBOUND_NETWORK = 0` does not remove
`open_network_connection`.

An explicit `BUILTIN_CAPABILITIES` assignment replaces the default set:

```text
BUILTIN_CAPABILITIES = core,barn-extensions,network,sqlite
OUTBOUND_NETWORK = 0
```

| Capability | Builtins |
| --- | --- |
| `core` | Standard base functions, including VM-owned `eval` and `pass` |
| `barn-extensions` | `upcase`, `downcase`, `capitalize`, `implode`, `trim`, `ltrim`, `rtrim`, `unique`, `mapmerge`, `connection_option`, `finished_tasks` |
| `background-tasks` | `background_test` |
| `allocator-stats` | `malloc_stats` |
| `process-stdin` | `read_stdin` |
| `network` | `open_network_connection`, `curl`, `listen`, `unlisten`, `listeners` |
| `sqlite` | All `sqlite_*` functions |

`none` disables every family. Unknown or duplicate names are configuration
errors. Disabled functions have no compiler/name lookup, dynamic call target,
public introspection entry, or presence claim. `function_info()` visibility is
separate: the existing hidden extensions remain callable when enabled. Runtime
feature reporting and profile manifests obtain builtin presence from the actual
registry, including hidden functions.

The canonical stock WSL Toast oracle does not register `background_test`,
`malloc_stats`, `read_stdin`, or `finished_tasks`. Barn's default availability of
these implementations is an explicit extension policy, not a stock Toast claim.
`builtins/testdata/descriptor_oracle/contracts.yaml` records that oracle census
and independent JSON contracts, including issue #238's third argument.

## Admission and effects

Signatures describe the positional registration checks, with optional union and
variadic rules. Semantic checks still belong to implementations. For example,
permissions and resource validity can reject arguments admitted by a signature.
The hidden `connection_option` extension admits both positional types at dispatch
because its implementation checks connection existence and permissions before
the option-name type. Moving that type check earlier would change `E_INVARG` or
`E_PERM` into `E_TYPE` for the same call.
Fixed-arity argument errors preserve their existing payloads, including plain
errors for extensions that previously validated their own arguments.

Protected-builtin redirection precedes admission and receives raw arguments.
Admission precedes an irreversible effect. Descriptors distinguish ordinary
transactional work, guarded irreversible effects, commit-gate reentry for
`dump_database`/`shutdown`, and implementation-owned SQLite statement boundaries.
`callers` and `task_stack` request line synchronization through their descriptors.

Tests substitute implementations in descriptor lists before construction. No
production mutation API exists for test injection. Callback slots in engine
fixtures allow callbacks to capture their runtime without changing its registry.
