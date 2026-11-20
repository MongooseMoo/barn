# Elle history shapes

## Goal
Expand transaction structures and expose multi-transaction cycles in Q's approved
Elle campaign. This is an independent PR against master.

## State
Implemented; PR CI pending. Baseline workflow 42344496116 passed all healthy and
deliberate fault cases. No local tests run. The new ring control and committed
shape counters remain unverified until CI returns their histories and witnesses.

## Decisions and why
- Add a four-property held ring: each transaction reads its predecessor's output
  before the barrier and appends to its own row afterward. A ring-only property
  read fault must expose its four-transaction dependency cycle while other
  synchronized waves retain their validation.
- Vary one to four slices and sparse read/write structures; retain all actual
  runtime-path requirements and native outcome handling.
- Allocate monotonically increasing unique tokens starting at 2^40 for generated writes instead
  of multiplying round/worker indices. Retries reuse the same logical operation
  token and their discarded attempts remain absent from the committed history.
- Keep deterministic negative controls small; broaden healthy rounds only.

## Surprises
The existing register generator always reads all three keys after each write,
and every main worker always has two slices. More seeds alone cannot change that.

## Next step
Publish for PR CI. Retain exact EDN and manifests, inspect the ring-only fault's
witness and committed shape counters, and fix failures before merging.
