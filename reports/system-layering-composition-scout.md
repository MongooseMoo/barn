# Composition, startup and observability survey

## State

P9 scout completed a read-only survey. No tests or implementation were run by
the scout. Coordinator inspected the late script read, Start failure path,
manifest checksum producer and panic/normal teardown distinctions.

## Repair early startup input validation

app.Run loads the runtime and starts memory watching before reading StartScript.
A missing/unreadable script returns directly without runtime/session teardown.
Read and validate the script in the same Run owner before constructing/loading
server resources, while preserving later script/StartLine execution ordering.
Do not invent a new Close facade to compensate for the late input operation.

## Stream profile checksums

profile.checksumFile reads an entire database into a temporary byte slice before
hashing it. BuildManifest hashes DatabasePath/ConfigPath before server load.
Benchmark the existing helper's allocations first, then use streaming SHA256
in that same owner, preserving exact digests and native open/read/close errors.
No cache, pool or new hashing layer is justified.

## Repair listener-start failure teardown

Start's ReserveListeners error path stops input/runtime and joins background
work but omits builtin Session.Close; normal shutdown calls it. Real handles can
exist through restored tasks when input starts before reservation. Close the
existing session after the existing failure cleanup and join a cleanup error
with the listen error. An embedded server fixture with a pre-opened real handle
and occupied listener can reproduce this without racing restored dispatch.

## Keep panic and composition contracts

Panic dumps emergency state, records ErrPanicShutdown and cancels. mainLoop
returns that error directly to the CLI's abort path, intentionally skipping
graceful hooks/ordinary dumping. Do not add a blanket shutdown defer that would
change this contract or delay process termination without proof.
No additional real late LoadDatabase failure producer was established.

Keep configuration duplicate/unknown rejection and final validation; profile
features derived from actual options/registry; manifest publication before game
sockets; shared listener vocabulary and post-hook acceptance; per-run passive
lifecycle probes separated from privileged diagnostics; bounded process metrics
separated from opt-in task records; payload-free tracing; and MOO profiler
session/nested-charge/sample-release ownership. Wall intervals overlap and do
not establish CPU attribution. No measured bottleneck ranking is claimed.
