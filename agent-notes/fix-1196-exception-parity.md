# Barn #1196 exception parity

## Goal

Fix the exception details reported in #1196 and add coverage for this class of builtin errors. Publish the Barn and conformance changes as separate pull requests.

## State

Barn distinguishes invalid paths, failed lookups, and non-regular files. Base64 validation raises the Toast message and original input. The MOO raise builtin, exec, and Base64 share an exception constructor.

Managed results for the same selection with capability admission were baseline Barn `21 failed, 52 passed`, fixed Barn `73 passed`, and stock Toast `73 passed`. The XML files are `barn-before-admitted.xml`, `barn-after-admitted.xml`, and `toast-admitted.xml` in `/tmp/barn-1196.NWR5MC` on Debian.

The Python result and campaign tests reported `Ran 19 tests` and `OK`. The Base64 corpus seed passed as a single Go test. The actual nightly probe generated from that seed also passed on managed Barn and Toast with admission. Each run reported `2 passed`.

## Decisions and why

Keep failure classification with each builtin. The VM appends the traceback. The nightly wrapper compares the code, message, and value for deterministic probes and uses the code for host and library diagnostics. The conformance cases assert the caught exception tuple directly.

## Surprises

The previous exec suite required forking, so Barn skipped even its independent path cases. Conformance PR #293 preserves the existing case identities and declares the forking requirement on the six cases that fork. The full Toast matrix passed with this correction.

Staticcheck reported `ST1005` for the capitalized MOO messages stored as Go errors. The resolver now returns them as public MOO message data.

## Next step

Verify the updated Barn PR checks. Land Barn PR #1200 before conformance PR #293 when a merge is authorized.
