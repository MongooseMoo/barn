"""Bounded design exploration, NOT Barn runtime or MVCC verification.

Run: python scripts/model-scheduling-gate.py
Uses only the Python standard library; deterministic, no servers or fixtures.
"""

from collections import deque
from dataclasses import dataclass, replace
from fractions import Fraction
from itertools import product


@dataclass(frozen=True)
class State:
    # N=new, Q=queued, O=offered, H=held, D=released, C=cancelled.
    status: tuple[str, ...]
    queue: tuple[int, ...] = ()
    offers: tuple[int, ...] = ()
    holders: tuple[int, ...] = ()
    cancel_requested: frozenset[int] = frozenset()


def set_status(state, request, status):
    values = list(state.status)
    values[request] = status
    return replace(state, status=tuple(values))


def successors(state, modes):
    """Each edge is one linearized semaphore operation.

    Offers reserve order, not ownership. Shared offers may be claimed in any
    order within their compatible cohort. A held cancellation retains ownership.
    Request indices represent distinct capabilities; generation reuse is not
    modeled. No mutex, VM, native stack or GC implementation is modeled here.
    """
    for request, status in enumerate(state.status):
        if status == "N":
            updated = set_status(state, request, "Q")
            yield "enqueue", replace(updated, queue=state.queue + (request,))
        elif status in ("Q", "O"):
            updated = set_status(state, request, "C")
            yield "cancel_waiter", replace(
                updated,
                queue=tuple(r for r in state.queue if r != request),
                offers=tuple(r for r in state.offers if r != request),
            )
        elif status == "H":
            if request not in state.cancel_requested:
                yield "cancel_holder", replace(
                    state, cancel_requested=state.cancel_requested | {request}
                )
            updated = set_status(state, request, "D")
            yield "owner_release", replace(
                updated,
                holders=tuple(r for r in state.holders if r != request),
                cancel_requested=state.cancel_requested - {request},
            )

    if state.queue:
        first = state.queue[0]
        outstanding = state.offers + state.holders
        if not outstanding or (
            modes[first] == "S" and all(modes[r] == "S" for r in outstanding)
        ):
            cohort = [first]
            if modes[first] == "S":
                for request in state.queue[1:]:
                    if modes[request] != "S":
                        break
                    cohort.append(request)
            updated = state
            for request in cohort:
                updated = set_status(updated, request, "O")
            yield "offer_prefix", replace(
                updated,
                queue=state.queue[len(cohort):],
                offers=state.offers + tuple(cohort),
            )

    for request in state.offers:
        updated = set_status(state, request, "H")
        yield "claim", replace(
            updated,
            offers=tuple(r for r in state.offers if r != request),
            holders=tuple(sorted(state.holders + (request,))),
        )


def check(state, modes):
    outstanding = state.offers + state.holders
    assert len(set(outstanding)) == len(outstanding), "duplicate grant"
    if any(modes[r] == "X" for r in outstanding):
        assert len(outstanding) == 1, "exclusive overlaps another offer/holder"
    for request, status in enumerate(state.status):
        assert (request in state.queue) == (status == "Q")
        assert (request in state.offers) == (status == "O")
        assert (request in state.holders) == (status == "H")
    assert state.cancel_requested <= set(state.holders)


def explore(modes):
    initial = State(("N",) * len(modes))
    pending = deque([initial])
    seen = {initial}
    edges = 0
    while pending:
        state = pending.popleft()
        check(state, modes)
        next_states = list(successors(state, modes))
        if not next_states:
            assert all(s in ("C", "D") for s in state.status), "stuck request"
        for operation, updated in next_states:
            edges += 1
            check(updated, modes)
            if operation == "cancel_holder":
                assert updated.holders == state.holders, "cancel unlocked holder"
            if operation == "claim":
                newly_held = set(updated.holders) - set(state.holders)
                assert newly_held <= set(state.offers), "stale/unoffered claim"
            if operation == "offer_prefix":
                added = tuple(r for r in updated.offers if r not in state.offers)
                assert added == state.queue[: len(added)], "FIFO overtaking"
            if updated not in seen:
                seen.add(updated)
                pending.append(updated)
    return len(seen), edges


def has_cycle(edges):
    """Check specified wait-for examples, not every possible runtime graph."""
    nodes = {node for edge in edges for node in edge}
    active, done = set(), set()

    def visit(node):
        if node in active:
            return True
        if node in done:
            return False
        active.add(node)
        if any(visit(target) for source, target in edges if source == node):
            return True
        active.remove(node)
        done.add(node)
        return False

    return any(visit(node) for node in nodes)


def counterexamples():
    worker_cycle = (
        ("gate head", "worker"),
        ("worker", "younger promotion"),
        ("younger promotion", "gate head"),
    )
    gc_cycle = (
        ("unclaimed offer", "VMStartMu"),
        ("VMStartMu", "sweep hook"),
        ("sweep hook", "unclaimed offer"),
    )
    assert has_cycle(worker_cycle)
    assert has_cycle(gc_cycle)
    # Reserving the executor before joining the gate removes the first edge.
    assert not has_cycle(worker_cycle[1:])
    # Maintenance must queue before closing VM starts, removing this edge.
    assert not has_cycle((gc_cycle[0], gc_cycle[2]))

    # A protected-read set grafted onto fixed snapshots is insufficient.
    snapshot = {"x": (0, 0), "y": (0, 0)}  # value, version
    current = dict(snapshot)
    protected = {"x"}
    effect_issued = True
    assert "y" not in protected  # A purported disjoint writer is admitted.
    current["y"] = (1, 1)
    later_read = snapshot["y"]
    failed_validation = later_read[1] != current["y"][1]
    assert effect_issued and failed_validation
    return 3


def service_example():
    # Two continuously ready principals, unequal slice costs, equal weights.
    # A serial toy schedule, not a performance estimate or parallel proof.
    cost = (1, 9)
    service = [0, 0]
    passes = [Fraction(0), Fraction(0)]
    turns = [0, 0]
    for sequence in range(1000):
        principal = min(range(2), key=lambda p: (passes[p], turns[p], p))
        service[principal] += cost[principal]
        turns[principal] += 1
        passes[principal] += cost[principal]
    assert abs(service[0] - service[1]) <= max(cost)
    return {"service_units": service, "turns": turns}


def main():
    if not __debug__:
        raise SystemExit("Run without -O: this exploration requires assertions")
    states = edges = 0
    for modes in product("SX", repeat=3):
        count, transitions = explore(modes)
        states += count
        edges += transitions
    print(f"gate model: {states} states, {edges} transitions, 8 mode assignments")
    print("PASS: exclusion, FIFO cohorts, cancellation ownership, finite completion paths")
    print(f"counterexamples reproduced: {counterexamples()} (worker, GC, fixed-snapshot promotion)")
    print(f"serial service example: {service_example()}")
    print("LIMIT: abstract bounded model; no Barn runtime, MVCC proof, GC proof, or performance claim")


if __name__ == "__main__":
    main()
