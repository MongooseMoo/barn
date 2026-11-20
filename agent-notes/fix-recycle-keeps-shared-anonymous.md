# fix/recycle-keeps-shared-anonymous

## Goal
`recycle(o)` must not recycle anonymous objects that `o`'s property values
reference while something else still references them. Toast's `bf_recycle`
(`objects.cc`) runs `:recycle`, cleans up contents and parents, and
`db_destroy_object` frees the property values; an anonymous object dies only
when its last reference goes. Barn recycled every anonymous object found in
the target's properties (including inside lists and maps), so a shared one
became invalid while other properties still pointed at it.

## State
Done. `FinishRecycleLifecycle` no longer recycles them. It hands each property
value that may hold a finalizable value to the new
`Execution.ReleaseFinalizable` hook, which the VM wires to
`collectPendingFinalizationsFromValue`, the path a variable's old value takes.
The deferred anonymous collector then recycles each one only when it is
unreachable from persistent state and live tasks. New candidate conformance
file `tests/mongoose-conformance/builtins/recycle_keeps_shared_anonymous.yaml`.

## Decisions and why
- Reuse the release ledger rather than computing reachability inside
  `recycle()`: the builtin can't see sibling tasks' VMs, and the deferred
  collector already snapshots every live root.
- Outside a VM (the synchronous test host) the hook is nil and nothing is
  recorded; those callers never ran the deferred collector anyway.

## Surprises
- The cascade came from 781973d4 ("Fix recycle lifecycle and chparent
  anonymous invalidation"); no conformance case pinned it.
- A related bug, found while checking this: `ResetMaxObject` lowers the shared
  id counter below live anonymous ids, so a later anonymous `create()` takes a
  live anonymous object's identity. Not fixed here; proposed instead that
  anonymous identities get their own namespace, as WAIFs have.

## Next step
Not checked: anonymous objects reachable only through WAIFs held by the
recycled object, and recycling inside a task that is then killed before its
deferred collection runs.
