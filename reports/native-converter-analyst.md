# Native converter analysis

## Scope

Read-only review of completed native frame conversion, reader retention,
iteration normalization and loader failure propagation. Pending control-transfer
implementation is excluded. No implementation edits or publication.

## State

Completed scoped source analysis. Two findings remain for parent disposition.
The advertised role protocol file is absent; inherited gauntlet instructions
apply. No broad approval or successful-resumption claim beyond existing evidence.

## Findings

Initial dynamic-except decoder candidate was retracted before any fix: the
encoding is count=2 followed by {ExceptCodesInLocal, slot}, and importedHandlers
preserves that pair correctly. It does not treat255 as the count. No panic was
established from that candidate; parent notified not to act on it.

Continuing actual origin, context, interruption and value-retention checks.
Three optional VM filename probes were invalid; subsequent searches use the
verified VM directory instead. Those failed reads changed no files.

### P2: Native inherited command environment is discarded

The native reader leaves CommandEnvironment unset. ConvertNativeFrame copies
only variables present in the newly compiled VarNames, then separately recovers
Args and Caller. It never recovers the six inherited command fields absent
from the current source's local slots. The lowerer allocates names on use, so
start/pause in the retained native producer have only `this` as a source local.

Actual current-reader output for all three retained native producer frames:

```text
argstr='', dobjstr='', iobjstr='', prepstr='', dobj=#-1, iobj=#-1
```

The restored fallback array therefore contains None rather than these recorded
values. vm/op_verb.go copyCommandLocals reads commandEnvironment and forwards
that fallback to later calls; vm/control.go also uses it for fork inheritance.
This is producer-backed state loss. Parent is producing a nonempty-command
checkpoint whose resumed verb calls a helper to verify the visible endpoint.

This/ThisValue, Player, Verb/StoredVerb and thread mode are separately retained
from PI. Programmer is restored through top task context and saved parent
context. Args and object Caller are explicitly recovered from full native
locals even when absent from current compiled VarNames; those fields do not
share this omission.

### P2: Restoration census can pass with zero native continuations

TestToastNativeCheckpointRestorationCensus accepts any nonempty environment
path, loops over its saved records and finishes with Logf only. It does not
require nonzero queued tasks, suspended/interrupted tasks or native frames.
The existing valid .tmp/conformance/db/Test.db is a concrete zero-task input.
Selecting it would produce zero coverage without a failed assertion. The
separate native-layout census received a nonzero guard, but this test did not.
The actual original49-task result remains valid; the mechanical verification
guard is missing for a future wrong input.

## Additional support boundary requiring an oracle witness

Native execute.cc EOP_FOR_LIST_1/2 advances string cursors by bytes:
memo_strlen is raw byte size and list.cc strget returns one byte at i-1. Barn's
existing types.Value.IterationColumns uses SplitChars UTF-8 decoding steps.
The importer currently derives a string cursor by subtracting1 while using the
Barn columns. A multibyte native string loop therefore needs explicit policy
and a managed native producer before claiming support; no original70-frame
UTF-8 loop or measured workload effect was established. Parent notified. This
is not a proven native-layout offset defect or a request to change general
Barn string semantics without that evidence.

## Verification and limits

Read the exact current native producer through current lambdamoo-db-py's
read_v17_header. Checked its three frames' actual inherited field values.
Read native read_activ_as_pi, EOP_FOR_LIST and strget source directly.
No implementation changes, servers or local Go test runs were performed.
The dynamic-except concern was retracted after reading the actual encoding.
Unsupported native builtin/function/pending-finally states return errors rather
than being silently interpreted. Interrupted WakeValue E_INTRPT follows the
existing SetResumeValue error path; no additional proven error-injection defect
was found. VM verifier remains after conversion. Pending control-transfer code
under another worker remains excluded from this review.
