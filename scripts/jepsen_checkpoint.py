#!/usr/bin/env python3
"""Jepsen checkpoint-durability campaign: Barn lifecycle owner and MOO adapter.

The Jepsen project in tests/jepsen-checkpoint schedules concurrent list-append
transactions and pause/resume/crash/checkpoint faults. Each operation is one
JSON line to this adapter, which owns every Barn process generation through
mongoose_barn_process.OwnedBarnServer (registered public Test.db profile) and
reaches MOO through moo-conformance-tests' SocketTransport.

Durability is judged only against explicit completed checkpoints: a graceful
owned SIGTERM whose database/WAIF pair the owner verified, preceded by a
frozen ledger read (the baseline). Every later generation launches from a copy
of the most recent verified pair. A command reply is never a durability
acknowledgment. Disconnects and timeouts are :info unknowns. Histories are
split per process generation and checked with the pinned Elle CLI, so Elle
never treats uncheckpointed replies as durable.

--workloads selects the row families the Jepsen clients mix (FAMILIES): the
smoke map rows, property rows spread across objects, and keyed-map rows.
Each selected family must show successful reads and writes around every
fault, and Elle checks all of them per generation.

The adapter journals every operation before it is sent and after it
completes (journal.jsonl), so an interrupted run keeps its evidence; analysis
reconciles that journal with Jepsen's history by operation id. Before each
crash and checkpoint the campaign reads every row its clients touched in the
generation, and after each resume and launch it requires a bounded
fresh-connection write and read. Analysis checks each scenario obligation and
each generation's runtime log, and reports one of four non-healthy result
classes (see EXIT_CODES) with every finding.

The live lane (plan section 2) checkpoints while clients continue: SIGALRM
requests one checkpoint at a time and its completion is acknowledged only by
Barn's `checkpoint complete` record plus the stable digest of the published
pair. A checkpoint-crash requests another checkpoint, waits a seeded random
delay, stops the owned process, classifies the frozen publication files,
kills it, copies the artifacts for forensics and restarts from the
interrupted checkpoint's actual output path, whose journal Barn's loader
recovers. Before admitting that generation, one task reads the recovered
rows; the snapshot oracle (snapshot_checks) checks them against the
generation's history and its completed (A) and pending (F) checkpoints.

The stage schedule (--stages, elle build) arms each generation with one
STAGE_SCHEDULE stop point; its stage-crash waits for the process's own
boundary record and stopped state instead of a random delay, then follows the
checkpoint-crash path. stage_coverage reports what each step reached, killed
and recovered. --control runs one negative control (CONTROLS) and passes only
on its designated witness.

The task lane (--tasks, plan section 4) plants test-owned held tasks before
checkpoints and releases them through a MOO-held driver only after the next
launch has read their state; task_checks is its lifecycle checker. The effects
family notifies a receiver connection and crosses set_connection_option's
irreversible boundary under forced conflicts; effect_checks reads the
receiver's lines (receipts.jsonl). Neither puts a fork/suspend/effect sequence
into an Elle transaction.
"""
from __future__ import annotations

import argparse
import json
import os
import platform
import random
import shutil
import socket
import socketserver
import subprocess
import sys
import threading
import time
from pathlib import Path

import check_elle
import mongoose_barn_process as barn_process

PROFILE_ID = "barn-linux-testdb-outbound-on"
CONFIG = barn_process.ROOT / "profiles" / "barn" / "outbound-on.conf"
JVM_HEAP = "-Xmx2g"  # Same bound as tests/jepsen-checkpoint/project.clj.
# Ceiling for a post-heal/restart probe or a final observation. Not a measured
# healthy bound; each probe records its actual latency.
PROBE_DEADLINE = 30.0
# Elle process for the campaign's own final observations; Jepsen client
# processes stay far below it.
OBSERVER_PROCESS = 10 ** 9
FAULTS = ("pause", "resume", "crash", "checkpoint", "live-checkpoint", "checkpoint-crash", "stage-crash")

# Deterministic stage schedule (plan section 2 interruption table) for the
# elle build's stop points (internal/isolationprobe/checkpoint.go). Each step
# names the point and its occurrence in one process: a restart from P.new
# publishes to P.new.new, so occurrence 1 is the process's first publication
# and occurrence 2 follows one completed live checkpoint (a previous pair).
# "recovery" arms a second stop in the restarted process's startup recovery,
# between the restore renames, for the double crash. "frozen" is the stage
# mongoose_barn_process.checkpoint_artifacts must classify from the files the
# stop leaves; "cut" is the snapshot cut the plan table requires recovery to
# select (unchanged input, the completed checkpoint A, or the pending F).
STAGE_SCHEDULE = {
    "capture-1": {"point": "checkpoint/capture", "occurrence": 1, "frozen": "staging", "cut": "unchanged input"},
    "serialize-1": {"point": "checkpoint/serialize", "occurrence": 1, "frozen": "staging", "cut": "unchanged input"},
    "staged-2": {"point": "checkpoint/staged", "occurrence": 2, "frozen": "sidecar-prepared", "cut": "completed"},
    "journal-published-1": {"point": "checkpoint/journal-published", "occurrence": 1, "frozen": "journal-published",
                            "cut": "pending"},
    "journal-published-2": {"point": "checkpoint/journal-published", "occurrence": 2, "frozen": "journal-published",
                            "cut": "completed"},
    "database-renamed-1": {"point": "checkpoint/database-renamed", "occurrence": 1, "frozen": "database-renamed",
                           "cut": "pending"},
    "database-renamed-2": {"point": "checkpoint/database-renamed", "occurrence": 2, "frozen": "database-renamed",
                           "cut": "completed"},
    "pair-renamed-1": {"point": "checkpoint/pair-renamed", "occurrence": 1, "frozen": "both-renamed",
                       "cut": "pending"},
    "double-crash": {"point": "checkpoint/database-renamed", "occurrence": 2, "frozen": "database-renamed",
                     "cut": "completed", "recovery": "checkpoint/recovery-database-restored:1"},
}
STOP_ENV = "BARN_ELLE_CHECKPOINT_STOP"
FAULT_ENV = "BARN_ELLE_FAULT"
# Ceiling for a requested checkpoint to reach its stop point. Not measured.
STOP_DEADLINE = 60.0
# Negative controls: the elle-only fault, the one stage step that exposes it,
# and the witness the analysis must report (findings' "witness").
CONTROLS = {
    "checkpoint-mixed-pair": {"stage": "database-renamed-1", "witness": "mixed-pair"},
    "checkpoint-drop-append": {"stage": "pair-renamed-1", "witness": "lost-checkpointed-update", "family": "props"},
    "checkpoint-duplicate-append": {"stage": "pair-renamed-1", "witness": "duplicate-token", "family": "props"},
    # No stage step: engine/task_runtime.go's commit-conflict retry publishes
    # the lost attempt's deferred notifications instead of discarding them, so
    # the effects receiver sees an operation's notification twice.
    "pending-effect-retry": {"witness": "duplicated-live-retry-effect", "family": "effects"},
    # No stage step: store validation faults (db/store/store_txn_validation.go,
    # store_hot_read.go). The shaped families' write-skew shapes (topology
    # t-cross-1/2 through room contents reads; authority v-observe against
    # v-mutate through the verb dispatch memo) become a two-transaction
    # anti-dependency cycle, which Elle names G2-item (the in-process verb-memo
    # fault's witness in elle.yml). These need overlapping transactions: run
    # them with --dwell and a higher rate and concurrency.
    "relationship-reads": {"witness": "elle:G2-item", "family": "topology"},
    "verb-memo": {"witness": "elle:G2-item", "family": "authority"},
    "hot-memo-guard": {"witness": "elle:G2-item", "family": "authority"},
}
# Ceiling for one live checkpoint to log completion or failure. Not measured.
CHECKPOINT_DEADLINE = 60.0
# checkpoint-crash waits uniform(0, DELAY_SPAN x the last completed live
# checkpoint's duration) after SIGALRM before stopping the server, so the
# stop lands before, during or after publication; the frozen files say which.
DELAY_SPAN = 1.5
FIRST_DELAY_SECONDS = 0.05  # before any live checkpoint has completed
# Elle process for the recovered snapshot of a generation (plan section 2 rule 5).
RECOVERY_PROCESS = OBSERVER_PROCESS + 1

CORRECTNESS = "correctness-violation"
RECOVERY = "recovery-failure"
INCOMPLETE = "incomplete-experiment"
INVALID = "invalid-harness-evidence"
# Exit status names the most severe class found; results.json lists them all.
PRECEDENCE = (CORRECTNESS, RECOVERY, INVALID, INCOMPLETE)
EXIT_CODES = {"healthy": 0, CORRECTNESS: 1, RECOVERY: 3, INCOMPLETE: 4, INVALID: 5}
# A --control run exits 0 only with its designated witness, else this.
CONTROL_MISSED_EXIT = 6

ADMIT_CODE = (
    'if (!("jepsen_ledger" in properties(#0))) add_property(#0, "jepsen_ledger", [], {#0, "r"}); endif '
    'if (!("jepsen_rows" in properties(#0))) add_property(#0, "jepsen_rows", [], {#0, "r"}); endif '
    'if (!("jepsen_probe" in properties(#0))) add_property(#0, "jepsen_probe", 0, {#0, "r"}); endif '
    '#0.jepsen_rows = []; return #0.jepsen_ledger;')
LEDGER_CODE = "return #0.jepsen_ledger;"
PROBE_READ_CODE = "return #0.jepsen_probe;"


def epoch_code(generation):
    """Mark the database as admitted for generation, after its rows exist."""
    return ('if (!("jepsen_epoch" in properties(#0))) add_property(#0, "jepsen_epoch", -1, {#0, "r"}); endif '
            f'#0.jepsen_epoch = {generation}; return #0.jepsen_epoch;')

# Row families, selected per campaign. A key's family is key // FAMILY_SPAN;
# the Jepsen workload draws each family's transactions from its own Elle
# list-append generator over keys base + phase offset + n (core.clj txns).
#   smoke: entries of the map #0.jepsen_rows, mirrored in #0.jepsen_ledger;
#          every transaction copies and rewrites both whole properties.
#   props: key K is the list in property jpK of row object K mod 4, read and
#          appended directly.
#   keyed: key K is entry K of the map property jm of holder K mod 2 (a map
#          held directly, then a clear inherited slot), mostly through keyed
#          access; see keyed_code.
#   effects: key K is the list in property jeK of the generation's effects
#          object, read and appended directly inside effects_code, which
#          also notifies the receiver connection and rewrites a hot property
#          to force validation conflicts (plan section 4).
#   authority, topology: shaped families (SHAPED, below). Key base + role
#          names one row of the generation's family objects; each
#          transaction is one shape, run as one verb call by a nonwizard
#          identity (plan section 3 rows 3 and 4).
FAMILY_SPAN = 10 ** 7
FAMILIES = {"smoke": {"base": 0, "key-count": 5},
            "props": {"base": FAMILY_SPAN, "key-count": 8},
            "keyed": {"base": 2 * FAMILY_SPAN, "key-count": 4},
            "effects": {"base": 3 * FAMILY_SPAN, "key-count": 4}}
# Fresh row objects for each generation, so its rows start empty: four
# property-row objects, then the keyed parent defining jm, a child holding
# its own map and a child whose slot stays clear.
ROWS_CODE = (
    'objs = {}; for i in [1..4] objs = {@objs, toint(create(#-1))}; endfor '
    'parent = create(#-1); add_property(parent, "jm", [], {#0, "r"}); '
    'own = create(parent); own.jm = []; inh = create(parent); '
    'return {@objs, toint(parent), toint(own), toint(inh)};')


def family_of(key):
    """The family owning key, or None."""
    return next((name for name, family in FAMILIES.items()
                 if family["base"] // FAMILY_SPAN == key // FAMILY_SPAN), None)


def parse_layout(value):
    """ROWS_CODE's reply: seven object numbers."""
    if not (isinstance(value, list) and len(value) == 7 and all(type(item) is int for item in value)):
        raise ValueError(f"row layout is not seven object numbers: {value!r}")
    return {"props": value[:4], "parent": value[4], "keyed": value[5:]}


# ---------------------------------------------------------------------------
# MOO transactions and native outcome categorisation

def _list_at(name, key):
    return f"(maphaskey({name}, {key}) ? {name}[{key}] | {{}})"


def props_code(f, key, value, layout):
    """A direct read or append of property row key; a missing property is {} until the first append adds it."""
    objects = layout["props"]
    obj, prop = f"#{objects[key % len(objects)]}", f"jp{key}"
    if f == "r":
        return f"out = {{@out, `{obj}.{prop} ! E_PROPNF => {{}}'}};"
    return (f"try {obj}.{prop} = {{@{obj}.{prop}, {value}}}; "
            f"except (E_PROPNF) add_property({obj}, \"{prop}\", {{{value}}}, {{#0, \"r\"}}); endtry")


def keyed_code(f, key, value, layout, mode):
    """A read or append of entry key of holder.jm.

    Mode 0 and 3: maphaskey(o.jm, K) and o.jm[K], read and assigned, which the
    compiler emits as keyed access (compiler/lower_expressions.go compileIndex
    and keyedMaphaskeyProperty, compiler/lower_assignments.go compileIndexAssign).
    Mode 1: a keyed read through a catch expression; the append's index
    expression first writes {} to the same entry, which the captured map must
    not see. Mode 2: whole-map read and replacement.
    """
    holders = layout["keyed"]
    obj = f"#{holders[key % len(holders)]}"
    if mode == 2:
        if f == "r":
            return f"m = {obj}.jm; out = {{@out, {_list_at('m', key)}}};"
        return f"m = {obj}.jm; m[{key}] = {{@{_list_at('m', key)}, {value}}}; {obj}.jm = m;"
    if mode == 1:
        if f == "r":
            return f"out = {{@out, `{obj}.jm[{key}] ! E_RANGE => {{}}'}};"
        captured = f"`{obj}.jm[({obj}.jm[{key}] = {{}}) == {{}} ? {key} | {key}] ! E_RANGE => {{}}'"
        return f"{obj}.jm[{key}] = {{@{captured}, {value}}};"
    entry = f"(maphaskey({obj}.jm, {key}) ? {obj}.jm[{key}] | {{}})"
    if f == "r":
        return f"out = {{@out, {entry}}};"
    return f"{obj}.jm[{key}] = {{@{entry}, {value}}};"


def txn_code(txn, layout=None, op_id=0, observation=False):
    """One MOO task running txn's micro-operations in order and returning its reads.

    Smoke rows append to the per-boot row map and the durable ledger. Property
    and keyed rows need the generation's layout; op_id picks the keyed mode.
    A client transaction of effects rows is effects_code; a campaign
    observation reads effects rows only, with no notification or hot write.
    """
    statements, smoke, appends = ["out = {};"], False, False
    for mop in txn:
        if not (isinstance(mop, list) and len(mop) == 3 and type(mop[1]) is int):
            raise ValueError(f"malformed micro-operation {mop!r}")
        f, key, value = mop
        if not ((f == "append" and type(value) is int) or (f == "r" and value is None)):
            raise ValueError(f"malformed micro-operation {mop!r}")
        family = family_of(key)
        if family is None:
            raise ValueError(f"micro-operation {mop!r} has a key in no row family")
        if family != "smoke" and layout is None:
            raise ValueError(f"{family} micro-operation {mop!r} needs the generation's row layout")
        if family in SHAPED:
            if not observation or f != "r":
                raise ValueError(f"{family} transactions run as shaped verb calls (shaped_code); txn_code only "
                                 f"reads their rows for observations")
            statements.append(f"out = {{@out, {shaped_row_expr(family, key, layout)}}};")
        elif family == "effects":
            if not observation:
                # A client transaction (one family per transaction, core.clj txns).
                return effects_code(txn, layout, op_id)
            statements.append(effects_row_code(f, key, value, layout))
        elif family == "props":
            statements.append(props_code(f, key, value, layout))
        elif family == "keyed":
            statements.append(keyed_code(f, key, value, layout, op_id % 4))
        elif f == "append":
            smoke = appends = True
            statements.append(f"rows[{key}] = {{@{_list_at('rows', key)}, {value}}};")
            statements.append(f"led[{key}] = {{@{_list_at('led', key)}, {value}}};")
        else:
            smoke = True
            statements.append(f"out = {{@out, {_list_at('rows', key)}}};")
    if smoke:
        statements[:0] = ["rows = #0.jepsen_rows;", "led = #0.jepsen_ledger;"]
    if appends:
        statements += ["#0.jepsen_rows = rows;", "#0.jepsen_ledger = led;"]
    statements.append("return out;")
    return " ".join(statements)


def observation_code(keys, layout):
    """Read every listed row in one task."""
    return txn_code([["r", key, None] for key in keys], layout, observation=True)


def recovery_observation_code(generation, keys, layout):
    """One task, run before admission: {epoch marker, queued task count, rows}.

    The rows of generation (its keys through its layout) are read only when the
    marker says the recovered database includes that generation's admission;
    otherwise its row objects need not exist and the rows reply is {}.
    """
    reads = observation_code(keys, layout) if keys else "out = {}; return out;"
    if not reads.endswith("return out;"):
        raise ValueError("observation code does not end by returning its reads")
    return ('e = ("jepsen_epoch" in properties(#0)) ? #0.jepsen_epoch | -1; t = length(queued_tasks()); '
            f'if (e != {-2 if generation is None else generation}) return {{e, t, {{}}}}; endif '
            + reads[:-len("return out;")] + "return {e, t, out};")


def parse_recovery_observation(value, keys, generation):
    """{epoch, tasks, rows}; rows maps each key to its list when the marker names generation."""
    if not (isinstance(value, list) and len(value) == 3 and type(value[0]) is int and type(value[1]) is int
            and isinstance(value[2], list)):
        raise ValueError(f"malformed recovery observation {value!r}")
    epoch, tasks, rows = value
    if epoch == generation and keys:
        if not (len(rows) == len(keys) and all(_int_list(row) for row in rows)):
            raise ValueError(f"malformed recovered rows {rows!r}")
        return {"epoch": epoch, "tasks": tasks, "rows": [list(row) for row in rows]}
    if rows:
        raise ValueError(f"recovered rows {rows!r} returned for epoch {epoch}, generation {generation}")
    return {"epoch": epoch, "tasks": tasks, "rows": None if epoch != generation else []}


def probe_write_code(token):
    return f"#0.jepsen_probe = {token}; return #0.jepsen_probe;"


# ---------------------------------------------------------------------------
# Effects lane (plan section 4): a receiver connection and an option
# connection per generation, both logged in as fresh programmers.

# One effects object per generation: the jeK rows, a hot counter every effects
# transaction rewrites, and pre, which a boundary transaction writes before
# its irreversible call so the boundary publishes it.
EFFECTS_CODE = ('e = create(#-1); add_property(e, "hot", 0, {#0, "r"}); add_property(e, "pre", 0, {#0, "r"}); '
                'return toint(e);')
EFFECT_PREFIX = "jepsen-effect"
DRAIN_PREFIX = "jepsen-drain"
TASK_EFFECT_PREFIX = "jepsen-task"
# Ceiling for a drain sentinel to reach the receiver. Not measured.
DRAIN_DEADLINE = 30.0


def boundary_op(op_id):
    """Odd operation ids cross set_connection_option's irreversible boundary."""
    return op_id % 2 == 1


def effects_row_code(f, key, value, layout):
    """A direct read or append of effects row key (property jeK of the effects object)."""
    effects = layout.get("effects") if layout else None
    if not effects:
        raise ValueError("effects micro-operations need the generation's effects objects")
    obj, prop = f"#{effects['object']}", f"je{key}"
    if f == "r":
        return f"out = {{@out, `{obj}.{prop} ! E_PROPNF => {{}}'}};"
    return (f"try {obj}.{prop} = {{@{obj}.{prop}, {value}}}; "
            f"except (E_PROPNF) add_property({obj}, \"{prop}\", {{{value}}}, {{#0, \"r\"}}); endtry")


def effects_code(txn, layout, op_id):
    """An effects transaction.

    h = E.hot is read first; notify(R, "<prefix> <op> pre") is deferred to
    commit. A boundary transaction then writes E.pre and calls
    set_connection_option(C, "flush-command", ...): a stale hot read makes
    the attempt re-run before the option is set, and the boundary publishes
    E.pre and so flushes the pre notification there. The row micro-ops follow
    the boundary, so every row write publishes at the final commit and the
    transaction stays atomic for Elle. Last, E.hot = h + 1 and the post
    notification. The receiver's lines, not notify's return value, establish
    delivery (effect_checks).
    """
    effects = layout.get("effects") if layout else None
    if not effects:
        raise ValueError("effects micro-operations need the generation's effects objects")
    obj, receiver, option = f"#{effects['object']}", f"#{effects['receiver']}", f"#{effects['option']}"
    statements = ["out = {};", f"h = {obj}.hot;", f'notify({receiver}, "{EFFECT_PREFIX} {op_id} pre");']
    if boundary_op(op_id):
        statements += [f"{obj}.pre = {op_id};",
                       f'set_connection_option({option}, "flush-command", "{EFFECT_PREFIX}-{op_id}");']
    for mop in txn:
        if not (isinstance(mop, list) and len(mop) == 3 and type(mop[1]) is int and family_of(mop[1]) == "effects"):
            raise ValueError(f"effects transaction holds a micro-operation {mop!r} outside the effects family")
        f, key, value = mop
        if not ((f == "append" and type(value) is int) or (f == "r" and value is None)):
            raise ValueError(f"malformed micro-operation {mop!r}")
        statements.append(effects_row_code(f, key, value, layout))
    statements +=[f"{obj}.hot = h + 1;", f'notify({receiver}, "{EFFECT_PREFIX} {op_id} post");', "return out;"]
    return " ".join(statements)


def drain_code(effects, token):
    """Send the drain sentinel and read the option connection's flush-command."""
    return (f'notify(#{effects["receiver"]}, "{DRAIN_PREFIX} {token}"); '
            f'return connection_option(#{effects["option"]}, "flush-command");')


# ---------------------------------------------------------------------------
# Shaped families (plan section 3 rows 3 and 4): authority and topology.
#
# Each transaction is one call to a verb on the family's per-generation ops
# object. The verb's owner is the acting nonwizard identity, so the call runs
# with that identity's permissions; the client connection is a fresh
# programmer (Test.db do_login_command) and the wizard only builds the objects
# and owns the chown control verb (Toast set_prop_info refuses a nonwizard
# owner change, property.cc:173-176). Every verb returns {reads, appends,
# observations}: one entry per read micro-op (its list, or an error literal),
# one per append (1, or the error literal of a denied append), and the values
# the family invariants check. An expected error is caught around exactly one
# builtin call or one property assignment, each of which has no effect when it
# raises; any other error escapes and the task is a native-error :info.

def _value(expr):
    """expr's value, or its error literal ("E_PERM") when it raises."""
    return f"(typeof(e = `{expr} ! ANY') == ERR ? toliteral(e) | e)"


def _caught(expr, ok='"ok"'):
    """ok when expr succeeds, else its error literal."""
    return f"(typeof(e = `{expr} ! ANY') == ERR ? toliteral(e) | {ok})"


def _dwell(dwell):
    """A tick loop between a transaction's reads and writes (--dwell), so
    concurrent transactions overlap on the real binary."""
    return [f"for dwell_i in [1..{dwell}]", "endfor"] if dwell > 0 else []


P_FLAGS = ("", "r", "w", "rw")   # p-mutate kinds 0-3; kind 4 is the chown control
AUTHORITY_ROLES = {"mp": 0, "d": 1, "mf": 2, "dsp": 3, "vx": 4}
TOPOLOGY_ROLES = {"cr1": 0, "cr2": 1, "ch1": 2, "ch2": 3, "created": 4, "recycled": 5, "moves": 6, "reparents": 7}
# name: (weight, micro-ops by role name). A transaction's shape is identified
# by its exact (function, role) sequence, so every shape of a family differs.
AUTHORITY_SHAPES = {
    "p-mutate": (3, [["r", "mp"], ["append", "mp"]]),
    "p-read": (2, [["r", "mp"], ["r", "d"]]),
    "p-write": (3, [["r", "mp"], ["r", "d"], ["append", "d"]]),
    "f-mutate": (3, [["r", "mf"], ["append", "mf"]]),
    "f-observe": (3, [["r", "mf"]]),
    "v-mutate": (2, [["r", "vx"], ["r", "dsp"], ["append", "dsp"]]),
    "v-observe": (3, [["r", "dsp"], ["append", "vx"]]),
    "v-read": (1, [["r", "dsp"], ["r", "vx"]]),
}
TOPOLOGY_SHAPES = {
    "t-create-1": (1, [["append", "cr1"], ["append", "ch1"]]),
    "t-create-2": (1, [["append", "cr2"], ["append", "ch2"]]),
    "t-cross-1": (2, [["r", "cr1"], ["append", "cr2"], ["append", "ch1"]]),
    "t-cross-2": (2, [["r", "cr2"], ["append", "cr1"], ["append", "ch2"]]),
    "t-read": (1, [["r", "cr1"], ["r", "cr2"], ["r", "ch1"], ["r", "ch2"]]),
    "c-create": (3, [["append", "created"]]),
    "c-move": (3, [["r", "moves"], ["append", "moves"]]),
    "c-reparent": (2, [["r", "reparents"], ["append", "reparents"]]),
    "c-recycle": (2, [["r", "recycled"], ["append", "recycled"]]),
    "c-observe": (3, [["r", "created"], ["r", "recycled"], ["r", "moves"], ["r", "reparents"]]),
}
SHAPED = {"authority": (AUTHORITY_ROLES, AUTHORITY_SHAPES), "topology": (TOPOLOGY_ROLES, TOPOLOGY_SHAPES)}


def shape_table(family):
    """The shapes Jepsen draws from (core.clj shaped-txns): roles as integers."""
    roles, shapes = SHAPED[family]
    return [{"name": name, "weight": weight, "mops": [[f, roles[role]] for f, role in mops]}
            for name, (weight, mops) in shapes.items()]


FAMILIES.update({"authority": {"base": 4 * FAMILY_SPAN, "shapes": shape_table("authority")},
                 "topology": {"base": 5 * FAMILY_SPAN, "shapes": shape_table("topology")}})


def identify_shape(family, txn):
    """The shape name of a shaped family's transaction, or ValueError."""
    roles, shapes = SHAPED[family]
    base = FAMILIES[family]["base"]
    signature = []
    for mop in txn:
        if not (isinstance(mop, list) and len(mop) == 3 and type(mop[1]) is int and family_of(mop[1]) == family):
            raise ValueError(f"{family} transaction holds a micro-operation {mop!r} outside the family")
        f, key, value = mop
        if not ((f == "append" and type(value) is int) or (f == "r" and value is None)):
            raise ValueError(f"malformed micro-operation {mop!r}")
        signature.append([f, key - base])
    for name, (_, mops) in shapes.items():
        if signature == [[f, roles[role]] for f, role in mops]:
            values = {value for f, _, value in txn if f == "append"}
            if len(values) > 1:
                raise ValueError(f"{family} transaction {txn!r} appends more than one value")
            return name
    raise ValueError(f"{family} transaction {txn!r} matches no shape")


# Identities are nonwizard programmers made by the wizard; "wizard" verbs are
# owned by the setup connection's (wizard) player.
def _identities(names):
    statements = []
    for name in names:
        statements += [f"{name} = create($nothing);", f"{name}.owner = {name};", f"set_player_flag({name}, 1);",
                       f"{name}.programmer = 1;"]
    return statements


def _verbs(owners, verbs):
    statements = []
    for name, (owner, lines) in verbs.items():
        who = owners[owner]
        statements.append(f'add_verb(o, {{{who}, "xd", "{name}"}}, {{"this", "none", "this"}});')
        statements.append(f'if (set_verb_code(o, "{name}", {_moo_lines(lines)})) return "{name} does not compile"; endif')
    return statements


def authority_verbs(dwell):
    """The authority verbs: name -> (identity, lines)."""
    d = _dwell(dwell)
    return {
        "p_flags": ("own", ["{v} = args;", "m = this.mp;", f"f = {_moo_lines(P_FLAGS)}[v % 5 + 1];", *d,
                            "r = " + _caught('set_property_info(this.po, "x", {this.own, f})') + ";",
                            "this.mp = {@m, v};", "return {{m}, {1}, {r}};"]),
        "p_chown": ("wizard", ["{v} = args;", "m = this.mp;", 'info = property_info(this.po, "x");',
                               "new = info[1] == this.own ? this.wr | this.own;", *d,
                               'set_property_info(this.po, "x", {new, info[2]});',
                               "this.mp = {@m, v};", "return {{m}, {1}, {toint(new)}};"]),
        "p_read": ("rd", ["m = this.mp;", "x = " + _value("this.po.x") + ";",
                          "c = " + _caught('set_property_info(this.po, "x", {this.rd, ""})') + ";",
                          "return {{m, x}, {}, {c}};"]),
        "p_write": ("wr", ["{v} = args;", "m = this.mp;", "x = " + _value("this.po.x") + ";", *d,
                           "if (typeof(x) == LIST)", "  a = " + _caught("this.po.x = {@x, v}", "1") + ";",
                           "else", "  a = x;", "endif", "return {{m, x}, {a}, {}};"]),
        "f_mutate": ("own", ["{v} = args;", "m = this.mf;", "k = v % 3;", *d,
                             "if (k == 0)",
                             '  if ("y" in properties(this.fa))',
                             "    r = " + _caught('delete_property(this.fa, "y")', '"deleted"') + ";",
                             "  else",
                             "    r = " + _caught('add_property(this.fa, "y", {v}, {this.own, "r"})', '"added"') + ";",
                             "  endif",
                             "elseif (k == 1)",
                             '  if (is_clear_property(this.fc, "z"))',
                             "    r = " + _caught("this.fc.z = {v}", '"set"') + ";",
                             "  else",
                             "    r = " + _caught('clear_property(this.fc, "z")', '"cleared"') + ";",
                             "  endif",
                             "else",
                             "  dest = parent(this.fc) == this.fa ? this.fb | this.fa;",
                             "  r = " + _caught("chparent(this.fc, dest)", "toint(dest)") + ";",
                             "endif",
                             "this.mf = {@m, v};", "return {{m}, {1}, {r}};"]),
        "f_observe": ("rd", ["m = this.mf;",
                             'y = "y" in properties(this.fa) ? 1 | 0;',
                             "fy = " + _value("this.fc.y") + ";", "fz = " + _value("this.fc.z") + ";",
                             "cz = " + _value('is_clear_property(this.fc, "z")') + ";",
                             "p = toint(parent(this.fc));",
                             "dy = " + _caught('delete_property(this.fa, "y")') + ";",
                             "cy = " + _caught('clear_property(this.fc, "y")') + ";",
                             "return {{m}, {}, {y, fy, fz, cz, p, dy, cy}};"]),
        "v_mutate": ("own", ["{v} = args;", "x = this.va.log;", "cur = this.vc:probe();",
                             'own = "probe" in verbs(this.vc);',
                             'code = {"return " + toliteral({@cur, v}) + ";"};', "k = v % 3;", *d,
                             "if (k == 0)",
                             '  set_verb_code(own ? this.vc | parent(this.vc), "probe", code);',
                             "elseif (k == 1 && own)",
                             '  set_verb_code(parent(this.vc), "probe", code);', '  delete_verb(this.vc, "probe");',
                             "elseif (k == 1)",
                             '  add_verb(this.vc, {this.own, "rxd", "probe"}, {"this", "none", "this"});',
                             '  set_verb_code(this.vc, "probe", code);',
                             "else",
                             "  dest = parent(this.vc) == this.va ? this.vb | this.va;",
                             '  set_verb_code(own ? this.vc | dest, "probe", code);', "  chparent(this.vc, dest);",
                             "endif",
                             "return {{x, cur}, {1}, {k}};"]),
        "v_observe": ("wr", ["{v} = args;", "cur = this.vc:probe();", *d, "this.va.log = {@this.va.log, v};",
                             "return {{cur}, {1}, {}};"]),
        "v_read": ("rd", ["return {{this.vc:probe(), this.va.log}, {}, {}};"]),
    }


AUTHORITY_LAYOUT = ("ops", "owner", "reader", "writer", "po", "fa", "fb", "fc", "va", "vb", "vc")


def authority_setup_code(dwell=0):
    """Wizard setup: identities, objects, markers and verbs; returns AUTHORITY_LAYOUT as integers."""
    statements = _identities(("own", "rd", "wr")) + [
        "o = create($nothing);",
        'po = create($nothing, own); add_property(po, "x", {}, {own, "rw"});',
        'fa = create($nothing, own); fa.r = 1; add_property(fa, "y", {0}, {own, "r"}); '
        'add_property(fa, "z", {-1}, {own, "r"});',
        'fb = create($nothing, own); add_property(fb, "z", {-2}, {own, "r"});',
        "fc = create(fa, own);",
        "va = create($nothing, own); vb = create($nothing, own);",
        'for q in ({va, vb}) add_verb(q, {own, "rxd", "probe"}, {"this", "none", "this"}); '
        'set_verb_code(q, "probe", {"return {};"}); endfor',
        "vc = create(va, own);",
        'add_property(va, "log", {}, {wr, "r"});',
        'add_property(o, "mp", {}, {own, "r"}); add_property(o, "mf", {}, {own, "r"});',
    ]
    for name in ("own", "rd", "wr", "po", "fa", "fb", "fc", "va", "vb", "vc"):
        statements.append(f'add_property(o, "{name}", {name}, {{player, "r"}});')
    statements += _verbs({"own": "own", "rd": "rd", "wr": "wr", "wizard": "player"}, authority_verbs(dwell))
    statements.append("return {toint(o), toint(own), toint(rd), toint(wr), toint(po), toint(fa), toint(fb), "
                      "toint(fc), toint(va), toint(vb), toint(vc)};")
    return " ".join(statements)


def topology_verbs(dwell):
    """The topology verbs: name -> (identity, lines). BUILDER mutates, READER observes."""
    d = _dwell(dwell)
    churn = "all = {@this.rooms[3].contents, @this.rooms[4].contents, @this.rooms[5].contents};"
    return {
        "tokens": ("rd", ["r = {};", "for x in (args[1])", "  r = {@r, x.tok};", "endfor", "return r;"]),
        "t_create": ("bld", ["{i, v} = args;", "x = create(this.tqs[i]);", "x.tok = v;", "move(x, this.rooms[i]);",
                             "return {{}, {1, 1}, {}};"]),
        "t_cross": ("bld", ["{i, j, v} = args;", "r = this:tokens(this.rooms[i].contents);", *d,
                            "x = create(this.tqs[i]);", "x.tok = v;", "move(x, this.rooms[j]);",
                            "return {{r}, {1, 1}, {}};"]),
        "t_read": ("rd", ["return {{this:tokens(this.rooms[1].contents), this:tokens(this.rooms[2].contents), "
                          "this:tokens(children(this.tqs[1])), this:tokens(children(this.tqs[2]))}, {}, {}};"]),
        "c_create": ("bld", ["{v} = args;", "x = create(this.tqs[3]);", "x.tok = v;", "move(x, this.rooms[3]);",
                             "this.created = {@this.created, v};", "return {{}, {1}, {toint(x)}};"]),
        "c_move": ("bld", ["{v} = args;", "m = this.moves;", churn, *d, "r = {};", "if (all)", "  x = all[1];",
                           "  dest = this.rooms[3 + v % 3];",
                           "  if (dest == x.location)", "    dest = this.rooms[3 + (v + 1) % 3];", "  endif",
                           "  move(x, dest);", "  r = {x.tok, toint(dest)};", "endif",
                           "this.moves = {@m, v};", "return {{m}, {1}, {r}};"]),
        "c_reparent": ("bld", ["{v} = args;", "m = this.reparents;", churn, *d, "r = {};", "if (all)",
                               "  x = all[1];", "  dest = parent(x) == this.tqs[3] ? this.tqs[4] | this.tqs[3];",
                               "  chparent(x, dest);", "  r = {x.tok, toint(dest)};", "endif",
                               "this.reparents = {@m, v};", "return {{m}, {1}, {r}};"]),
        "c_recycle": ("bld", ["{v} = args;", "g = this.recycled;", churn, *d, "r = {0, -1};", "if (all)",
                              "  x = all[1];", "  r = {x.tok, toint(x)};", "  recycle(x);", "endif",
                              "this.graves[v] = r;", "this.recycled = {@g, v};", "return {{g}, {1}, {r}};"]),
        "c_observe": ("rd", [
            "rooms = {};",
            "for t in ({this.rooms[3], this.rooms[4], this.rooms[5]})",
            "  items = {};",
            "  for x in (t.contents)",
            "    items = {@items, valid(x) ? {toint(x), 1, x.tok, toint(x.location), toint(parent(x)), "
            "`x:kind() ! ANY => 0'} | {toint(x), 0}};",
            "  endfor",
            "  rooms = {@rooms, items};",
            "endfor",
            "kids = {};",
            "for q in ({this.tqs[3], this.tqs[4]})",
            "  items = {};",
            "  for x in (children(q))",
            "    items = {@items, valid(x) ? {toint(x), 1, x.tok, toint(parent(x))} | {toint(x), 0}};",
            "  endfor",
            "  kids = {@kids, items};",
            "endfor",
            "graves = {};",
            "for v in (mapkeys(this.graves))",
            "  {t, n} = this.graves[v];",
            "  graves = {@graves, {v, t, n, valid(toobj(n)) && `toobj(n).tok ! ANY => 0' == t}};",
            "endfor",
            "return {{this.created, this.recycled, this.moves, this.reparents}, {}, {rooms, kids, graves}};"]),
    }


TOPOLOGY_LAYOUT = ("ops", "builder", "reader", "room_parent", "t1", "t2", "t3", "t4", "t5", "tq0", "tq1", "tq2",
                   "tq3", "tq4")


def topology_setup_code(dwell=0):
    """Wizard setup: identities, rooms, parents, ledgers and verbs; returns TOPOLOGY_LAYOUT as integers."""
    statements = _identities(("bld", "rd")) + [
        "o = create($nothing);",
        'rp = create($nothing, bld); add_verb(rp, {bld, "rxd", "accept"}, {"this", "none", "this"}); '
        'set_verb_code(rp, "accept", {"return 1;"});',
        "rooms = {}; for i in [1..5] rooms = {@rooms, create(rp, bld)}; endfor",
        'tq0 = create($nothing, bld); add_property(tq0, "tok", 0, {bld, "r"});',
        "tqs = {}; for i in [1..4] tqs = {@tqs, create(tq0, bld)}; endfor",
        'for i in [3..4] add_verb(tqs[i], {bld, "rxd", "kind"}, {"this", "none", "this"}); '
        'set_verb_code(tqs[i], "kind", {tostr("return ", i, ";")}); endfor',
        'add_property(o, "rooms", rooms, {player, "r"}); add_property(o, "tqs", tqs, {player, "r"});',
    ]
    for name in ("created", "recycled", "moves", "reparents"):
        statements.append(f'add_property(o, "{name}", {{}}, {{bld, "r"}});')
    statements.append('add_property(o, "graves", [], {bld, "r"});')
    statements += _verbs({"bld": "bld", "rd": "rd"}, topology_verbs(dwell))
    statements.append("ids = {toint(o), toint(bld), toint(rd), toint(rp)}; "
                      "for r in (rooms) ids = {@ids, toint(r)}; endfor ids = {@ids, toint(tq0)}; "
                      "for q in (tqs) ids = {@ids, toint(q)}; endfor return ids;")
    return " ".join(statements)


SETUP = {"authority": (authority_setup_code, AUTHORITY_LAYOUT), "topology": (topology_setup_code, TOPOLOGY_LAYOUT)}


def parse_shaped_layout(family, value):
    """A shaped family's setup reply: its layout object numbers by name."""
    names = SETUP[family][1]
    if not (isinstance(value, list) and len(value) == len(names) and _int_list(value)):
        raise ValueError(f"{family} setup did not return {len(names)} object numbers: {value!r}")
    return dict(zip(names, value))


def shaped_row_expr(family, key, layout):
    """A wizard expression reading one shaped row (campaign observations)."""
    objects = (layout or {}).get(family)
    if not objects:
        raise ValueError(f"{family} rows need the generation's {family} objects")
    role = key - FAMILIES[family]["base"]
    if family == "authority":
        rows = {0: f"#{objects['ops']}.mp", 1: f"#{objects['po']}.x", 2: f"#{objects['ops']}.mf",
                3: f"#{objects['vc']}:probe()", 4: f"#{objects['va']}.log"}
    else:
        ops = f"#{objects['ops']}"
        rows = {0: f"{ops}:tokens(#{objects['t1']}.contents)", 1: f"{ops}:tokens(#{objects['t2']}.contents)",
                2: f"{ops}:tokens(children(#{objects['tq1']}))", 3: f"{ops}:tokens(children(#{objects['tq2']}))",
                4: f"{ops}.created", 5: f"{ops}.recycled", 6: f"{ops}.moves", 7: f"{ops}.reparents"}
    if role not in rows:
        raise ValueError(f"{family} key {key} names no row")
    return rows[role]


def shaped_code(family, txn, layout):
    """The one verb call running a shaped transaction."""
    objects = (layout or {}).get(family)
    if not objects:
        raise ValueError(f"{family} micro-operations need the generation's {family} objects")
    shape = identify_shape(family, txn)
    v = next((value for f, _, value in txn if f == "append"), None)
    calls = {"p-mutate": f"p_chown({v})" if v is not None and v % 5 == 4 else f"p_flags({v})",
             "p-read": "p_read()", "p-write": f"p_write({v})", "f-mutate": f"f_mutate({v})",
             "f-observe": "f_observe()", "v-mutate": f"v_mutate({v})", "v-observe": f"v_observe({v})",
             "v-read": "v_read()",
             "t-create-1": f"t_create(1, {v})", "t-create-2": f"t_create(2, {v})",
             "t-cross-1": f"t_cross(1, 2, {v})", "t-cross-2": f"t_cross(2, 1, {v})", "t-read": "t_read()",
             "c-create": f"c_create({v})", "c-move": f"c_move({v})", "c-reparent": f"c_reparent({v})",
             "c-recycle": f"c_recycle({v})", "c-observe": "c_observe()"}
    return f"return #{objects['ops']}:{calls[shape]};"


def complete_shaped(family, txn, result):
    """Map a shaped verb's {reads, appends, observations} reply to a completion.

    The effective value keeps each read that returned a list and each append
    that returned 1; a read or append that returned an error literal had no
    effect and is listed under denied instead. Anything else is :info.
    """
    if not result.success:
        return {"type": "info", "error": native_error(result)}
    shape = identify_shape(family, txn)
    value = result.value
    reads = [mop for mop in txn if mop[0] == "r"]
    appends = [mop for mop in txn if mop[0] == "append"]
    if not (isinstance(value, list) and len(value) == 3 and isinstance(value[0], list) and len(value[0]) == len(reads)
            and isinstance(value[1], list) and len(value[1]) == len(appends) and isinstance(value[2], list)
            and all(_int_list(item) or isinstance(item, str) for item in value[0])
            and all(item == 1 or isinstance(item, str) for item in value[1])):
        return {"type": "info", "error": {"kind": "malformed-response", "value": repr(value)}}
    observed, written = iter(value[0]), iter(value[1])
    effective, denied = [], []
    for f, key, item in txn:
        outcome = next(observed) if f == "r" else next(written)
        if isinstance(outcome, str):
            denied.append([f, key, outcome])
        elif f == "r":
            effective.append([f, key, outcome])
        else:
            effective.append([f, key, item])
    return {"type": "ok", "value": effective, "denied": denied, "shape": shape, "observations": value[2]}


# ---------------------------------------------------------------------------
# Task lane (plan section 4): test-owned tasks held across a checkpoint and
# restart behind a MOO-held driver, released only after the recovery read.
# Semantics pinned on the managed Toast oracle by moo-conformance-tests
# server/restart_held_task_semantics.yaml (timed suspension, task-only anon
# and WAIF aliases, programmer ownership, verb edited while suspended, recycled
# reference, kill after restart); a delayed fork's locals and an indefinite
# suspend's resume-at-load by audit/task_scheduling_toast_oracle.yaml and
# server/restart_task_semantics.yaml. An indefinite suspend() resumes with 0 at
# load, so every held suspension is timed (HOLD_SECONDS) and released by
# resume(). A queued fork cannot be resumed, so its body first polls a gate.

HOLD_SECONDS = 3600
FORK_DELAY = 10
# Ceiling for every released continuation to record its result. Not measured;
# it covers FORK_DELAY.
RELEASE_DEADLINE = 45.0
TASK_KINDS = ("fork", "suspend", "values", "edit", "cancel", "recycle", "effect", "invalid")
# Released with resume(tid, {token, "wake"}); cancel is killed instead and
# fork passes its gate.
RESUMED_KINDS = ("suspend", "values", "edit", "recycle", "effect", "invalid")


def _moo_lines(lines):
    return "{" + ", ".join(json.dumps(line) for line in lines) + "}"


# Verbs on the scenario object. Owner "p" is the nonwizard programmer the
# tasks run as; "wizard" verbs are wizard-only builtins (task_local, and
# notify to another player) behind a helper.
TASK_VERBS = {
    "record": ("p", ["k = args[1];",
                     "this.results[k] = {@(maphaskey(this.results, k) ? this.results[k] | {}), args[2]};"]),
    "set_local": ("wizard", ["set_task_local(args[1]);"]),
    "get_local": ("wizard", ["return task_local();"]),
    "spawn": ("p", ["fork tid (0)", "  this:(args[1])(@listdelete(args, 1));", "endfork", "suspend(0);",
                    "return tid;"]),
    "fork_hold": ("p", ["tok = args[1];", "lv = {tok, \"fork-local\"};", f"fork tid ({FORK_DELAY})",
                        "  while (!this.gate)", "    suspend(1);", "  endwhile",
                        "  this:record(tok, {\"fork\", lv, toint(task_perms()), task_id()});", "endfork", "return tid;"]),
    "suspend_inner": ("p", ["inner_local = {args[1], \"inner\"};", f"v = suspend({HOLD_SECONDS});",
                            "return {v, inner_local, toint(task_perms())};"]),
    "suspend_body": ("p", ["marker = {args[1], \"outer\"};",
                           "this:record(args[1], {\"suspend\", marker, task_id(), this:suspend_inner(args[1])});"]),
    "values_body": ("p", ["tok = args[1];", "a = create(args[2], 1);", "a.tok = tok;", "b = {a, a};",
                          "w = args[3]:new();", "w.tok = tok;", "w2 = w;", "this:set_local({tok, a, w});",
                          f"v = suspend({HOLD_SECONDS});", "c = b[1];", "c.tok = tok + 1;", "w2.tok = tok + 2;",
                          "held = this:get_local();",
                          "this:record(tok, {\"values\", v, valid(a), typeof(a), a == b[1], b[1] == b[2], "
                          "a == held[2], a.tok, typeof(w), w == w2, w == held[3], w.tok, toint(task_perms())});"]),
    "edit_body": ("p", [f"v = suspend({HOLD_SECONDS});", "this:record(args[1], {\"edit\", \"old\", v});"]),
    "cancel_body": ("p", [f"v = suspend({HOLD_SECONDS});", "this:record(args[1], {\"cancel\", \"ran\", v});"]),
    # The task whose saved program the harness invalidates once (invalidate_task_fingerprint).
    "invalid_body": ("p", [f"v = suspend({HOLD_SECONDS});", "this:record(args[1], {\"invalid\", v});"]),
    "recycle_body": ("p", ["h = args[2];", f"v = suspend({HOLD_SECONDS});",
                           "this:record(args[1], {\"recycle\", valid(h), typeof(h), h == args[2], v});"]),
    "effect_body": ("wizard", [f"v = suspend({HOLD_SECONDS});",
                               f"notify(this.receiver, tostr(\"{TASK_EFFECT_PREFIX} \", args[1]));",
                               "this:record(args[1], {\"effect\", v});"]),
}
EDITED_CODE = ["this:record(args[1], {\"edit\", \"new\", args});"]


def task_setup_code():
    """Create the programmer, the scenario object, its anon parent and WAIF
    class, and every scenario verb; returns {p, o, ap, wc} as integers."""
    statements = ["p = create($nothing);", "p.owner = p;", "set_player_flag(p, 1);", "p.programmer = 1;",
                  "ap = create($nothing, p);", 'add_property(ap, "tok", 0, {p, "rw"});',
                  "wc = create($waif, p);", 'add_property(wc, ":tok", 0, {p, "rw"});',
                  "o = create($nothing, p);", 'add_property(o, "results", [], {p, "rw"});',
                  'add_property(o, "gate", 0, {p, "rw"});', 'add_property(o, "receiver", #-1, {p, "rw"});']
    for name, (owner, lines) in TASK_VERBS.items():
        who = "p" if owner == "p" else "player"
        statements.append(f'add_verb(o, {{{who}, "xd", "{name}"}}, {{"this", "none", "this"}});')
        statements.append(f'if (set_verb_code(o, "{name}", {_moo_lines(lines)})) return "{name} does not compile"; endif')
    statements.append("return {toint(p), toint(o), toint(ap), toint(wc)};")
    return " ".join(statements)


def task_spawn_code(setup, kind, token, extra=None):
    """Start one scenario task; returns its task id."""
    o = f"#{setup['object']}"
    if kind == "fork":
        return f"return {o}:fork_hold({token});"
    args = {"suspend": "", "edit": "", "cancel": "", "effect": "", "invalid": "",
            "values": f", #{setup['anon_parent']}, #{setup['waif_class']}",
            "recycle": f", #{extra}"}[kind]
    return f'return {o}:spawn("{kind}_body", {token}{args});'


def task_state_code(setup):
    """{queued task ids, the scenario object's results or 0 when it is not valid}."""
    o = f"#{setup['object']}"
    return ("ids = {}; for t in (queued_tasks()) ids = {@ids, t[1]}; endfor "
            f"return {{ids, valid({o}) ? {o}.results | 0}};")


def task_release_code(plant, receiver):
    """The driver: open the fork gate, point effect tasks at the receiver, kill
    cancel tasks and resume every suspension with {token, "wake"}."""
    o = f"#{plant['setup']['object']}"
    statements = [f"{o}.receiver = #{receiver};" if receiver is not None else "", f"{o}.gate = 1;"]
    for task in plant["tasks"]:
        if task.get("invalidated"):
            continue  # dropped at load; resume() would raise E_INVARG
        if task["kind"] == "cancel":
            statements.append(f"kill_task({task['tid']});")
        elif task["kind"] in RESUMED_KINDS:
            statements.append(f'resume({task["tid"]}, {{{task["token"]}, "wake"}});')
    statements.append("return 1;")
    return " ".join(statement for statement in statements if statement)


class Receiver:
    """A harness connection logged in as a fresh programmer (Test.db
    do_login_command), whose every later output line is journaled with its
    arrival time by a reader thread until the server closes it."""

    def __init__(self, transport_factory, port, on_line):
        self.transport = transport_factory(port)
        self.transport.connect("programmer")
        result = self.transport.execute("return toint(player);")
        if not result.success or type(result.value) is not int:
            self.transport.disconnect()
            raise RuntimeError(f"receiver login did not name its player: {result.value!r}")
        self.player = result.value
        self.on_line = on_line
        self.thread = None

    def start(self):
        sock = self.transport.sock
        sock.settimeout(None)

        def read():
            buffer = b""
            while True:
                try:
                    data = sock.recv(65536)
                except OSError:
                    break
                if not data:
                    break
                buffer += data
                while b"\n" in buffer:
                    line, buffer = buffer.split(b"\n", 1)
                    text = type(self.transport)._strip_telnet_commands(line).decode("utf-8", "replace").rstrip("\r")
                    self.on_line(text, time.monotonic_ns())

        self.thread = threading.Thread(target=read, daemon=True)
        self.thread.start()
        return self

    def close(self):
        self.transport.disconnect()
        if self.thread is not None:
            self.thread.join(5)


DROPPED_TASK_MESSAGE = "dropped suspended task: saved program does not match its source"


def invalidate_task_fingerprint(text, tid):
    """Flip one hex digit of suspended task tid's first saved program fingerprint.

    In a Barn-written checkpoint the task section starts with the line
    "<start> <tid> <wake value...>" after "N suspended tasks", and each saved
    frame's program identity list ends with its 64-hex-digit fingerprint,
    which precedes that frame's builtin-layout hex (db/format/writer_vm.go
    programIdentityValue). The first 64-hex line after the header is
    therefore the first frame's fingerprint. Barn's loader recompiles the
    frame's source, finds the fingerprint different and drops only that task
    (engine/task_load.go LoadSuspendedTasks). Returns (new text, line number).
    """
    lines = text.split("\n")
    start = next((index for index, line in enumerate(lines) if line.endswith(" suspended tasks")), None)
    if start is None:
        raise ValueError("checkpoint has no suspended tasks section")
    header = next((index for index in range(start + 1, len(lines))
                   if len(lines[index].split()) >= 2 and lines[index].split()[1] == str(tid)
                   and lines[index].split()[0].lstrip("-").isdigit()), None)
    if header is None:
        raise ValueError(f"suspended task {tid} is not in the checkpoint")
    target = next((index for index in range(header + 1, len(lines))
                   if len(lines[index]) == 64 and all(c in "0123456789abcdef" for c in lines[index])), None)
    if target is None:
        raise ValueError(f"suspended task {tid} has no saved program fingerprint")
    line = lines[target]
    lines[target] = ("1" if line[0] == "0" else "0") + line[1:]
    return "\n".join(lines), target + 1


def parse_task_state(value):
    if not (isinstance(value, list) and len(value) == 2 and _int_list(value[0])
            and (value[1] == 0 or isinstance(value[1], dict))):
        raise ValueError(f"malformed task state {value!r}")
    results = None if value[1] == 0 else {int(key): records for key, records in value[1].items()}
    return {"ids": value[0], "results": results}


def _int_list(value):
    return isinstance(value, list) and all(type(item) is int for item in value)


def native_error(result):
    error = result.error
    return {"kind": "native-error",
            "code": None if error is None else str(getattr(error, "value", error)),
            "message": result.error_message}


def complete_txn(txn, result):
    """Map a SocketTransport ExecutionResult for txn to a Jepsen completion.

    A native MOO error does not prove its writes rolled back, so it is :info
    with its code and message; so is a reply whose shape does not match txn.
    """
    if not result.success:
        return {"type": "info", "error": native_error(result)}
    reads = [mop for mop in txn if mop[0] == "r"]
    value = result.value
    if not (isinstance(value, list) and len(value) == len(reads) and all(_int_list(item) for item in value)):
        return {"type": "info", "error": {"kind": "malformed-response", "value": repr(value)}}
    observed = iter(value)
    return {"type": "ok", "value": [[f, key, next(observed)] if f == "r" else [f, key, item]
                                    for f, key, item in txn]}


def parse_ledger(value):
    """A MOO map of integer keys to integer lists, as SocketTransport parses it."""
    if not (isinstance(value, dict) and all(type(key) is int and _int_list(items) for key, items in value.items())):
        raise ValueError(f"ledger is not a map of integer lists: {value!r}")
    return value


def ledger_json(ledger):
    return {str(key): list(items) for key, items in sorted(ledger.items())}


def ledger_from_json(data):
    return {int(key): list(items) for key, items in data.items()}


class _EofWatch:
    """Socket proxy recording whether the peer closed during a read."""

    def __init__(self, sock):
        self.sock, self.eof = sock, False

    def recv(self, size):
        data = self.sock.recv(size)
        if not data:
            self.eof = True
        return data

    def __getattr__(self, name):
        return getattr(self.sock, name)


def recording_transport_class(base):
    """SocketTransport keeping each raw framed reply; EOF before the frame ends raises."""

    class RecordingTransport(base):
        last_raw = None

        def _receive(self):
            self.last_raw = None
            real = self.sock
            watch = self.sock = _EofWatch(real)
            try:
                raw = super()._receive()
            finally:
                self.sock = real
            self.last_raw = raw
            if watch.eof:
                raise ConnectionError("server closed the connection before the framed reply completed")
            return raw

    return RecordingTransport


def socket_transport_factory():
    from moo_conformance.transport import SocketTransport
    transport = recording_transport_class(SocketTransport)
    return lambda port: transport("127.0.0.1", port, ensure_standard_properties=False)


# ---------------------------------------------------------------------------
# Provenance: every input the verdict depends on

def _file_row(path):
    path = Path(path)
    if not path.is_file():
        return {"path": str(path), "sha256": None}
    return {"path": str(path.resolve()), "size_bytes": path.stat().st_size,
            "sha256": barn_process.bench.sha256_file(path)}


def _command_output(command, cwd=None):
    try:
        result = subprocess.run(command, cwd=cwd, capture_output=True, text=True, timeout=60, check=False)
    except (OSError, subprocess.SubprocessError) as error:
        return f"unavailable: {type(error).__name__}: {error}"
    return (result.stdout + result.stderr).strip() if result.returncode == 0 else \
        f"unavailable: exit {result.returncode}: {(result.stdout + result.stderr).strip()}"


def _filesystem(path):
    """The mount holding path, from /proc/self/mountinfo (None off Linux)."""
    try:
        lines = Path("/proc/self/mountinfo").read_text().splitlines()
    except OSError:
        return None
    best = None
    target = str(Path(path).resolve())
    for line in lines:
        fields = line.split()
        if " - " not in line or len(fields) < 5:
            continue
        mount = fields[4]
        if target == mount or target.startswith(mount.rstrip("/") + "/"):
            tail = line.split(" - ", 1)[1].split()
            if best is None or len(mount) > len(best["mount_point"]):
                best = {"mount_point": mount, "fstype": tail[0] if tail else None,
                        "source": tail[1] if len(tail) > 1 else None}
    return best


def provenance(args, output):
    db = args.db.resolve()
    try:
        import moo_conformance.transport as transport_module
        transport = _file_row(transport_module.__file__)
    except ImportError as error:
        transport = {"unavailable": str(error)}
    try:
        os_release = Path("/etc/os-release").read_text()
    except OSError:
        os_release = None
    affinity = getattr(os, "sched_getaffinity", None)
    return {
        "parameters": {name: getattr(args, name) for name in
                       ("phase1", "phase3", "phase_live", "rate", "concurrency", "seed", "jepsen_timeout",
                        "workloads", "stages", "stage_seconds", "control", "tasks", "dwell")},
        "task_lane": {"kinds": list(TASK_KINDS), "hold_seconds": HOLD_SECONDS, "fork_delay_seconds": FORK_DELAY,
                      "release_deadline_seconds": RELEASE_DEADLINE},
        "effects_lane": {"drain_deadline_seconds": DRAIN_DEADLINE},
        "stage_schedule": {name: STAGE_SCHEDULE[name] for name in args.stages},
        "live_checkpoints": {"deadline_seconds": CHECKPOINT_DEADLINE, "delay_span": DELAY_SPAN,
                             "first_delay_seconds": FIRST_DELAY_SECONDS},
        "families": {name: FAMILIES[name] for name in args.workloads},
        "jvm_heap": JVM_HEAP, "probe_deadline_seconds": PROBE_DEADLINE,
        "barn": _file_row(args.barn), "config": _file_row(CONFIG), "profile_id": PROFILE_ID,
        "source_tree": {"head": _command_output(["git", "rev-parse", "HEAD"], barn_process.ROOT),
                        "tree": _command_output(["git", "rev-parse", "HEAD^{tree}"], barn_process.ROOT),
                        "status": _command_output(["git", "status", "--porcelain"], barn_process.ROOT)},
        "checker_jar": _file_row(args.elle_jar),
        "jepsen": {"jar": _file_row(args.jepsen_jar),
                   "project_clj": _file_row(barn_process.ROOT / "tests" / "jepsen-checkpoint" / "project.clj")},
        "java": _command_output([args.java, "-version"]),
        "fixture": {"database": _file_row(db), "waif_identities": _file_row(str(db) + ".waifids"),
                    "conformance_revision": _command_output(["git", "rev-parse", "HEAD"], db.parent),
                    "transport": transport},
        "runner": {"uname": platform.uname()._asdict(), "os_release": os_release,
                   "filesystem": _filesystem(output), "cpu_count": os.cpu_count(),
                   "cpu_affinity": len(affinity(0)) if affinity else None,
                   "environment": {key: os.environ.get(key) for key in
                                   ("RUNNER_NAME", "RUNNER_OS", "RUNNER_ENVIRONMENT", "GITHUB_RUN_ID",
                                    "GITHUB_RUN_ATTEMPT", "GITHUB_SHA", "GOMAXPROCS", "GOGC", "GOMEMLIMIT")}},
    }


# ---------------------------------------------------------------------------
# Lifecycle owner: one OwnedBarnServer per process generation

def free_port():
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        return probe.getsockname()[1]


class Campaign:
    def __init__(self, executable, source_db, output, transport_factory, *, server_class=None, port_source=free_port,
                 probe_deadline=PROBE_DEADLINE, seed=1, stages=(), control=None, tasks=False, effects=False,
                 receiver_class=Receiver, release_deadline=RELEASE_DEADLINE, shaped=(), dwell=0):
        self.executable, self.output = executable, output
        self.shaped = list(shaped)          # selected shaped families: build their objects at every launch
        self.dwell = dwell                  # tick loop between a shaped verb's reads and writes
        self.tasks = tasks                  # task lane: plant held tasks before checkpoints
        self.effects = effects              # effects family selected: drain the receiver at observations
        self.receiver_class = receiver_class
        self.release_deadline = release_deadline
        self.plants = []                    # every task-plant record (task lane)
        self.live_planted = set()           # generations whose live lane planted a set
        self.invalidated = False            # the invalid-task policy check ran (invalidate)
        self.connections = []               # this generation's receiver and option connections
        self.drains = 0
        self.receipts_lock = threading.Lock()
        self.receipts_path = output / "receipts.jsonl"
        self.stages = list(stages)          # STAGE_SCHEDULE steps, armed one per generation in order
        self.next_stage = 0
        self.armed = {}                     # generation -> the step its stop spec selects
        self.control = control              # CONTROLS name: BARN_ELLE_FAULT for every process
        self.random = random.Random(seed)   # checkpoint-crash delays only
        self.checkpoint_seq = 0
        self.last_checkpoint_seconds = None
        self.transport_factory = transport_factory
        self.server_class = server_class or (lambda *args, **kwargs: barn_process.OwnedBarnServer(
            *args, config=CONFIG, profile_id=PROFILE_ID, **kwargs))
        self.port_source = port_source
        self.probe_deadline = probe_deadline
        self.lock = threading.Lock()        # guards live, touched, fault_seq, active_fault
        self.lifecycle = threading.Lock()   # serializes faults and launches
        self.journal_lock = threading.Lock()
        self.live = None                    # (generation, port) while clients may send
        self.touched = {}                   # generation -> every row key a client sent
        self.layouts = {}                   # generation -> its row objects (parse_layout)
        self.fault_seq, self.active_fault = 0, None
        self.probes = 0
        self.server = None
        self.generation = -1
        self.failures = []
        # The launch source: the original fixture, then each verified pair.
        self.source = {"database": str(source_db.resolve()),
                       "database_sha256": barn_process.bench.sha256_file(source_db),
                       "waif_identities": None, "waif_identities_sha256": None,
                       "baseline": None, "baseline_generation": None}
        self.log_path = output / "lifecycle.jsonl"
        self.journal_path = output / "journal.jsonl"

    def record(self, row):
        row = dict(row, monotonic_ns=time.monotonic_ns())
        with self.log_path.open("a", encoding="utf-8") as stream:
            stream.write(json.dumps(row, default=str) + "\n")

    def journal(self, row):
        """Append one adapter event durably before the caller acts on it."""
        line = json.dumps(dict(row, monotonic_ns=time.monotonic_ns()), default=str) + "\n"
        with self.journal_lock, self.journal_path.open("a", encoding="utf-8") as stream:
            stream.write(line)
            stream.flush()
            os.fsync(stream.fileno())

    def receipt(self, generation, line, received_ns):
        """Journal one line the receiver connection of generation got."""
        row = json.dumps({"generation": generation, "line": line, "monotonic_ns": received_ns}) + "\n"
        with self.receipts_lock, self.receipts_path.open("a", encoding="utf-8") as stream:
            stream.write(row)
            stream.flush()

    def phase(self):
        """Fault phase for a journal record; the caller holds self.lock."""
        return {"fault_seq": self.fault_seq, "fault": self.active_fault,
                "paused": bool(self.server is not None and self.server.paused)}

    def touch(self, generation, keys):
        with self.lock:
            self.touched.setdefault(generation, set()).update(keys)

    def fail(self, event, error):
        message = f"{event}: {type(error).__name__}: {error}"
        self.failures.append(message)
        self.record({"event": "failure", "operation": event, "error": message})
        return message

    def evaluate(self, port, code):
        transport = self.transport_factory(port)
        try:
            transport.connect("wizard")
            result = transport.execute(code)
        finally:
            transport.disconnect()
        if not result.success:
            raise RuntimeError(f"lifecycle evaluation failed: {native_error(result)}; raw={transport.last_raw!r}")
        return result.value

    def bounded(self, attempt):
        """Retry attempt() on fresh connections until it succeeds or the deadline passes."""
        started, errors = time.monotonic(), []
        while True:
            try:
                return True, attempt(), errors, time.monotonic() - started
            except Exception as error:
                errors.append(f"{type(error).__name__}: {error}")
                if time.monotonic() - started >= self.probe_deadline:
                    return False, None, errors, time.monotonic() - started
                time.sleep(0.5)

    def probe(self, reason):
        """Require useful work after a heal or restart: a fresh write, then a fresh read."""
        generation, (_, port) = self.generation, self.live
        self.probes += 1
        token = self.probes

        def attempt():
            written = self.evaluate(port, probe_write_code(token))
            read = self.evaluate(port, PROBE_READ_CODE)
            if type(written) is not int or type(read) is not int:
                raise RuntimeError(f"probe replies are not integers: {written!r}, {read!r}")
            return {"written": written, "read": read}

        ok, value, errors, seconds = self.bounded(attempt)
        self.record({"event": "probe", "generation": generation, "reason": reason, "token": token, "ok": ok,
                     "value": value, "failed_attempts": errors, "seconds": seconds,
                     "deadline_seconds": self.probe_deadline})

    def observe(self, reason):
        """Read every row any client touched in this generation, retired keys included."""
        generation = self.generation
        with self.lock:
            keys, live = sorted(self.touched.get(generation, ())), self.live
        row = {"event": "observation", "generation": generation, "reason": reason, "keys": keys}
        if not keys:
            row.update(ok=True, value=[])
        elif live is None or self.server.paused:
            row.update(ok=False, skipped="generation not live and running")
        else:
            def attempt():
                value = self.evaluate(live[1], observation_code(keys, self.layouts.get(generation)))
                if not (isinstance(value, list) and len(value) == len(keys) and all(_int_list(v) for v in value)):
                    raise RuntimeError(f"malformed observation {value!r}")
                return value
            ok, value, errors, seconds = self.bounded(attempt)
            row.update(ok=ok, value=value, failed_attempts=errors, seconds=seconds,
                       deadline_seconds=self.probe_deadline)
        self.record(row)
        if self.effects:
            self.drain(reason)

    def drain(self, reason):
        """Effects lane: notify a sentinel to the receiver and wait for it. The
        receiver's output is FIFO and a flushed notify is written before its
        task replies (server/connection.go SendNotification), so the sentinel's
        arrival proves every earlier completed notify arrived. Also records the
        option connection's flush-command and Barn's commit retry counter
        (coverage only)."""
        generation = self.generation
        with self.lock:
            live = self.live
        effects = (self.layouts.get(generation) or {}).get("effects")
        self.drains += 1
        token = f"{generation} {self.drains}"
        row = {"event": "effect-drain", "generation": generation, "reason": reason, "token": token}
        if live is None or effects is None or self.server.paused:
            row.update(ok=False, skipped="generation not live and running with an effects receiver")
            self.record(row)
            return
        sent_ns = time.monotonic_ns()
        try:
            row["option"] = self.evaluate(live[1], drain_code(effects, token))
        except Exception as error:
            row.update(ok=False, error=f"{type(error).__name__}: {error}")
            self.record(row)
            return
        expected, deadline = f"{DRAIN_PREFIX} {token}", time.monotonic() + DRAIN_DEADLINE
        received = None
        while received is None and time.monotonic() < deadline:
            with self.receipts_lock:
                lines = self.receipts_path.read_text(encoding="utf-8").splitlines() \
                    if self.receipts_path.exists() else []
            received = next((json.loads(line)["monotonic_ns"] for line in reversed(lines)
                             if json.loads(line)["line"] == expected and json.loads(line)["generation"] == generation),
                            None)
            if received is None:
                time.sleep(0.05)
        row.update(ok=received is not None, sent_ns=sent_ns, received_ns=received,
                   deadline_seconds=DRAIN_DEADLINE, commit_retries=self.commit_retries())
        self.record(row)

    def commit_retries(self):
        """barn.commit_retries from the owned debug endpoint, or None."""
        address = ((getattr(self.server, "identity", None) or {}).get("endpoints") or {}).get("debug")
        if not address:
            return None
        try:
            import urllib.request
            with urllib.request.urlopen(f"http://{address}/debug/vars", timeout=5) as response:
                return json.loads(response.read().decode("utf-8")).get("barn.commit_retries")
        except Exception:
            return None

    # -- task lane ---------------------------------------------------------

    def plant(self, point):
        """Create one scenario set of held tasks and verify every task is queued
        with no result before the caller checkpoints."""
        generation, (_, port) = self.generation, self.live
        seq = len(self.plants) + 1
        value = self.evaluate(port, task_setup_code())
        if not (isinstance(value, list) and len(value) == 4 and _int_list(value)):
            raise RuntimeError(f"task setup failed: {value!r}")
        setup = dict(zip(("programmer", "object", "anon_parent", "waif_class"), value))
        recycled = self.evaluate(port, "return toint(create($nothing));")
        tasks = []
        for index, kind in enumerate(TASK_KINDS):
            token = 1000000 + seq * 100 + index
            tid = self.evaluate(port, task_spawn_code(setup, kind, token, recycled))
            if type(tid) is not int:
                raise RuntimeError(f"task {kind} did not start: {tid!r}")
            tasks.append({"kind": kind, "token": token, "tid": tid})
        o = f"#{setup['object']}"
        self.evaluate(port, f'set_verb_code({o}, "edit_body", {_moo_lines(EDITED_CODE)}); '
                            f"recycle(#{recycled}); return 1;")
        state = parse_task_state(self.evaluate(port, task_state_code(setup)))
        plant = {"seq": seq, "point": point, "generation": generation, "setup": setup, "tasks": tasks,
                 "recycled": recycled, "verified": state, "planted_ns": time.monotonic_ns()}
        self.record(dict(plant, event="task-plant"))
        missing = [task["tid"] for task in tasks if task["tid"] not in state["ids"]]
        if missing or state["results"]:
            raise RuntimeError(f"task plant {seq} is not held: tasks {missing} not queued, results {state['results']}")
        self.plants.append(plant)
        return plant

    def task_recovery(self, generation, port):
        """Before admission and before any release: every plant's held state in
        the database this generation loaded. Returns the plants still held."""
        held = []
        for plant in self.plants:
            def attempt(plant=plant):
                return parse_task_state(self.evaluate(port, task_state_code(plant["setup"])))
            ok, state, errors, seconds = self.bounded(attempt)
            tids = [task["tid"] for task in plant["tasks"] if not task.get("invalidated")]
            present = [tid for tid in tids if ok and tid in state["ids"]]
            if not ok or state["results"] is None:
                status = "absent" if ok else "unreadable"
            elif len(present) == len(tids) and not state["results"]:
                status = "held"
            elif not present:
                status = "completed"
            else:
                status = "partial"
            self.record({"event": "task-recovery", "generation": generation, "plant": plant["seq"], "ok": ok,
                         "state": state, "present": present, "status": status, "failed_attempts": errors,
                         "seconds": seconds})
            if status == "held":
                held.append(plant)
        return held

    def open_connections(self, generation, port):
        """The generation's receiver (journaling to receipts.jsonl) and option
        connection, each a fresh programmer."""
        receiver = self.receiver_class(self.transport_factory, port,
                                       lambda line, ns: self.receipt(generation, line, ns)).start()
        self.connections.append(receiver)
        option = self.receiver_class(self.transport_factory, port, lambda line, ns: None).start()
        self.connections.append(option)
        self.record({"event": "effect-connections", "generation": generation, "receiver": receiver.player,
                     "option": option.player})
        return {"receiver": receiver.player, "option": option.player}

    def close_connections(self):
        for connection in self.connections:
            try:
                connection.close()
            except Exception:
                pass
        self.connections = []

    def release(self, generation, port, plants, receiver):
        """The driver, run after the recovery read: release every held plant and
        wait until each released continuation recorded its result and the
        plant's tasks left the queue."""
        for plant in plants:
            self.evaluate(port, task_release_code(plant, receiver))
            released_ns = time.monotonic_ns()
            expected = {task["token"] for task in plant["tasks"]
                        if task["kind"] != "cancel" and not task.get("invalidated")}
            tids = {task["tid"] for task in plant["tasks"] if not task.get("invalidated")}
            deadline, state = time.monotonic() + self.release_deadline, None
            while True:
                state = parse_task_state(self.evaluate(port, task_state_code(plant["setup"])))
                done = state["results"] is not None and expected <= set(state["results"]) \
                    and not tids & set(state["ids"])
                if done or time.monotonic() >= deadline:
                    break
                time.sleep(0.25)
            self.record({"event": "task-release", "generation": generation, "plant": plant["seq"],
                         "released_ns": released_ns, "receiver": receiver, "complete": done, "state": state,
                         "deadline_seconds": self.release_deadline})

    @staticmethod
    def _copy_pair(database, sidecar, sha256, sidecar_sha256, target):
        shutil.copyfile(database, target)
        if barn_process.bench.sha256_file(target) != sha256:
            raise RuntimeError("generation database copy does not match its verified source")
        if sidecar is not None:
            copied = Path(str(target) + ".waifids")
            shutil.copyfile(sidecar, copied)
            if barn_process.bench.sha256_file(copied) != sidecar_sha256:
                raise RuntimeError("generation WAIF identity copy does not match its verified source")

    def launch(self, kind, restart=None):
        """Start the next generation.

        Without restart, from an exact copy of the launch source. With a
        checkpoint-crash restart, from the interrupted checkpoint's actual
        output path in place (Barn's loader recovers its journal), or from a
        copy of the unchanged input when no checkpoint output existed.
        """
        self.close_connections()
        self.generation += 1
        generation, source = self.generation, self.source
        run_dir = self.output / f"gen-{generation:03d}"
        run_dir.mkdir()
        database = run_dir / "input.db"
        extra = {}
        if restart is None:
            self._copy_pair(source["database"], source["waif_identities"], source["database_sha256"],
                            source["waif_identities_sha256"], database)
        else:
            extra["recovered_input"] = restart
            if restart["kind"] == "interrupted-checkpoint-output":
                database = Path(restart["database"])
            else:
                pair = restart["input_pair"]
                sidecar = Path(restart["database"] + ".waifids") if pair["waif_identities"] else None
                self._copy_pair(restart["database"], sidecar, pair["database"], pair["waif_identities"], database)
        if restart is not None and restart.get("recovery_stop"):
            self.recovery_stop(generation, database, restart)
        environment = self.environment()
        if self.next_stage < len(self.stages):
            step = self.stages[self.next_stage]
            self.next_stage += 1
            self.armed[generation] = step
            environment[STOP_ENV] = f"{STAGE_SCHEDULE[step]['point']}:{STAGE_SCHEDULE[step]['occurrence']}"
        if environment:
            extra["environment"] = environment
        port = self.port_source()
        server = self.server_class(Path(self.executable), database, run_dir, port, **extra)
        self.server = server
        server.start()
        self.observe_recovery(generation, port, restart)
        held = self.task_recovery(generation, port) if self.plants else []
        effects = self.open_connections(generation, port) if (self.tasks or self.effects) else None
        if held:
            self.release(generation, port, held, effects["receiver"])
        recovered = parse_ledger(self.evaluate(port, ADMIT_CODE))
        layout = parse_layout(self.evaluate(port, ROWS_CODE))
        if self.effects:
            effects_object = self.evaluate(port, EFFECTS_CODE)
            if type(effects_object) is not int:
                raise RuntimeError(f"effects object was not created: {effects_object!r}")
            layout["effects"] = dict(effects, object=effects_object)
        for family in self.shaped:
            layout[family] = parse_shaped_layout(family, self.evaluate(port, SETUP[family][0](self.dwell)))
        if self.evaluate(port, epoch_code(generation)) != generation:
            raise RuntimeError(f"epoch marker of generation {generation} was not written")
        self.record({"event": "admit", "generation": generation, "kind": kind, "port": port,
                     "source": {key: value for key, value in source.items() if key != "baseline"},
                     "baseline_generation": source["baseline_generation"],
                     "baseline": None if source["baseline"] is None else ledger_json(source["baseline"]),
                     "recovered": ledger_json(recovered), "rows": layout, "restart": restart,
                     "stage": self.armed.get(generation), "server": server.identity})
        with self.lock:
            self.layouts[generation] = layout
            self.live = (generation, port)
        self.probe("launch")
        return generation

    def observe_recovery(self, generation, port, restart):
        """Before admission and before any client can send: the epoch marker, the
        queued task count and, after a checkpoint-crash, every row the crashed
        generation touched (plan section 2: R before admission resets rows)."""
        crashed = None if restart is None else restart["generation"]
        with self.lock:
            keys = sorted(self.touched.get(crashed, ())) if crashed is not None else []
            layout = self.layouts.get(crashed)
        row = {"event": "recovery-observation", "generation": generation, "observed_generation": crashed,
               "keys": keys}

        def attempt():
            return parse_recovery_observation(
                self.evaluate(port, recovery_observation_code(crashed, keys, layout)), keys, crashed)

        ok, value, errors, seconds = self.bounded(attempt)
        row.update(ok=ok, failed_attempts=errors, seconds=seconds, deadline_seconds=self.probe_deadline,
                   **(value or {}))
        self.record(row)
        if not ok:
            raise RuntimeError(f"recovery observation of generation {generation} failed: {errors[-1:]}")

    def _quiesce(self):
        with self.lock:
            live, self.live = self.live, None
        return live

    def pause(self):
        state = self.server.pause()
        self.record({"event": "pause", "generation": self.generation, "observed_state": state})
        return {"generation": self.generation}

    def resume(self):
        if self.server.paused:
            self.server.resume()
        self.record({"event": "resume", "generation": self.generation})
        self.probe("resume")
        return {"generation": self.generation}

    def crash(self):
        generation = self.generation
        self.observe("crash")
        self._quiesce()
        result = self.server.crash()
        self.record({"event": "crash", "generation": generation, "result": result})
        return {"crashed": generation, "generation": self.launch("crash")}

    def checkpoint(self):
        generation = self.generation
        if self.server.paused:
            self.server.resume()
        self.observe("checkpoint")
        if self.tasks:
            self.plant("graceful-checkpoint")
        _, port = self.live
        baseline = parse_ledger(self.evaluate(port, LEDGER_CODE))
        self._quiesce()
        result = self.server.stop()
        self.record({"event": "checkpoint", "generation": generation, "baseline": ledger_json(baseline),
                     "result": result})
        verdict = result["final_checkpoint"]
        if not verdict.get("verified"):
            raise RuntimeError(f"checkpoint of generation {generation} was not verified: {verdict.get('reason')}")
        outputs = verdict["outputs"]
        self.source = {"database": outputs["database"]["path"],
                       "database_sha256": outputs["database"]["sha256"],
                       "waif_identities": outputs["waif_identities"]["path"],
                       "waif_identities_sha256": outputs["waif_identities"]["sha256"],
                       "baseline": baseline, "baseline_generation": generation}
        if self.tasks and not self.invalidated:
            self.invalidate(generation)
        return {"checkpointed": generation, "generation": self.launch("checkpoint")}

    def invalidate(self, generation):
        """Once per campaign: copy the verified pair, flip the saved program
        fingerprint of the latest plant's "invalid" task, rewrite the sidecar's
        database digest header to match, and make that pair the launch source.
        Barn must drop exactly that task and restore the rest (plan section 4:
        a deliberately invalid task is not a lost valid task)."""
        plant = self.plants[-1]
        task = next(item for item in plant["tasks"] if item["kind"] == "invalid")
        directory = self.output / f"gen-{generation:03d}-invalidated"
        directory.mkdir()
        database, sidecar = directory / "input.db", directory / "input.db.waifids"
        # Bytes throughout: no newline translation inside saved strings.
        original = Path(self.source["database"]).read_bytes().decode("utf-8", "surrogateescape")
        text, line = invalidate_task_fingerprint(original, task["tid"])
        database.write_bytes(text.encode("utf-8", "surrogateescape"))
        digest = barn_process.bench.sha256_file(database)
        header, _, rest = Path(self.source["waif_identities"]).read_bytes().partition(b"\n")
        if not header.startswith(b"barn-waif-identities-v1 "):
            raise RuntimeError(f"unexpected WAIF identity sidecar header {header!r}")
        sidecar.write_bytes(f"barn-waif-identities-v1 {digest}\n".encode() + rest)
        before = dict(self.source)
        self.source.update(database=str(database), database_sha256=digest, waif_identities=str(sidecar),
                           waif_identities_sha256=barn_process.bench.sha256_file(sidecar))
        task["invalidated"] = True
        self.invalidated = True
        self.record({"event": "task-invalidate", "generation": generation, "plant": plant["seq"], "tid": task["tid"],
                     "token": task["token"], "line": line,
                     "from": {key: before[key] for key in ("database", "database_sha256", "waif_identities",
                                                           "waif_identities_sha256")},
                     "to": {key: self.source[key] for key in ("database", "database_sha256", "waif_identities",
                                                              "waif_identities_sha256")}})

    def _request_checkpoint(self, purpose, **extra):
        """SIGALRM one checkpoint; the previous one was acknowledged or its process is gone."""
        if self.server.paused:
            self.server.resume()
        self.checkpoint_seq += 1
        boundary = self.server.request_checkpoint()
        self.record(dict({"event": "checkpoint-request", "generation": self.generation, "seq": self.checkpoint_seq,
                          "purpose": purpose, "boundary": boundary}, **extra))
        return self.checkpoint_seq, boundary

    def live_checkpoint(self):
        """Checkpoint while clients continue; acknowledged by `checkpoint complete` and the published pair."""
        generation = self.generation
        if self.tasks and not self.stages and generation not in self.live_planted:
            # Held through this checkpoint A and the generation's checkpoint-crash,
            # so recovery runs the real journal path (task_checks: present iff the
            # selected cut is A or F).
            if self.server.paused:
                self.server.resume()
            self.live_planted.add(generation)
            self.plant("live-checkpoint")
        seq, boundary = self._request_checkpoint("live")
        result = self.server.wait_checkpoint(boundary, CHECKPOINT_DEADLINE)
        self.record(dict(result, event="checkpoint-" + result["status"], generation=generation, seq=seq))
        if result["status"] != "complete":
            raise RuntimeError(f"live checkpoint {seq} of generation {generation} failed: {result['failed']}")
        self.last_checkpoint_seconds = (result["duration_ms"] or 0) / 1000
        return {"checkpoint": seq, "generation": generation}

    def checkpoint_crash(self):
        """Request a checkpoint, stop and kill the owned process at a seeded random
        moment, classify the frozen files, and restart from the actual artifacts."""
        generation = self.generation
        if self.server.paused:
            self.server.resume()
        self.observe("checkpoint-crash")
        span = DELAY_SPAN * (self.last_checkpoint_seconds if self.last_checkpoint_seconds else FIRST_DELAY_SECONDS)
        delay = self.random.uniform(0, span)
        seq, boundary = self._request_checkpoint("crash", delay_seconds=delay, delay_span_seconds=span)
        time.sleep(delay)
        self._quiesce()
        state = self.server.pause()
        return self._kill_and_restart(generation, seq, boundary, {"observed_state": state})

    def stage_crash(self):
        """The armed schedule step: complete occurrence-1 live checkpoints, request
        one more, wait until the process stops itself at the step's point (its
        boundary record and /proc state T), then kill and restart as checkpoint_crash."""
        generation = self.generation
        step = self.armed.get(generation)
        if step is None:
            raise RuntimeError(f"generation {generation} has no armed stage step")
        spec = STAGE_SCHEDULE[step]
        if self.server.paused:
            self.server.resume()
        for _ in range(spec["occurrence"] - 1):
            self.live_checkpoint()
        self.observe("checkpoint-crash")
        requested = f"{spec['point']}:{spec['occurrence']}"
        seq, boundary = self._request_checkpoint("stage", stage=step, requested_stop=requested)
        try:
            stopped = self.server.wait_stop(boundary, requested, STOP_DEADLINE)
        except Exception as error:
            self.record({"event": "stage-unreached", "generation": generation, "seq": seq, "stage": step,
                         "requested_stop": requested, "error": f"{type(error).__name__}: {error}"})
            raise
        self._quiesce()
        return self._kill_and_restart(generation, seq, boundary, dict(stopped, stage=step),
                                      recovery_stop=spec.get("recovery"))

    def _kill_and_restart(self, generation, seq, boundary, stop, recovery_stop=None):
        """The owned process is stopped: classify the frozen files, SIGKILL it,
        preserve the artifacts and restart from them."""
        stopped_ns = time.monotonic_ns()
        database = self.server.db
        artifacts = barn_process.checkpoint_artifacts(database)
        try:
            log = self.server.checkpoint_log(boundary)
        except Exception as error:
            log = {"unavailable": f"{type(error).__name__}: {error}"}
        self.record(dict(stop, event="checkpoint-stop", generation=generation, seq=seq, stopped_ns=stopped_ns,
                         artifacts=artifacts, log=log))
        result = self.server.crash()
        forensic = barn_process.preserve_artifacts(database, self.output / f"gen-{generation:03d}-forensic")
        if artifacts["output"] is not None or artifacts["journal"] is not None:
            restart = {"kind": "interrupted-checkpoint-output", "database": str(Path(str(database) + ".new"))}
        else:
            restart = {"kind": "no-checkpoint-output", "database": str(database),
                       "input_pair": barn_process.pair_digest(database)}
        restart.update(generation=generation, seq=seq, stage=artifacts["stage"], crashed_database=str(database))
        if recovery_stop is not None and restart["kind"] == "interrupted-checkpoint-output":
            restart["recovery_stop"] = recovery_stop
        self.record({"event": "checkpoint-crash", "generation": generation, "seq": seq, "result": result,
                     "forensic": forensic, "restart": restart})
        return {"crashed": generation, "generation": self.launch("checkpoint-crash", restart)}

    def environment(self):
        """Variables every owned process of this campaign gets (the control's fault)."""
        return {FAULT_ENV: self.control} if self.control else {}

    def recovery_stop(self, generation, database, restart):
        """Double crash: start Barn on the interrupted output with a stop between
        the restore renames of its startup recovery, kill it there, and keep the
        second artifact set; the caller then recovers the same files again."""
        run_dir = self.output / f"gen-{generation:03d}-recovery-stop"
        run_dir.mkdir()
        environment = dict(self.environment(), **{STOP_ENV: restart["recovery_stop"]})
        server = self.server_class(Path(self.executable), database, run_dir, self.port_source(),
                                   recovered_input=restart, environment=environment)
        self.server = server
        stopped = server.start_until_stop(restart["recovery_stop"], STOP_DEADLINE)
        crashed = Path(restart["crashed_database"])
        artifacts = barn_process.checkpoint_artifacts(crashed)
        result = server.crash()
        forensic = barn_process.preserve_artifacts(crashed, self.output / f"gen-{generation:03d}-recovery-forensic")
        self.record(dict(stopped, event="recovery-stop", generation=generation,
                         crashed_generation=restart["generation"], seq=restart["seq"], artifacts=artifacts,
                         result=result, forensic=forensic, server=server.identity))

    def fault(self, name):
        with self.lifecycle:
            if self.server is None or self.failures:
                return {"type": "info", "error": {"kind": "campaign-failed", "failures": self.failures}}
            with self.lock:
                self.fault_seq += 1
                seq, self.active_fault = self.fault_seq, name
            self.journal({"event": "fault-begin", "fault_seq": seq, "f": name, "generation": self.generation})
            try:
                reply = {"type": "ok", "value": getattr(self, name.replace("-", "_"))(),
                         "generation": self.generation}
            except Exception as error:
                self._quiesce()
                reply = {"type": "info", "error": {"kind": "lifecycle", "message": self.fail(name, error)},
                         "generation": self.generation}
            finally:
                with self.lock:
                    self.active_fault = None
            self.journal({"event": "fault-end", "fault_seq": seq, "f": name, "type": reply["type"],
                          "generation": self.generation})
            return reply

    def finish(self):
        with self.lifecycle:
            self._quiesce()
            self.close_connections()
            if self.server is None or self.server.proc is None or self.server.proc.poll() is not None:
                return
            try:
                if self.server.paused:
                    self.server.resume()
                self.record({"event": "final-stop", "generation": self.generation, "result": self.server.stop()})
            except Exception as error:
                self.fail("final-stop", error)


class ClientSession:
    """One Jepsen client connection: a SocketTransport bound to one generation."""

    def __init__(self, campaign):
        self.campaign, self.transport, self.generation = campaign, None, None
        # Shaped families run from a fresh programmer connection, so the
        # wizard runs no workload code (shaped_code).
        self.shaped_transport, self.shaped_generation = None, None

    def close(self):
        if self.transport is not None:
            self.transport.disconnect()
        self.transport = self.generation = None
        self.close_shaped()

    def close_shaped(self):
        if self.shaped_transport is not None:
            self.shaped_transport.disconnect()
        self.shaped_transport = self.shaped_generation = None

    def txn(self, op_id, client, txn):
        """Journal the request, run it, and journal the completion before replying."""
        self.campaign.journal({"event": "request", "id": op_id, "client": client, "value": txn})
        reply = self._run(op_id, txn)
        self.campaign.journal(dict(reply, event="complete", id=op_id))
        return reply

    def _run(self, op_id, txn):
        campaign = self.campaign
        with campaign.lock:
            live, phase = campaign.live, campaign.phase()
            layout = None if live is None else campaign.layouts.get(live[0])
        if live is None:
            return {"type": "fail", "generation": None, "error": {"kind": "no-live-generation"}}
        generation, port = live
        family = family_of(txn[0][1]) if txn and isinstance(txn[0], list) and len(txn[0]) == 3 \
            and type(txn[0][1]) is int else None
        shaped = family in SHAPED
        code = shaped_code(family, txn, layout) if shaped else txn_code(txn, layout, op_id)
        current = (self.shaped_transport, self.shaped_generation) if shaped else (self.transport, self.generation)
        if current[0] is None or current[1] != generation:
            if shaped:
                self.close_shaped()
            else:
                self.close()
            transport = campaign.transport_factory(port)
            try:
                transport.connect("programmer" if shaped else "wizard")
            except Exception as error:
                transport.disconnect()
                # The transaction was never sent.
                return {"type": "fail", "generation": generation,
                        "error": {"kind": "connect", "message": f"{type(error).__name__}: {error}"}}
            if shaped:
                self.shaped_transport, self.shaped_generation = transport, generation
            else:
                self.transport, self.generation = transport, generation
        transport = self.shaped_transport if shaped else self.transport
        campaign.touch(generation, [key for _, key, _ in txn])
        campaign.journal({"event": "send", "id": op_id, "generation": generation, "port": port,
                          "code": code, "phase": phase})
        try:
            result = transport.execute(code)
        except Exception as error:
            raw = transport.last_raw
            if shaped:
                self.close_shaped()
            else:
                self.close()
            return {"type": "info", "generation": generation, "raw": raw,
                    "error": {"kind": "transport", "message": f"{type(error).__name__}: {error}"}}
        completion = complete_shaped(family, txn, result) if shaped else complete_txn(txn, result)
        return dict(completion, generation=generation, raw=transport.last_raw)


class AdapterHandler(socketserver.StreamRequestHandler):
    def handle(self):
        campaign = self.server.campaign
        session = ClientSession(campaign)
        try:
            for line in self.rfile:
                request = json.loads(line)
                if request.get("op") == "txn":
                    reply = session.txn(request.get("id"), request.get("process"), request["value"])
                elif request.get("op") in FAULTS:
                    reply = campaign.fault(request["op"])
                else:
                    reply = {"type": "info", "error": {"kind": "unknown-request", "request": request}}
                reply = dict(reply, id=request.get("id"))
                self.wfile.write((json.dumps(reply) + "\n").encode("utf-8"))
                self.wfile.flush()
        finally:
            session.close()


class AdapterServer(socketserver.ThreadingTCPServer):
    daemon_threads = True

    def __init__(self, campaign):
        super().__init__(("127.0.0.1", 0), AdapterHandler)
        self.campaign = campaign


# ---------------------------------------------------------------------------
# Analysis: reconciliation, generation splitting, Elle EDN, durability,
# scenario obligations and runtime logs

def load_jsonl(path):
    return [json.loads(line) for line in Path(path).read_text(encoding="utf-8").splitlines() if line.strip()]


ADAPTER_SIDE = ("adapter-timeout", "adapter-io")  # Jepsen gave up without the adapter's reply


def reconcile(history, journal):
    """Check Jepsen's client completions against the adapter journal by operation id.

    Returns (history, counts, contradictions). The returned history fills in
    the generation of an operation Jepsen stopped waiting for from the
    adapter's send record (it stays :info), and marks an operation the
    adapter never sent as unsent.
    """
    events, contradictions = {}, []
    for row in journal:
        if row.get("event") in ("request", "send", "complete"):
            slot = events.setdefault(row.get("id"), {})
            if row["event"] in slot:
                contradictions.append(f"journal records {row['event']} twice for operation {row.get('id')}")
            slot[row["event"]] = row
    counts = {"pending": 0, "late": 0, "unsent": 0, "never_reached_adapter": 0}
    invoked, seen, placed = {}, set(), []
    for position, op in enumerate(history):
        process = op.get("process")
        if type(process) is not int:
            placed.append(op)
            continue
        if op["type"] == "invoke":
            invoked[process] = op.get("value")
            placed.append(op)
            continue
        op = dict(op)
        placed.append(op)
        op_id = op.get("op-id")
        where = f"history position {position} (operation {op_id})"
        if op_id is None:
            contradictions.append(f"{where}: client completion has no operation id")
            continue
        if op_id in seen:
            contradictions.append(f"{where}: operation id completed twice")
        seen.add(op_id)
        slot = events.get(op_id, {})
        request, send, done = slot.get("request"), slot.get("send"), slot.get("complete")
        adapter_side = (op.get("error") or {}).get("kind") in ADAPTER_SIDE
        if request is None:
            if adapter_side:
                op["unsent"] = True
                counts["never_reached_adapter"] += 1
            else:
                contradictions.append(f"{where}: Jepsen recorded an adapter reply the adapter never journaled")
            continue
        if request.get("value") != invoked.get(process) or request.get("client") != process:
            contradictions.append(f"{where}: journal request (client {request.get('client')}, "
                                  f"{request.get('value')}) is not Jepsen's invocation "
                                  f"(process {process}, {invoked.get(process)})")
        if send is None:
            if adapter_side:
                op["unsent"] = True
                counts["unsent"] += 1
            elif op["type"] != "fail" or (done or {}).get("type") != "fail":
                contradictions.append(f"{where}: Jepsen recorded {op['type']} for an operation the adapter "
                                      f"never sent (journal completion {(done or {}).get('type')})")
            else:
                counts["unsent"] += 1
            continue
        generation = send.get("generation")
        if op["type"] == "fail":
            contradictions.append(f"{where}: Jepsen recorded fail for an operation sent in generation {generation}")
        elif op.get("generation") is None:
            if not adapter_side:
                contradictions.append(f"{where}: adapter reply without a generation for a sent operation")
            op["generation"] = generation
            counts["late" if done is not None else "pending"] += 1
        elif op["generation"] != generation:
            contradictions.append(f"{where}: Jepsen generation {op['generation']}, adapter sent it in "
                                  f"generation {generation}")
        elif done is None:
            contradictions.append(f"{where}: the adapter's completion is missing from the journal")
        elif (done.get("type"), done.get("generation")) != (op["type"], generation) or \
                (op["type"] == "ok" and done.get("value") != op.get("value")):
            contradictions.append(f"{where}: Jepsen recorded {op['type']} {op.get('value')}, journal recorded "
                                  f"{done.get('type')} {done.get('value')} in generation {done.get('generation')}")
    for op_id, slot in events.items():
        if op_id not in seen and "request" in slot:
            contradictions.append(f"journal operation {op_id} has no Jepsen completion")
    return placed, counts, contradictions


def split_generations(history):
    """Assign each client invocation/completion pair to its adapter generation.

    Returns ({generation: ops in history order}, counts). An operation the
    adapter never sent (a :fail with no generation, or one reconcile marked
    unsent) is excluded. Any other completion without a generation cannot be
    placed in one process lifetime, which would make per-generation checking
    unsound, so it is an error.
    """
    pending, placed = {}, {}
    counts = {"nemesis": 0, "unsent": 0}
    for position, op in enumerate(history):
        process = op["process"]
        if type(process) is not int:
            counts["nemesis"] += 1
            continue
        if op["type"] == "invoke":
            if process in pending:
                raise ValueError(f"process {process} invoked twice at history position {position}")
            pending[process] = position
            continue
        invoked = pending.pop(process, None)
        if invoked is None:
            raise ValueError(f"completion without invocation at history position {position}")
        generation = op.get("generation")
        if generation is None:
            if op["type"] != "fail" and not op.get("unsent"):
                raise ValueError(f"{op['type']} completion at history position {position} has no generation")
            counts["unsent"] += 1
            continue
        placed[invoked] = placed[position] = generation
    if pending:
        raise ValueError(f"invocations without completion at history positions {sorted(pending.values())}")
    buckets = {}
    for position, op in enumerate(history):
        if position in placed:
            buckets.setdefault(placed[position], []).append(op)
    return buckets, counts


def observation_ops(lifecycle, generation):
    """The campaign's final observations of a generation as read-only Elle transactions."""
    ops = []
    for row in lifecycle:
        if row.get("event") == "observation" and row.get("generation") == generation and row.get("ok") \
                and row.get("keys"):
            ops.append({"process": OBSERVER_PROCESS, "type": "invoke", "f": "txn",
                        "value": [["r", key, None] for key in row["keys"]]})
            ops.append({"process": OBSERVER_PROCESS, "type": "ok", "f": "txn",
                        "value": [["r", key, value] for key, value in zip(row["keys"], row["value"])]})
    # The snapshot recovered after this generation's checkpoint-crash is a read
    # of a historical cut of its rows, appended last; the serializable check
    # imposes no real-time order on it (plan section 2 rule 5). Its rows started
    # empty at admission, so no S0 prefix needs stripping.
    for row in lifecycle:
        if row.get("event") == "recovery-observation" and row.get("observed_generation") == generation \
                and row.get("ok") and row.get("epoch") == generation and row.get("keys"):
            ops.append({"process": RECOVERY_PROCESS, "type": "invoke", "f": "txn",
                        "value": [["r", key, None] for key in row["keys"]]})
            ops.append({"process": RECOVERY_PROCESS, "type": "ok", "f": "txn",
                        "value": [["r", key, value] for key, value in zip(row["keys"], row["rows"])]})
    return ops


def edn(value):
    if value is None:
        return "nil"
    if type(value) is int:
        return str(value)
    if isinstance(value, list):
        return "[" + " ".join(edn(item) for item in value) + "]"
    raise ValueError(f"unsupported EDN value {value!r}")


def elle_ops(ops):
    """ops ready for Elle: each invocation completed :ok carries the ok value's
    micro-ops (reads nil), so an append a shaped family reported denied
    appears nowhere; an :ok pair with no effective micro-op is dropped."""
    completions, pending = {}, {}
    for index, op in enumerate(ops):
        if op["type"] == "invoke":
            pending[op["process"]] = index
        elif op["process"] in pending:
            completions[pending.pop(op["process"])] = index
    dropped, ready = set(), []
    for index, op in enumerate(ops):
        if index in dropped:
            continue
        done = completions.get(index)
        if op["type"] == "invoke" and done is not None and ops[done]["type"] == "ok":
            if not ops[done]["value"]:
                dropped.add(done)
                continue
            op = dict(op, value=[[f, key, None if f == "r" else item] for f, key, item in ops[done]["value"]])
        ready.append(op)
    return ready


def elle_anomalies(directory):
    """Anomaly names Elle wrote witnesses for (<directory>/<anomaly>.txt)."""
    directory = Path(directory)
    return sorted(path.stem for path in directory.glob("*.txt")) if directory.is_dir() else []


def elle_history(ops):
    """One generation's ops in the EDN form the Go Elle recorder writes."""
    lines = []
    for index, op in enumerate(ops):
        mops = " ".join(f"[:{f} {edn(key)} {edn(value)}]" for f, key, value in op["value"])
        lines.append(f"{{:index {index} :process {op['process']} :type :{op['type']} :f :txn :value [{mops}]}}")
    return "\n".join(lines) + "\n"


def append_tokens(ops, types):
    """Appended smoke tokens: only smoke rows are mirrored in the durable ledger."""
    return {(key, value) for op in ops if op["type"] in types
            for f, key, value in op["value"] if f == "append" and family_of(key) == "smoke"}


def durability_violations(baseline, recovered, acked, attempted):
    """Compare a recovered ledger with the explicit checkpoint baseline.

    Every baseline list must survive as a prefix, and every token acknowledged
    in the checkpointed generation must survive. Tokens after the prefix are
    allowed only if an :ok or :info append after the checkpoint attempted them.
    """
    problems = []
    for key, prefix in sorted(baseline.items()):
        got = recovered.get(key, [])
        if got[:len(prefix)] != prefix:
            problems.append(f"key {key}: checkpointed prefix {prefix} not preserved; recovered {got}")
    for key, got in sorted(recovered.items()):
        if len(set(got)) != len(got):
            problems.append(f"key {key}: duplicate tokens in recovered {got}")
        prefix = baseline.get(key, [])
        if got[:len(prefix)] == prefix:
            for value in got[len(prefix):]:
                if (key, value) not in attempted:
                    problems.append(f"key {key}: recovered token {value} was never attempted after the checkpoint")
    for key, value in sorted(acked):
        if value not in recovered.get(key, []):
            problems.append(f"key {key}: token {value} acknowledged before the checkpoint is missing")
    return problems


def checkpoint_comparisons(buckets, lifecycle):
    comparisons = []
    for row in lifecycle:
        if row.get("event") != "admit" or row.get("baseline") is None:
            continue
        start, generation = row["baseline_generation"], row["generation"]
        between = [op for g, ops in buckets.items() if start <= g < generation for op in ops]
        acked = append_tokens(buckets.get(start, []), {"ok"})
        attempted = append_tokens(between, {"ok", "info"})
        comparisons.append({"generation": generation, "kind": row["kind"], "baseline_generation": start,
                            "acknowledged_tokens": len(acked), "attempted_tokens": len(attempted),
                            "violations": durability_violations(ledger_from_json(row["baseline"]),
                                                                ledger_from_json(row["recovered"]),
                                                                acked, attempted)})
    return comparisons


# ---------------------------------------------------------------------------
# Snapshot oracle for checkpoint-crash recovery (plan section 2 rules 1-6)

def _pair(outputs):
    """{database, waif_identities} sha256 from published-pair evidence (stable_file_digest rows)."""
    if not outputs or "database" not in outputs:
        return None
    sidecar = outputs.get("waif_identities")
    return {"database": outputs["database"]["sha256"], "waif_identities": sidecar["sha256"] if sidecar else None}


def launch_pair(admit):
    """The pair a generation actually ran from: what recovery selected, else its input."""
    identity = admit.get("server") or {}
    return _pair(identity.get("recovered_pair")) or identity.get("input_pair")


def pending_pairs(artifacts, completed):
    """Every pair the frozen files identify as the pending checkpoint F: the
    journal's next pair, a staged temp pair with its sidecar, a recovery
    directory's next pair, or a published output that is not A.

    While a journal is present the output is a published pair only when it is
    the journal's next pair: between the two renames it holds the next
    database with the previous (or no) sidecar, a mixed pair that is never a
    permitted cut (plan section 2 table)."""
    pairs = []
    journal = artifacts.get("journal") or {}
    if journal.get("next"):
        pairs.append(journal["next"])
    if artifacts.get("temp") and artifacts["temp"].get("waif_identities"):
        pairs.append(artifacts["temp"])
    for members in (artifacts.get("recovery_directories") or {}).values():
        if members.get("next"):
            pairs.append(members["next"])
    output = artifacts.get("output")
    if output and output != completed and (not artifacts.get("journal") or output == journal.get("next")):
        pairs.append(output)
    unique = []
    for pair in pairs:
        if pair not in unique:
            unique.append(pair)
    return unique


def epoch_transactions(journal, generation):
    """Every operation the adapter sent in generation, with journal times.

    The send record is written before sending and the completion after the
    reply, so send_ns is no later than the real send and complete_ns no
    earlier than the real completion. An operation never sent is absent, so
    its tokens count as never attempted.
    """
    requests, sends, completes = {}, {}, {}
    for row in journal:
        table = {"request": requests, "send": sends, "complete": completes}.get(row.get("event"))
        if table is not None:
            table[row.get("id")] = row
    txns = []
    for op_id, send in sends.items():
        if send.get("generation") != generation:
            continue
        value = (requests.get(op_id) or {}).get("value") or []
        done = completes.get(op_id) or {}
        # An ok completion's value is its effective micro-ops: a shaped
        # family's denied append had no effect (complete_shaped).
        effective = done["value"] if done.get("type") == "ok" and "value" in done else value
        txns.append({"id": op_id, "value": value, "send_ns": send["monotonic_ns"],
                     "type": done.get("type"), "complete_ns": done.get("monotonic_ns"),
                     "error": done.get("error") or {}, "result": done.get("value") if done.get("type") == "ok" else None,
                     "writes": [(key, item) for f, key, item in effective or [] if f == "append"]})
    return txns


def _prefix_comparable(left, right):
    shorter = min(len(left), len(right))
    return left[:shorter] == right[:shorter]


def snapshot_common_violations(rows, txns, observations):
    """Checks independent of the selected cut, over the recovered rows R (D = R: rows start empty).

    Rule 4: an atomic transaction's writes are all present or all absent
    (native-error replies are excluded: an unseen exception is not a rollback).
    Rule 5: an included successful transaction's reads are included; every
    successful read and final observation is prefix-comparable with R; R holds
    no repeated or never-sent token.

    Returns [(witness, message)]; witness names the violated rule for
    negative controls (CONTROLS).
    """
    problems = []
    present = {(key, item) for key, items in rows.items() for item in items}
    attempted = {write for txn in txns for write in txn["writes"]}
    for key, items in sorted(rows.items()):
        if len(set(items)) != len(items):
            problems.append(("duplicate-token", f"key {key}: recovered row {items} repeats a token"))
        for item in items:
            if (key, item) not in attempted:
                problems.append(("never-sent-token",
                                 f"key {key}: recovered token {item} was never sent in this generation"))
    for txn in txns:
        if not txn["writes"]:
            continue
        included = [write for write in txn["writes"] if write in present]
        if included and len(included) != len(txn["writes"]) and txn["error"].get("kind") != "native-error":
            problems.append(("partial-transaction", f"operation {txn['id']} ({txn['type']}): recovered {included} "
                                                    f"of its writes {txn['writes']}, not all or none"))
        if txn["type"] == "ok" and len(included) == len(txn["writes"]):
            for f, key, items in txn["result"] or []:
                missing = [item for item in items if (key, item)
                           not in present] if f == "r" else []
                if missing:
                    problems.append(("missing-read-dependency",
                                     f"operation {txn['id']}: its writes were recovered but tokens {missing} it "
                                     f"read on key {key} were not"))
    reads = [(f"operation {txn['id']}", key, items) for txn in txns if txn["type"] == "ok"
             for f, key, items in txn["result"] or [] if f == "r"]
    reads += [(f"final observation {index}", key, items) for index, row in enumerate(observations)
              for key, items in zip(row["keys"], row["value"])]
    for who, key, items in reads:
        if key in rows and not _prefix_comparable(items, rows[key]):
            problems.append(("non-prefix-read",
                             f"{who}: read {items} on key {key} is not prefix-comparable with recovered {rows[key]}"))
    return problems


def snapshot_cut_violations(cut, generation, epoch, start_epoch, rows, txns):
    """Rule 3 for one candidate cut {name, request_ns, ack_ns}. Returns [(witness, message)]."""
    present = {(key, item) for key, items in (rows or {}).items() for item in items}
    if cut["name"] == "unchanged input":
        problems = []
        if epoch != start_epoch:
            problems.append(("epoch-mismatch", f"the input generation {generation} started from had epoch marker "
                                               f"{start_epoch}, recovered marker is {epoch}"))
        if present:
            problems.append(("rows-without-checkpoint", f"no checkpoint was recovered, yet rows hold {sorted(present)}"))
        return problems
    if epoch != generation:
        return [("epoch-mismatch", f"{cut['name']} was captured after generation {generation} was admitted, "
                                   f"but the recovered epoch marker is {epoch}")]
    problems = []
    for txn in txns:
        if txn["type"] == "ok" and txn["writes"] and txn["complete_ns"] is not None \
                and txn["complete_ns"] < cut["request_ns"]:
            missing = [write for write in txn["writes"] if write not in present]
            if missing:
                problems.append(("lost-checkpointed-update",
                                 f"operation {txn['id']} completed before the checkpoint request; "
                                 f"its writes {missing} are missing"))
        if cut.get("ack_ns") is not None and txn["send_ns"] > cut["ack_ns"]:
            extra = [write for write in txn["writes"] if write in present]
            if extra:
                problems.append(("post-acknowledgment-write",
                                 f"operation {txn['id']} was first sent after the checkpoint completed; "
                                 f"its writes {extra} are present"))
    return problems


def held_task_count(lifecycle, generation):
    """Task-lane tasks generation's launch found queued (task-recovery rows)."""
    return sum(len(row.get("present") or []) for row in lifecycle
               if row.get("event") == "task-recovery" and row.get("generation") == generation)


def snapshot_checks(lifecycle, journal):
    """Check each checkpoint-crash recovery. Returns (rows, [(class, message)]).

    Each row's "witnesses" lists the rules its findings violated (see
    snapshot_common_violations and snapshot_cut_violations), which negative
    controls require."""
    findings, results = [], []

    def find(row, kind, witness, message):
        if witness not in row["witnesses"]:
            row["witnesses"].append(witness)
        if kind not in row["classes"]:
            row["classes"].append(kind)
        findings.append((kind, message))

    by_event = {}
    for row in lifecycle:
        by_event.setdefault(row.get("event"), []).append(row)
    admits = {row["generation"]: row for row in by_event.get("admit", [])}
    observed = {row["generation"]: row for row in by_event.get("recovery-observation", [])}
    requests = {row["seq"]: row for row in by_event.get("checkpoint-request", [])}
    stops = {row["seq"]: row for row in by_event.get("checkpoint-stop", [])}
    for crash in by_event.get("checkpoint-crash", []):
        generation, seq = crash["generation"], crash["seq"]
        where = f"checkpoint-crash of generation {generation}"
        admit, observation = admits.get(generation + 1), observed.get(generation + 1)
        stop, request, start = stops.get(seq), requests.get(seq), observed.get(generation)
        row = {"generation": generation, "seq": seq, "stage": (stop or {}).get("artifacts", {}).get("stage"),
               "witnesses": [], "classes": []}
        results.append(row)
        if None in (admit, observation, stop, request, start, admits.get(generation)) or not observation.get("ok"):
            row["skipped"] = "recovery evidence incomplete (see obligations)"
            continue
        recovered = launch_pair(admit) if (admit.get("server") or {}).get("recovered_pair") else None
        if recovered is None:
            find(row, INCOMPLETE, "no-recovered-pair", f"{where}: the recovered generation recorded no recovered pair")
            row["skipped"] = "no recovered pair"
            continue
        start_pair = launch_pair(admits[generation])
        completed = [item for item in by_event.get("checkpoint-complete", [])
                     if item["generation"] == generation and item["seq"] < seq]
        latest = completed[-1] if completed else None
        latest_pair = _pair(latest.get("outputs")) if latest else None
        if latest is not None and latest_pair is None:
            find(row, INCOMPLETE, "no-checkpoint-digest",
                 f"{where}: completed checkpoint {latest['seq']} has no captured pair digest")
        try:
            expected = barn_process.expected_recovery(stop["artifacts"]) or start_pair
        except ValueError as error:
            find(row, RECOVERY, "unreadable-journal", f"{where}: {error}")
            expected = None
        row.update(recovered=recovered, expected=expected)
        if expected is not None and expected != recovered:
            find(row, RECOVERY, "recovery-stage-mismatch", f"{where}: recovery selected {recovered}; frozen stage "
                                                           f"{row['stage']} requires {expected}")
        cuts = [{"name": "unchanged input", "pair": start_pair}]
        if latest_pair is not None:
            cuts.append({"name": f"completed checkpoint {latest['seq']}", "pair": latest_pair,
                         "request_ns": requests[latest["seq"]]["boundary"]["signal_ns"],
                         "ack_ns": latest["observed_ns"]})
        for pair in pending_pairs(stop["artifacts"], latest_pair):
            cuts.append({"name": f"pending checkpoint {seq}", "pair": pair,
                         "request_ns": request["boundary"]["signal_ns"], "ack_ns": None})
        matching = [cut for cut in cuts if cut["pair"] == recovered]
        row["candidate_cuts"] = [cut["name"] for cut in matching]
        if not matching:
            find(row, RECOVERY, "no-permitted-cut", f"{where}: recovered pair {recovered} matches no permitted cut "
                                                    f"({[cut['name'] for cut in cuts]})")
            # The publication contract never accepts a database with WAIF
            # identities other than its own pair's (plan section 2 table).
            mixed = next((cut for cut in cuts if cut["pair"] and cut["pair"]["database"] == recovered["database"]
                          and cut["pair"]["waif_identities"] != recovered["waif_identities"]), None)
            if mixed is not None:
                find(row, CORRECTNESS, "mixed-pair",
                     f"{where}: mixed pair accepted: the recovered database is {mixed['name']}'s, but its WAIF "
                     f"identities are {recovered['waif_identities']}, not {mixed['pair']['waif_identities']}")
            continue
        # Task-lane plants are held behind their driver (they cannot change the
        # rows before release) and task_checks judges them; count only the rest.
        recovered_tasks = observation["tasks"] - held_task_count(lifecycle, generation + 1)
        launch_tasks = start["tasks"] - held_task_count(lifecycle, generation)
        if recovered_tasks != launch_tasks:
            find(row, INCOMPLETE, "restored-task", f"{where}: {recovered_tasks} queued tasks recovered outside the "
                                                   f"task lane, {launch_tasks} at the generation's own launch; a "
                                                   f"restored task could have changed the observed rows")
        txns = epoch_transactions(journal, generation)
        rows = None if observation.get("rows") is None else dict(zip(observation["keys"], observation["rows"]))
        if rows is not None:
            finals = [item for item in by_event.get("observation", [])
                      if item.get("generation") == generation and item.get("ok") and item.get("keys")]
            for witness, problem in snapshot_common_violations(rows, txns, finals):
                find(row, CORRECTNESS, witness, f"{where}: {problem}")
        per_cut = {cut["name"]: snapshot_cut_violations(cut, generation, observation["epoch"], start["epoch"],
                                                        rows, txns) for cut in matching}
        row["cut_violations"] = {name: [message for _, message in problems] for name, problems in per_cut.items()}
        if all(per_cut.values()):
            for name, problems in per_cut.items():
                for witness, problem in problems:
                    find(row, CORRECTNESS, witness, f"{where}: no permitted cut fits; as {name}: {problem}")
        else:
            row["selected_cut"] = next(name for name, problems in per_cut.items() if not problems)
    return results, findings


def _table_cut(name, recovery):
    """Whether a selected cut name is the plan table's cut kind for a stage step."""
    return {"unchanged input": name == "unchanged input",
            "completed": name.startswith("completed checkpoint"),
            "pending": name.startswith("pending checkpoint")}[recovery]


def stage_coverage(lifecycle, selected, recoveries):
    """Per selected STAGE_SCHEDULE step: what the run actually reached, stopped,
    killed and recovered, and with what verdict. Returns (rows, [(class, message)]).

    The reached point is the process's own boundary record, never the request.
    A step without a record, a stop, a kill and a recovery is incomplete; a
    record from another process or point, or frozen files of another stage,
    is invalid evidence; recovery that is not the plan table's cut, or a double
    crash that does not converge, is a recovery failure. recoveries are
    snapshot_checks rows, whose findings the caller reports separately.
    """
    findings, rows = [], []
    by_seq = {row.get("seq"): row for row in recoveries}
    stops = {row.get("stage"): row for row in lifecycle if row.get("event") == "checkpoint-stop" and row.get("stage")}
    unreached = {row.get("stage"): row for row in lifecycle if row.get("event") == "stage-unreached"}
    crashes = {row.get("seq"): row for row in lifecycle if row.get("event") == "checkpoint-crash"}
    second = {row.get("seq"): row for row in lifecycle if row.get("event") == "recovery-stop"}
    for step in STAGE_SCHEDULE:
        spec = STAGE_SCHEDULE[step]
        requested = f"{spec['point']}:{spec['occurrence']}"
        row = {"stage": step, "requested": requested, "plan_cut": spec["cut"]}
        rows.append(row)
        if step not in selected:
            row["verdict"] = "not-selected"
            continue
        problems = []

        def problem(kind, message):
            problems.append(kind)
            findings.append((kind, f"stage {step}: {message}"))

        stop = stops.get(step)
        if stop is None:
            reason = (unreached.get(step) or {}).get("error", "no checkpoint-stop was recorded")
            problem(INCOMPLETE, f"{requested} was not reached: {reason}")
            row["verdict"] = INCOMPLETE
            continue
        record = stop.get("record") or {}
        seq = stop.get("seq")
        reached = f"{record.get('point')}:{record.get('occurrence')}"
        crash = crashes.get(seq) or {}
        recovery = by_seq.get(seq) or {}
        row.update(reached=reached, pid=record.get("pid"), owned_pid=stop.get("owned_pid"),
                   stopped_state=stop.get("observed_state"), frozen_stage=(stop.get("artifacts") or {}).get("stage"),
                   expected_frozen_stage=spec["frozen"], killed_exit=(crash.get("result") or {}).get("exit_code"),
                   restart=(crash.get("restart") or {}).get("kind"), recovered=recovery.get("recovered"),
                   candidate_cuts=recovery.get("candidate_cuts"), selected_cut=recovery.get("selected_cut"),
                   witnesses=recovery.get("witnesses", []))
        if reached != requested or record.get("pid") != stop.get("owned_pid"):
            problem(INVALID, f"boundary record {record} is not {requested} from owned PID {stop.get('owned_pid')}")
        if stop.get("observed_state") != "T":
            problem(INCOMPLETE, f"owned PID was not observed stopped at the record (state {stop.get('observed_state')!r})")
        if row["frozen_stage"] != spec["frozen"]:
            problem(INVALID, f"stopped at {reached} but the frozen files are stage {row['frozen_stage']!r}, "
                             f"not {spec['frozen']!r}")
        if row["killed_exit"] != -9:
            problem(INCOMPLETE, f"SIGKILL was not observed (exit code {row['killed_exit']})")
        if "recovery" in spec:
            again = second.get(seq)
            if again is None:
                problem(INCOMPLETE, f"no second stop during recovery ({spec['recovery']}) was recorded")
            else:
                second_record = again.get("record") or {}
                row.update(second_reached=f"{second_record.get('point')}:{second_record.get('occurrence')}",
                           second_stopped_state=again.get("observed_state"),
                           second_killed_exit=(again.get("result") or {}).get("exit_code"),
                           second_frozen_stage=(again.get("artifacts") or {}).get("stage"))
                if row["second_reached"] != spec["recovery"] or second_record.get("pid") != again.get("owned_pid"):
                    problem(INVALID, f"recovery boundary record {second_record} is not {spec['recovery']} from "
                                     f"owned PID {again.get('owned_pid')}")
                if again.get("observed_state") != "T" or row["second_killed_exit"] != -9:
                    problem(INCOMPLETE, f"second crash not observed: state {again.get('observed_state')!r}, "
                                        f"exit {row['second_killed_exit']}")
                if not again.get("forensic"):
                    problem(INCOMPLETE, "the second crash's artifact set was not preserved")
                try:
                    first = barn_process.expected_recovery(stop.get("artifacts") or {})
                    then = barn_process.expected_recovery(again.get("artifacts") or {})
                except (ValueError, KeyError, TypeError) as error:
                    problem(RECOVERY, f"double crash artifacts are unreadable: {error}")
                else:
                    row.update(first_expected=first, second_expected=then)
                    if first != then or (recovery.get("recovered") is not None and recovery["recovered"] != first):
                        problem(RECOVERY, f"repeated recovery does not converge: first crash requires {first}, "
                                          f"second crash requires {then}, recovered {recovery.get('recovered')}")
        if not recovery or recovery.get("skipped") or recovery.get("recovered") is None:
            problem(INCOMPLETE, f"no recovery was checked ({recovery.get('skipped', 'no recovery row')})")
        elif recovery.get("candidate_cuts") and not any(_table_cut(name, spec["cut"])
                                                        for name in recovery["candidate_cuts"]):
            problem(RECOVERY, f"recovered pair {recovery['recovered']} is {recovery['candidate_cuts']}, not the "
                              f"plan table's {spec['cut']} cut")
        failed = [kind for kind in PRECEDENCE if kind in problems or kind in recovery.get("classes", [])]
        row["verdict"] = failed[0] if failed else "healthy"
    return rows, findings


# ---------------------------------------------------------------------------
# Task lifecycle checker (plan section 4). Separate from Elle and the append
# oracle: a fork/suspend/release sequence is never one Elle transaction.

def expected_task_record(task, programmer):
    """The record a released task of kind must write once, from its token."""
    token, kind = task["token"], task["kind"]
    wake = [token, "wake"]
    return {"fork": ["fork", [token, "fork-local"], programmer, task["tid"]],
            "suspend": ["suspend", [token, "outer"], task["tid"], [wake, [token, "inner"], programmer]],
            "values": ["values", wake, 1, 12, 1, 1, 1, token + 1, 13, 1, 1, token + 2, programmer],
            "edit": ["edit", "old", wake],
            "invalid": ["invalid", wake],
            "recycle": ["recycle", 0, 1, 1, wake],
            "effect": ["effect", wake]}.get(kind)


def record_witness(kind, record, expected):
    """The witness naming how record departs from expected (None when equal)."""
    if record == expected:
        return None
    if not isinstance(record, list) or len(record) != len(expected) or record[0] != expected[0]:
        return "wrong-task-record"
    if kind == "edit" and record[1] != "old":
        return "wrong-program-after-edit"
    if kind == "recycle" and record[1] != 0:
        return "recycled-reference-valid"
    wake_at = {"suspend": None, "values": 1, "edit": 2, "recycle": 4, "effect": 1, "invalid": 1}.get(kind)
    if kind == "suspend":
        inner = record[3] if isinstance(record[3], list) and len(record[3]) == 3 else [None, None, None]
        if inner[0] != expected[3][0]:
            return "wrong-wake-value"
        if inner[2] != expected[3][2]:
            return "wrong-task-perms"
        return "wrong-locals"
    if wake_at is not None and record[wake_at] != expected[wake_at]:
        return "wrong-wake-value"
    if kind in ("fork", "values") and record[-1 if kind == "values" else 2] != expected[-1 if kind == "values" else 2]:
        return "wrong-task-perms"
    if kind == "values":
        return "wrong-alias"
    if kind == "fork":
        return "wrong-locals" if record[1] != expected[1] else "wrong-task-id"
    return "wrong-task-record"


def _selected_cut_names(recoveries, generation):
    row = next((item for item in recoveries or [] if item.get("generation") == generation), None)
    return None if row is None else row.get("candidate_cuts")


def task_checks(lifecycle, receipts, recoveries, live=False):
    """Check every task-lane plant. Returns (results, [(class, witness, message)]).

    Per launch (task-recovery): a plant is held (every task queued, no result),
    completed (no task queued, results present) or absent (scenario object not
    valid); anything else is a finding. The generation launched from a plant's
    own graceful checkpoint must find it held; the recovery after a live plant's
    checkpoint-crash must find it held when the selected cut is A or F and absent
    when only the unchanged input fits. Per release: each task's record exactly
    once with its expected content, cancel never, and every task out of the queue.
    """
    findings, rows = [], []

    def find(kind, witness, message, row=None):
        findings.append((kind, witness, message))
        if row is not None and witness not in row["witnesses"]:
            row["witnesses"].append(witness)

    plants = {row["seq"]: row for row in lifecycle if row.get("event") == "task-plant"}
    admits = {row["generation"]: row for row in lifecycle if row.get("event") == "admit"}
    releases = {}
    for row in lifecycle:
        if row.get("event") == "task-release":
            releases.setdefault(row["plant"], []).append(row)
    effect_lines = {}
    for receipt in receipts:
        line = receipt.get("line", "")
        if line.startswith(TASK_EFFECT_PREFIX + " "):
            key = (receipt.get("generation"), line[len(TASK_EFFECT_PREFIX) + 1:].strip())
            effect_lines[key] = effect_lines.get(key, 0) + 1
    for seq, plant in sorted(plants.items()):
        programmer = plant["setup"]["programmer"]
        summary = {"plant": seq, "point": plant["point"], "generation": plant["generation"], "launches": [],
                   "releases": [], "witnesses": []}
        rows.append(summary)
        invalid = {row["tid"] for row in lifecycle if row.get("event") == "task-invalidate" and row["plant"] == seq}
        all_tids = [task["tid"] for task in plant["tasks"]]
        if len(set(all_tids)) != len(all_tids):
            find(INVALID, "task-plant", f"plant {seq}: task ids {all_tids} repeat", summary)
        tids = [tid for tid in all_tids if tid not in invalid]
        for recovery in (row for row in lifecycle if row.get("event") == "task-recovery" and row["plant"] == seq):
            generation, state = recovery["generation"], recovery.get("state") or {}
            where = f"plant {seq} at the launch of generation {generation}"
            launch = {"generation": generation, "status": "unreadable"}
            summary["launches"].append(launch)
            if not recovery.get("ok"):
                find(INCOMPLETE, "task-unreadable", f"{where}: held state unreadable: "
                                                    f"{recovery.get('failed_attempts')}", summary)
                continue
            ids = state.get("ids") or []
            if invalid & set(ids):
                find(CORRECTNESS, "invalid-task-policy", f"{where}: the deliberately invalidated task(s) "
                                                         f"{sorted(invalid & set(ids))} were restored", summary)
            if len(set(ids)) != len(ids):
                find(CORRECTNESS, "duplicated-continuation", f"{where}: queued_tasks repeats a task id: {ids}",
                     summary)
            present = [tid for tid in tids if tid in ids]
            results = state.get("results")
            if results is None:
                status = "absent"
                if present:
                    find(CORRECTNESS, "lost-task", f"{where}: tasks {present} are queued but their scenario "
                                                   f"object is not valid", summary)
            elif not results and len(present) == len(tids):
                status = "held"
            elif results and not present:
                status = "completed"
            elif not results:
                status = "partial"
                find(CORRECTNESS, "lost-task", f"{where}: held tasks {sorted(set(tids) - set(present))} of "
                                               f"{tids} are missing from queued_tasks", summary)
            else:
                status = "partial"
                earlier = sorted(release["generation"] for release in releases.get(seq, [])
                                 if release["generation"] < generation and not release.get("complete"))
                if earlier:
                    # A checkpoint taken after an incomplete release captured
                    # its leftovers; the lost tasks were reported at that release.
                    find(CORRECTNESS, "lost-task", f"{where}: tasks {present} are still queued with results "
                                                   f"{results} left by the incomplete release in generation "
                                                   f"{earlier[-1]}", summary)
                else:
                    find(CORRECTNESS, "ran-before-release", f"{where}: results {results} were written before the "
                                                            f"driver released tasks {present}", summary)
            launch["status"] = status
            admit = admits.get(generation) or {}
            restart = admit.get("restart") or {}
            expected = None
            if plant["point"] == "graceful-checkpoint" and admit.get("kind") == "checkpoint" \
                    and admit.get("baseline_generation") == plant["generation"]:
                expected, why = "held", "it was planted before this verified checkpoint"
            elif plant["point"] == "live-checkpoint" and restart.get("generation") == plant["generation"]:
                cuts = _selected_cut_names(recoveries, plant["generation"])
                if cuts:
                    if all(name == "unchanged input" for name in cuts):
                        expected, why = "absent", "only the unchanged input (before the plant) fits the recovery"
                    elif "unchanged input" not in cuts:
                        expected, why = "held", f"the recovery selected {cuts}, captured after the plant"
            if expected == "held" and status != "held" and status != "partial":
                find(CORRECTNESS, "lost-task", f"{where}: {status}, but {why}", summary)
            elif expected == "absent" and status != "absent":
                find(CORRECTNESS, "unexpected-task", f"{where}: {status}, but {why}", summary)
        for release in releases.get(seq, []):
            generation, state = release["generation"], release.get("state") or {}
            where = f"plant {seq} released in generation {generation}"
            results = state.get("results") or {}
            queued = [tid for tid in tids if tid in (state.get("ids") or [])]
            summary["releases"].append({"generation": generation, "complete": release.get("complete")})
            if queued:
                find(CORRECTNESS, "lost-task", f"{where}: tasks {queued} still queued "
                                               f"{release.get('deadline_seconds')}s after release", summary)
            for task in plant["tasks"]:
                token, kind = task["token"], task["kind"]
                records = results.get(token) or results.get(str(token)) or []
                if task["tid"] in invalid:
                    if records:
                        find(CORRECTNESS, "invalid-task-policy", f"{where}: the invalidated task {task['tid']} "
                                                                 f"ran and recorded {records}", summary)
                    continue
                if kind == "cancel":
                    if records:
                        find(CORRECTNESS, "killed-task-ran", f"{where}: killed task {task['tid']} recorded "
                                                             f"{records}", summary)
                    continue
                if not records:
                    find(CORRECTNESS, "lost-task", f"{where}: {kind} task {task['tid']} (token {token}) never "
                                                   f"recorded a result", summary)
                    continue
                if len(records) > 1:
                    find(CORRECTNESS, "duplicated-continuation", f"{where}: {kind} task {task['tid']} recorded "
                                                                 f"{len(records)} times: {records}", summary)
                wanted = expected_task_record(task, programmer)
                witness = record_witness(kind, records[0], wanted)
                if witness:
                    find(CORRECTNESS, witness, f"{where}: {kind} task {task['tid']} recorded {records[0]}, "
                                               f"expected {wanted}", summary)
                if kind == "effect":
                    count = effect_lines.get((generation, str(token)), 0)
                    if count != 1:
                        find(CORRECTNESS, "duplicated-effect" if count > 1 else "lost-effect",
                             f"{where}: the receiver got the effect task's token {token} {count} times", summary)
        released_in = {release["generation"] for release in releases.get(seq, [])}
        for task in plant["tasks"]:
            if task["kind"] != "effect":
                continue
            for (generation, token), count in effect_lines.items():
                if token == str(task["token"]) and generation not in released_in:
                    find(CORRECTNESS, "effect-before-release",
                         f"plant {seq}: the receiver of generation {generation} got effect token {token} "
                         f"{count} times, with no release of the plant in that generation", summary)
    if plants:
        if not any(release.get("complete") for items in releases.values() for release in items):
            findings.append((INCOMPLETE, "task-coverage", "task lane: no plant was released to completion"))
        graceful = [seq for seq, plant in plants.items() if plant["point"] == "graceful-checkpoint"]
        if not any(release.get("complete") for seq in graceful for release in releases.get(seq, [])):
            findings.append((INCOMPLETE, "task-coverage", "task lane: no graceful-checkpoint plant was released "
                                                          "after its restart"))
        if live:
            crashed = [row for row in rows if row["point"] == "live-checkpoint" and any(
                launch["status"] == "held" and admits.get(launch["generation"], {}).get("kind") == "checkpoint-crash"
                for launch in row["launches"])]
            if not crashed:
                findings.append((INCOMPLETE, "task-coverage", "task lane: no live plant was recovered held through "
                                                              "a checkpoint-crash"))
    return rows, findings


# ---------------------------------------------------------------------------
# Effect checker (plan section 4): live-process retries must not duplicate an
# externally visible effect; the receiver's lines are the delivery evidence.

def effect_checks(lifecycle, journal, receipts):
    """Returns (results, [(class, witness, message)])."""
    findings = []
    requests, sends, completes = {}, {}, {}
    for row in journal:
        table = {"request": requests, "send": sends, "complete": completes}.get(row.get("event"))
        if table is not None:
            table[row.get("id")] = row
    sent = {}
    for op_id, send in sends.items():
        value = (requests.get(op_id) or {}).get("value") or []
        if value and all(isinstance(mop, list) and len(mop) == 3 and type(mop[1]) is int
                         and family_of(mop[1]) == "effects" for mop in value):
            done = completes.get(op_id) or {}
            sent[(send.get("generation"), op_id)] = {"type": done.get("type"), "complete_ns": done.get("monotonic_ns"),
                                                     "boundary": boundary_op(op_id)}
    counts = {}
    for receipt in receipts:
        parts = receipt.get("line", "").split()
        if len(parts) == 3 and parts[0] == EFFECT_PREFIX and parts[1].lstrip("-").isdigit() \
                and parts[2] in ("pre", "post"):
            key = (receipt.get("generation"), int(parts[1]), parts[2])
            counts[key] = counts.get(key, 0) + 1
    for (generation, op_id, part), count in sorted(counts.items()):
        if (generation, op_id) not in sent:
            findings.append((CORRECTNESS, "invented-effect", f"generation {generation}: the receiver got "
                                                             f"{op_id} {part}, which was not sent in that generation"))
        elif count > 1:
            findings.append((CORRECTNESS, "duplicated-live-retry-effect",
                             f"generation {generation}: operation {op_id}'s {part} notification was received "
                             f"{count} times"))
    drains = {}
    for row in lifecycle:
        if row.get("event") == "effect-drain" and row.get("ok"):
            drains[row["generation"]] = row
            options = {"", ".flush"} | {f"{EFFECT_PREFIX}-{op}" for (g, op), item in sent.items()
                                        if g == row["generation"] and item["boundary"]}
            if row.get("option") not in options:
                findings.append((CORRECTNESS, "invented-effect", f"generation {row['generation']}: drained "
                                                                 f"flush-command {row.get('option')!r} was set by no "
                                                                 f"boundary operation of that generation"))
    for (generation, op_id), item in sorted(sent.items()):
        pre, post = counts.get((generation, op_id, "pre"), 0), counts.get((generation, op_id, "post"), 0)
        if post and not pre:
            findings.append((CORRECTNESS, "lost-effect", f"generation {generation}: operation {op_id}'s post "
                                                         f"notification arrived without its pre notification"))
        drain = drains.get(generation)
        if item["type"] == "ok" and drain is not None and item["complete_ns"] is not None \
                and item["complete_ns"] < drain["sent_ns"] and not (pre and post):
            findings.append((CORRECTNESS, "lost-effect", f"generation {generation}: operation {op_id} completed "
                                                         f"ok before the drain, but its notifications arrived "
                                                         f"pre {pre}, post {post} times"))
    ok = [key for key, item in sent.items() if item["type"] == "ok"]
    boundary = [key for key in ok if sent[key]["boundary"]]
    retries = [row.get("commit_retries") for row in drains.values() if row.get("commit_retries") is not None]
    results = {"sent": len(sent), "ok": len(ok), "ok_boundary": len(boundary), "drains": len(drains),
               "max_commit_retries": max(retries) if retries else None,
               "witnesses": sorted({witness for _, witness, _ in findings})}
    if not ok:
        findings.append((INCOMPLETE, "effect-coverage", "effects: no successful effects transaction"))
    if not boundary:
        findings.append((INCOMPLETE, "effect-coverage", "effects: no successful transaction crossed the "
                                                        "set_connection_option boundary"))
    if not drains:
        findings.append((INCOMPLETE, "effect-coverage", "effects: no receiver drain succeeded"))
    if not retries or max(retries) <= 0:
        findings.append((INCOMPLETE, "effect-coverage", f"effects: validation conflicts were not shown to be "
                                                        f"forced (barn.commit_retries {results['max_commit_retries']})"))
    return results, findings


def shaped_transactions(journal, family):
    """Every ok shaped transaction of family: id, generation, request value,
    effective value, denied micro-ops, shape and observations."""
    requests, sends = {}, {}
    txns = []
    for row in journal:
        event = row.get("event")
        if event == "request":
            requests[row.get("id")] = row
        elif event == "send":
            sends[row.get("id")] = row
        elif event == "complete" and row.get("type") == "ok" and "shape" in row:
            request = requests.get(row.get("id")) or {}
            value = request.get("value") or []
            if not value or family_of(value[0][1]) != family:
                continue
            txns.append({"id": row.get("id"), "generation": (sends.get(row.get("id")) or {}).get("generation"),
                         "request": value, "value": row.get("value") or [], "denied": row.get("denied") or [],
                         "shape": row["shape"], "observations": row.get("observations") or []})
    return txns


def _reads(txn):
    """{role: list} for the effective reads and {role: literal} for the denied ones."""
    base = FAMILIES[family_of(txn["request"][0][1])]["base"]
    read = {key - base: items for f, key, items in txn["value"] if f == "r"}
    denied = {key - base: literal for f, key, literal in txn["denied"]}
    return read, denied


def _append_value(txn):
    return next(value for f, _, value in txn["request"] if f == "append")


def replay_permissions(markers):
    """PO.x's owner role and flags after the P-machine markers in order (initial: OWNER, "rw")."""
    owner, flags = "owner", "rw"
    for v in markers:
        if v % 5 == 4:
            owner = "writer" if owner == "owner" else "owner"
        elif owner == "owner":
            flags = P_FLAGS[v % 5]
    return owner, flags


def replay_definitions(markers):
    """F-machine state after the markers: y defined on FA, FA.y's value, FC.z's
    local value (None while clear) and FC's parent ("fa"/"fb")."""
    state = {"y": True, "y_value": [0], "z_local": None, "parent": "fa"}
    for v in markers:
        kind = v % 3
        if kind == 0:
            if state["y"]:
                state["y"] = False
            else:
                state.update(y=True, y_value=[v])
        elif kind == 1:
            state["z_local"] = v if state["z_local"] is None else None
        else:
            state.update(parent="fb" if state["parent"] == "fa" else "fa", z_local=None)
    return state


def authority_expectations(txn, objects):
    """[(witness, what, expected, observed)] for one ok P or F transaction,
    from the markers it read in the same task. V-machine transactions are Elle's."""
    shape, observations = txn["shape"], txn["observations"]
    read, denied = _reads(txn)
    roles = AUTHORITY_ROLES
    checks = []
    if shape.startswith("p-"):
        markers = read.get(roles["mp"], [])
        owner, flags = replay_permissions(markers)
        if shape == "p-mutate":
            v = _append_value(txn)
            if v % 5 == 4:
                new = objects["writer"] if owner == "owner" else objects["owner"]
                checks.append(("mutation-outcome", "chown result", [new], observations))
            else:
                checks.append(("mutation-outcome", f"set_property_info flags {P_FLAGS[v % 5]!r}",
                               ["ok" if owner == "owner" else "E_PERM"], observations))
            return checks
        readable = "r" in flags or (shape == "p-write" and owner == "writer")
        got_read = "list" if roles["d"] in read else denied.get(roles["d"])
        checks.append(("stale-permission" if (got_read == "list") != readable else "error-precedence",
                       f"read of PO.x (owner {owner}, flags {flags!r})", "list" if readable else "E_PERM",
                       got_read))
        if shape == "p-read":
            checks.append(("error-precedence", "READER chown of PO.x", ["E_PERM"], observations))
        else:
            writable = readable and ("w" in flags or owner == "writer")
            got_write = "written" if any(f == "append" for f, _, _ in txn["value"]) else \
                next((literal for f, _, literal in txn["denied"] if f == "append"), None)
            checks.append(("stale-permission" if (got_write == "written") != writable else "error-precedence",
                           f"append to PO.x (owner {owner}, flags {flags!r})", "written" if writable else "E_PERM",
                           got_write))
        return checks
    if shape.startswith("f-"):
        state = replay_definitions(read.get(roles["mf"], []))
        if shape == "f-mutate":
            v = _append_value(txn)
            kind = v % 3
            # The state before this mutation decides its action.
            expected = ["deleted" if state["y"] else "added"] if kind == 0 else \
                ["cleared" if state["z_local"] is not None else "set"] if kind == 1 else \
                [objects["fb"] if state["parent"] == "fa" else objects["fa"]]
            checks.append(("mutation-outcome", f"definition mutation kind {kind}", expected, observations))
            return checks
        if not (isinstance(observations, list) and len(observations) == 7):
            return [("mutation-outcome", "f-observe observations", "seven values", observations)]
        has_y = state["parent"] == "fa" and state["y"]
        z_value = [state["z_local"]] if state["z_local"] is not None else ([-1] if state["parent"] == "fa" else [-2])
        expected = [("stale-definition", '"y" in properties(FA)', 1 if state["y"] else 0),
                    ("stale-definition", "FC.y", state["y_value"] if has_y else "E_PROPNF"),
                    ("stale-inheritance", "FC.z", z_value),
                    ("stale-inheritance", 'is_clear_property(FC, "z")', 1 if state["z_local"] is None else 0),
                    ("stale-inheritance", "parent(FC)", objects[state["parent"]]),
                    ("error-precedence", 'READER delete_property(FA, "y")', "E_PERM"),
                    ("error-precedence", 'READER clear_property(FC, "y")', "E_PERM" if has_y else "E_PROPNF")]
        return [(witness, what, want, got) for (witness, what, want), got in zip(expected, observations)]
    return checks


def authority_coverage(txn):
    """Coverage labels for one ok authority transaction (from what it observed)."""
    shape, observations = txn["shape"], txn["observations"]
    read, denied = _reads(txn)
    labels = [shape]
    if shape == "p-mutate":
        v = _append_value(txn)
        labels.append("p-chown" if v % 5 == 4 else f"p-flags:{observations[0] if observations else None}")
    elif shape in ("p-read", "p-write"):
        labels.append(f"{shape}:read-" + ("permitted" if AUTHORITY_ROLES["d"] in read else
                                          str(denied.get(AUTHORITY_ROLES["d"]))))
        if shape == "p-write":
            labels.append("p-write:" + ("written" if any(f == "append" for f, _, _ in txn["value"]) else "denied"))
    elif shape == "f-mutate" and observations:
        kind = _append_value(txn) % 3
        labels.append(f"f-mutate:{kind}:" + (str(observations[0]) if kind < 2 else "reparented"))
    elif shape == "f-observe" and len(observations) == 7:
        labels.append("f-observe:FC.y-" + ("present" if isinstance(observations[1], list) else str(observations[1])))
        labels.append(f"f-observe:clear-{observations[6]}")
    elif shape == "v-mutate" and observations:
        labels.append(f"v-mutate:{observations[0]}")
    return labels


AUTHORITY_COVERAGE = ("p-mutate", "p-read", "p-write", "f-mutate", "f-observe", "v-mutate", "v-observe", "v-read",
                      "p-chown", "p-flags:ok", "p-flags:E_PERM", "p-read:read-permitted", "p-read:read-E_PERM",
                      "p-write:written", "p-write:denied", "f-mutate:0:added", "f-mutate:0:deleted",
                      "f-mutate:1:set", "f-mutate:1:cleared", "f-mutate:2:reparented", "f-observe:FC.y-present",
                      "f-observe:FC.y-E_PROPNF", "f-observe:clear-E_PERM", "f-observe:clear-E_PROPNF",
                      "v-mutate:0", "v-mutate:1", "v-mutate:2")


def topology_violations(txn, objects):
    """[(witness, message)] for one ok c-observe snapshot (see the module note
    in agent-notes/jepsen-authority-topology.md for the invariants)."""
    read, _ = _reads(txn)
    roles = TOPOLOGY_ROLES
    created, recycled = read.get(roles["created"], []), read.get(roles["recycled"], [])
    observations = txn["observations"]
    if not (isinstance(observations, list) and len(observations) == 3):
        return [("token-not-conserved", f"malformed observation {observations!r}")]
    rooms, kids, graves = observations
    problems = []
    by_v = {}
    for grave in graves:
        v, token, obj, back = grave
        by_v[v] = (token, obj)
        if back:
            problems.append(("invalid-reference", f"recycled object #{obj} (token {token}) is valid with its token"))
    if sorted(by_v) != sorted(recycled):
        problems.append(("token-not-conserved", f"graves {sorted(by_v)} do not match the RECYCLED ledger {recycled}"))
    gone = [token for v, (token, _) in sorted(by_v.items()) if token]
    if len(set(gone)) != len(gone):
        problems.append(("duplicate-recycle", f"a token was recycled twice: {gone}"))
    unknown = [token for token in gone if token not in created]
    if unknown:
        problems.append(("token-not-conserved", f"recycled tokens {unknown} were never created"))
    live = sorted(set(created) - set(gone))
    room_ids = [objects["t3"], objects["t4"], objects["t5"]]
    parent_ids = {objects["tq3"]: 3, objects["tq4"]: 4}
    seen = []
    for room, items in zip(room_ids, rooms):
        for item in items:
            if not item[1]:
                problems.append(("invalid-reference", f"room #{room} holds invalid object #{item[0]}"))
                continue
            obj, _, token, location, parent, kind = item
            seen.append(token)
            if location != room:
                problems.append(("reciprocal-topology", f"#{obj} (token {token}) is in room #{room}'s contents but "
                                                        f"its location is #{location}"))
            if parent not in parent_ids:
                problems.append(("reciprocal-topology", f"#{obj} (token {token}) has parent #{parent}"))
            elif kind != parent_ids[parent]:
                problems.append(("stale-dispatch", f"#{obj} (token {token}) with parent #{parent} dispatched "
                                                   f"kind() to {kind}"))
    if sorted(seen) != live:
        problems.append(("token-not-conserved", f"rooms hold tokens {sorted(seen)}; live tokens are {live}"))
    children = []
    for parent, items in zip(parent_ids, kids):
        for item in items:
            if not item[1]:
                problems.append(("invalid-reference", f"#{parent}'s children hold invalid object #{item[0]}"))
                continue
            obj, _, token, actual = item
            children.append(token)
            if actual != parent:
                problems.append(("reciprocal-topology", f"#{obj} (token {token}) is a child of #{parent} but its "
                                                        f"parent is #{actual}"))
    if sorted(children) != live:
        problems.append(("token-not-conserved", f"children hold tokens {sorted(children)}; live tokens are {live}"))
    return problems


def topology_coverage(txn):
    shape, observations = txn["shape"], txn["observations"]
    labels = [shape.rsplit("-", 1)[0] if shape[-1].isdigit() else shape]
    if shape in ("c-move", "c-reparent") and observations and observations[0]:
        labels.append(f"{shape}:applied")
    elif shape == "c-recycle" and observations and observations[0] and observations[0][0]:
        labels.append("c-recycle:applied")
    return labels


TOPOLOGY_COVERAGE = ("t-create", "t-cross", "t-read", "c-create", "c-move", "c-reparent", "c-recycle", "c-observe",
                     "c-move:applied", "c-reparent:applied", "c-recycle:applied")


def shaped_checks(lifecycle, journal, families):
    """The authority and topology invariants and coverage. Returns (results,
    [(class, witness, message)]). Elle checks the rows separately."""
    layouts = {row["generation"]: row.get("rows") or {} for row in lifecycle if row.get("event") == "admit"}
    findings, results = [], {}
    for family in families:
        counts, checked, found = {}, 0, []
        txns = shaped_transactions(journal, family)
        for txn in txns:
            objects = layouts.get(txn["generation"], {}).get(family)
            if objects is None:
                found.append((INVALID, f"{family}-layout", f"operation {txn['id']}: generation "
                                                           f"{txn['generation']} recorded no {family} objects"))
                continue
            where = f"generation {txn['generation']} operation {txn['id']} ({txn['shape']})"
            if family == "authority":
                labels = authority_coverage(txn)
                for witness, what, expected, observed in authority_expectations(txn, objects):
                    if expected != observed:
                        found.append((CORRECTNESS, witness, f"{where}: {what}: expected {expected!r}, "
                                                            f"observed {observed!r}"))
            else:
                labels = topology_coverage(txn)
                if txn["shape"] == "c-observe":
                    for witness, problem in topology_violations(txn, objects):
                        found.append((CORRECTNESS, witness, f"{where}: {problem}"))
            checked += 1
            for label in labels:
                counts[label] = counts.get(label, 0) + 1
        required = AUTHORITY_COVERAGE if family == "authority" else TOPOLOGY_COVERAGE
        missing = [label for label in required if not counts.get(label)]
        for label in missing:
            found.append((INCOMPLETE, f"{family}-coverage", f"{family}: no successful transaction showed {label}"))
        results[family] = {"checked": checked, "coverage": dict(sorted(counts.items())), "missing": missing,
                           "denied": sum(len(txn["denied"]) for txn in txns),
                           "witnesses": sorted({witness for kind, witness, _ in found if kind == CORRECTNESS})}
        findings += found
    return results, findings


def outcome_counts(history):
    counts = {}
    for op in history:
        if type(op["process"]) is int and op["type"] != "invoke":
            kind = (op.get("error") or {}).get("kind", "")
            label = op["type"] + (":" + kind if kind else "")
            counts[label] = counts.get(label, 0) + 1
    return counts


def generation_stats(buckets):
    """Per generation: completions, successful transactions, and successful
    transactions with a read and with a write, overall and per row family."""
    stats = {}
    for generation, ops in buckets.items():
        row = stats.setdefault(generation, {"completions": 0, "ok": 0, "ok_reads": 0, "ok_writes": 0,
                                            "families": {}})
        for op in ops:
            if op["type"] == "invoke":
                continue
            row["completions"] += 1
            if op["type"] == "ok":
                functions = {mop[0] for mop in op["value"]}
                row["ok"] += 1
                row["ok_reads"] += "r" in functions
                row["ok_writes"] += "append" in functions
                for family in {family_of(key) for _, key, _ in op["value"]}:
                    counts = row["families"].setdefault(str(family), {"ok_reads": 0, "ok_writes": 0})
                    counts["ok_reads"] += any(f == "r" and family_of(k) == family for f, k, _ in op["value"])
                    counts["ok_writes"] += any(f == "append" and family_of(k) == family for f, k, _ in op["value"])
    return stats


def family_totals(stats):
    """Successful transactions with a read and with a write per family, over every generation."""
    totals = {}
    for row in stats.values():
        for family, counts in row["families"].items():
            total = totals.setdefault(family, {"ok_reads": 0, "ok_writes": 0})
            total["ok_reads"] += counts["ok_reads"]
            total["ok_writes"] += counts["ok_writes"]
    return dict(sorted(totals.items()))


FAULT_EVENTS = ("admit", "pause", "resume", "crash", "checkpoint")


def obligations(buckets, lifecycle, families=("smoke",), live=False, staged=False):
    """Every obligation of the fixed pause/checkpoint/crash scenario that the evidence does not meet.

    Returns (stats, [(class, message)]). A probe or observation that ran and
    failed is a recovery/progress failure; anything absent is incomplete.
    Each selected row family needs its own successful reads and writes. With
    the live lane, at least one live checkpoint must complete and at least one
    checkpoint-crash must stop, kill and recover its generation.
    """
    stats = generation_stats(buckets)
    missing = []

    def useful(generation, when):
        row = stats.get(generation, {}).get("families", {})
        for family in families:
            for key, what in (("ok_reads", "read"), ("ok_writes", "write")):
                if not row.get(family, {}).get(key):
                    missing.append((INCOMPLETE, f"{when}: generation {generation} has no successful {family} "
                                                f"transaction with a {what}"))

    def nearby(index, step, event, reason, generation):
        index += step
        while 0 <= index < len(lifecycle):
            row = lifecycle[index]
            if row.get("event") == event and row.get("reason") == reason and row.get("generation") == generation:
                return row
            if row.get("event") in FAULT_EVENTS:
                return None
            index += step
        return None

    def checked(row, kind, what):
        if row is None or row.get("skipped"):
            missing.append((INCOMPLETE, f"missing {kind} {what}"
                                        + (f" ({row['skipped']})" if row is not None else "")))
        elif not row.get("ok"):
            missing.append((RECOVERY, f"{kind} {what} did not succeed within {row.get('deadline_seconds')}s: "
                                      f"{row.get('failed_attempts')}"))

    faults = [row for row in lifecycle if row.get("event") in ("pause", "resume", "crash", "checkpoint")]
    # A stage schedule runs only stage crashes and the final checkpoint;
    # stage_coverage holds its per-step obligations.
    for name in ("checkpoint",) if staged else ("pause", "crash", "checkpoint"):
        if not any(row["event"] == name for row in faults):
            missing.append((INCOMPLETE, f"scenario: zero {name} faults were applied"))
    if faults and faults[-1]["event"] != "checkpoint":
        missing.append((INCOMPLETE, f"scenario: ended after {faults[-1]['event']} of generation "
                                    f"{faults[-1].get('generation')}, not at its final checkpoint"))
    admits = {row["generation"]: row for row in lifecycle if row.get("event") == "admit"}
    final_checkpoint = faults[-1] if faults and faults[-1]["event"] == "checkpoint" else None
    checkpointed = None
    for index, row in enumerate(lifecycle):
        event, generation = row.get("event"), row.get("generation")
        if event == "admit":
            checked(nearby(index, 1, "probe", "launch", generation), "progress probe after launch of generation",
                    str(generation))
        elif event == "pause" and row.get("observed_state") != "T":
            missing.append((INCOMPLETE, f"pause of generation {generation}: owned PID was not observed stopped "
                                        f"(state {row.get('observed_state')!r})"))
        elif event == "resume":
            checked(nearby(index, 1, "probe", "resume", generation), "progress probe after resume of generation",
                    str(generation))
        elif event in ("crash", "checkpoint"):
            what = f"{event} of generation {generation}"
            checked(nearby(index, -1, "observation", event, generation), "final observation before", what)
            useful(generation, f"before {what}")
            result = row.get("result") or {}
            if event == "crash" and result.get("exit_code") != -9:
                missing.append((INCOMPLETE, f"{what}: SIGKILL was not observed on the owned PID "
                                            f"(exit code {result.get('exit_code')})"))
            if event == "checkpoint":
                if not (result.get("final_checkpoint") or {}).get("verified"):
                    missing.append((INCOMPLETE, f"{what}: checkpoint was not completed and verified"))
                else:
                    checkpointed = generation
            admitted = admits.get(generation + 1) if type(generation) is int else None
            if admitted is None or admitted.get("kind") != event or \
                    admitted.get("baseline_generation") != checkpointed:
                missing.append((INCOMPLETE, f"{what}: generation {generation + 1 if type(generation) is int else '?'}"
                                            f" was not admitted from checkpointed generation {checkpointed}"))
            if row is not final_checkpoint:
                useful(generation + 1 if type(generation) is int else None, f"after {what}")
        elif event == "checkpoint-crash":
            what = f"checkpoint-crash of generation {generation}"
            checked(nearby(index, -1, "observation", event, generation), "final observation before", what)
            useful(generation, f"before {what}")
            useful(generation + 1, f"after {what}")
            seq = row.get("seq")
            stop = next((item for item in lifecycle if item.get("event") == "checkpoint-stop"
                         and item.get("seq") == seq), None)
            if not any(item.get("event") == "checkpoint-request" and item.get("seq") == seq for item in lifecycle):
                missing.append((INCOMPLETE, f"{what}: no checkpoint request was recorded"))
            if stop is None or stop.get("observed_state") != "T":
                missing.append((INCOMPLETE, f"{what}: owned PID was not observed stopped before the kill"))
            elif (stop.get("artifacts") or {}).get("stage") not in barn_process.CHECKPOINT_STAGES:
                missing.append((INCOMPLETE, f"{what}: frozen checkpoint stage was not classified "
                                            f"({(stop.get('artifacts') or {}).get('stage')!r})"))
            if (row.get("result") or {}).get("exit_code") != -9:
                missing.append((INCOMPLETE, f"{what}: SIGKILL was not observed on the owned PID "
                                            f"(exit code {(row.get('result') or {}).get('exit_code')})"))
            if not row.get("forensic"):
                missing.append((INCOMPLETE, f"{what}: no forensic copy of the artifacts was preserved"))
            admitted = admits.get(generation + 1)
            restart = (admitted or {}).get("restart") or {}
            if admitted is None or admitted.get("kind") != event or restart.get("seq") != seq:
                missing.append((INCOMPLETE, f"{what}: generation {generation + 1} was not admitted from its "
                                            f"interrupted checkpoint"))
            else:
                if not (admitted.get("server") or {}).get("recovered_pair"):
                    missing.append((INCOMPLETE, f"{what}: generation {generation + 1} recorded no recovered pair"))
                observation = next((item for item in lifecycle if item.get("event") == "recovery-observation"
                                    and item.get("generation") == generation + 1), None)
                checked(observation, "recovery observation of", what)
        elif event == "checkpoint-request" and row.get("purpose") == "live":
            if not any(item.get("event") == "checkpoint-complete" and item.get("seq") == row.get("seq")
                       and item.get("outputs") for item in lifecycle):
                missing.append((INCOMPLETE, f"live checkpoint {row.get('seq')} of generation {generation}: no "
                                            f"completion with a published pair digest"))
    if live:
        for event, what in (("checkpoint-complete", "completed live checkpoints"),
                            ("checkpoint-crash", "checkpoint-crash faults")):
            if not any(row.get("event") == event for row in lifecycle):
                missing.append((INCOMPLETE, f"scenario: zero {what}"))
    for generation, row in sorted(stats.items()):
        if row["completions"] and not row["ok"]:
            missing.append((INCOMPLETE, f"generation {generation}: {row['completions']} client completions and no "
                                        f"successful transaction (unexamined live interval)"))
    return stats, missing


PANIC_MESSAGES = ("panic in verb call", "panic in eval", "panic in task")
CHECKPOINT_FAILURES = ("checkpoint failed", "final checkpoint failed", "emergency dump failed")
RESTORE_FAILURES = ("failed to restore queued task", "failed to restore suspended task",
                    DROPPED_TASK_MESSAGE)


def log_findings(inputs, lifecycle):
    """Scan each launched generation's runtime logs. Returns ({generation: records}, [(class, message)]).

    One diagnostic is declared: the task lane's deliberately invalidated task
    (task-invalidate rows) is dropped at load with DROPPED_TASK_MESSAGE naming
    its task_id; those records are expected, and each invalidated task needs
    at least one. Otherwise a panic is a correctness violation, a checkpoint or
    task-restore failure a recovery failure, and any other ERROR record is
    unexamined evidence.
    """
    counts, findings = {}, []
    invalid = {row["tid"]: row for row in lifecycle if row.get("event") == "task-invalidate"}
    dropped = set()
    for generation in sorted(row["generation"] for row in lifecycle if row.get("event") == "admit"):
        directory = inputs / f"gen-{generation:03d}"
        try:
            lines = (directory / "logs" / "latest.jsonl").read_text(encoding="utf-8", errors="replace").splitlines()
        except FileNotFoundError:
            findings.append((INCOMPLETE, f"generation {generation}: runtime log logs/latest.jsonl is missing"))
            lines = []
        counts[generation] = 0
        for number, line in enumerate(lines, 1):
            if not line.strip():
                continue
            counts[generation] += 1
            where = f"generation {generation} logs/latest.jsonl:{number}"
            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                findings.append((INCOMPLETE, f"{where}: unparseable runtime log record: {line}"))
                continue
            message = record.get("msg") if isinstance(record, dict) else None
            message = message if isinstance(message, str) else ""
            if message.startswith("PANIC:") or message in PANIC_MESSAGES:
                findings.append((CORRECTNESS, f"{where}: panic: {line}"))
            elif message in CHECKPOINT_FAILURES:
                findings.append((RECOVERY, f"{where}: checkpoint failure: {line}"))
            elif message == DROPPED_TASK_MESSAGE and record.get("task_id") in invalid \
                    and generation > invalid[record["task_id"]]["generation"]:
                dropped.add(record["task_id"])
            elif message in RESTORE_FAILURES:
                findings.append((RECOVERY, f"{where}: task restore failure: {line}"))
            elif isinstance(record, dict) and record.get("level") == "ERROR":
                findings.append((INCOMPLETE, f"{where}: unexamined ERROR record: {line}"))
        try:
            text = (directory / "server.log").read_text(encoding="utf-8", errors="replace")
        except FileNotFoundError:
            findings.append((INCOMPLETE, f"generation {generation}: server.log is missing"))
            continue
        for number, line in enumerate(text.splitlines(), 1):
            if line.startswith(("panic: ", "fatal error: ")):
                findings.append((CORRECTNESS, f"generation {generation} server.log:{number}: Go runtime crash: {line}"))
    for tid, row in sorted(invalid.items()):
        if tid not in dropped:
            findings.append((INCOMPLETE, f"invalid-task policy: no generation after {row['generation']} logged "
                                         f"{DROPPED_TASK_MESSAGE!r} for the invalidated task {tid}"))
    return counts, findings


def analyze(inputs, output, jar, java):
    """Check one finished campaign directory; returns (results, findings)."""
    findings = []

    def find(kind, message, witness=None):
        findings.append({"class": kind, "message": message, **({"witness": witness} if witness else {})})

    lifecycle = load_jsonl(inputs / "lifecycle.jsonl")
    for row in lifecycle:
        if row.get("event") == "failure":
            find(INVALID if row.get("operation") in ("jepsen", "campaign") else RECOVERY, row["error"])
    results = {}
    try:
        results["provenance"] = json.loads((inputs / "provenance.json").read_text(encoding="utf-8"))
    except FileNotFoundError:
        find(INVALID, "no provenance.json: the campaign's inputs are not recorded")
    results["logs"], log_problems = log_findings(inputs, lifecycle)
    for kind, message in log_problems:
        find(kind, message)
    try:
        history = load_jsonl(inputs / "history.jsonl")
    except FileNotFoundError:
        find(INCOMPLETE, "Jepsen wrote no history.jsonl")
        return results, findings
    results["outcomes"] = outcome_counts(history)
    for label, count in results["outcomes"].items():
        if label.endswith((":native-error", ":malformed-response")):
            find(INCOMPLETE, f"{count} client completions were {label}")
        elif label.endswith(tuple(":" + kind for kind in ADAPTER_SIDE)):
            find(INVALID, f"{count} client completions were {label}")
    try:
        journal = load_jsonl(inputs / "journal.jsonl")
    except FileNotFoundError:
        find(INVALID, "no adapter journal.jsonl to reconcile with the Jepsen history")
        return results, findings
    history, results["reconciliation"], contradictions = reconcile(history, journal)
    for message in contradictions:
        find(INVALID, f"journal contradicts history: {message}")
    try:
        buckets, results["split"] = split_generations(history)
    except ValueError as error:
        find(INVALID, f"history cannot be split per generation: {error}")
        return results, findings
    results["checkpoints"] = checkpoint_comparisons(buckets, lifecycle)
    for row in results["checkpoints"]:
        for problem in row["violations"]:
            find(CORRECTNESS, f"generation {row['generation']} ({row['kind']}): {problem}")
    # Campaigns recorded before row families existed ran the smoke rows only.
    families = (results.get("provenance") or {}).get("parameters", {}).get("workloads") or ["smoke"]
    unknown = [name for name in families if name not in FAMILIES]
    if unknown:
        find(INVALID, f"provenance names unknown row families {unknown}")
    families = [name for name in families if name in FAMILIES]
    # Campaigns recorded before the live lane existed have no phase_live.
    parameters = (results.get("provenance") or {}).get("parameters", {})
    # Campaigns recorded before stage schedules existed have no stages.
    stages = parameters.get("stages") or []
    unknown = [name for name in stages if name not in STAGE_SCHEDULE]
    if unknown:
        find(INVALID, f"provenance names unknown stage steps {unknown}")
    live = bool(parameters.get("phase_live")) and not stages
    results["generations"], missing = obligations(buckets, lifecycle, families, live, staged=bool(stages))
    results["families"] = {"selected": families, "totals": family_totals(results["generations"])}
    results["missing_obligations"] = [message for _, message in missing]
    for kind, message in missing:
        find(kind, message)
    results["recoveries"], snapshot_problems = snapshot_checks(lifecycle, journal)
    for kind, message in snapshot_problems:
        find(kind, message)
    receipts = load_jsonl(inputs / "receipts.jsonl") if (inputs / "receipts.jsonl").exists() else []
    parameters = (results.get("provenance") or {}).get("parameters", {})
    if parameters.get("tasks") or any(row.get("event") == "task-plant" for row in lifecycle):
        results["tasks"], task_problems = task_checks(lifecycle, receipts, results["recoveries"], live)
        if not any(row.get("event") == "task-plant" for row in lifecycle):
            task_problems.append((INCOMPLETE, "task-coverage", "task lane selected, but no plant was recorded"))
        for kind, witness, message in task_problems:
            find(kind, message, witness)
    if "effects" in families:
        results["effects"], effect_problems = effect_checks(lifecycle, journal, receipts)
        for kind, witness, message in effect_problems:
            find(kind, message, witness)
    shaped = [name for name in families if name in SHAPED]
    if shaped:
        results["shaped"], shaped_problems = shaped_checks(lifecycle, journal, shaped)
        for kind, witness, message in shaped_problems:
            find(kind, message, witness)
    if stages:
        results["coverage"], coverage_problems = stage_coverage(lifecycle, stages, results["recoveries"])
        for kind, message in coverage_problems:
            find(kind, message)
    stages = {}
    for row in lifecycle:
        if row.get("event") == "checkpoint-stop":
            stage = (row.get("artifacts") or {}).get("stage")
            stages[stage] = stages.get(stage, 0) + 1
    results["checkpoint_stages"] = stages
    results["elle"] = []
    observed = {row["generation"] for row in lifecycle if row.get("event") == "observation"}
    observed |= {row["observed_generation"] for row in lifecycle
                 if row.get("event") == "recovery-observation" and row.get("observed_generation") is not None}
    for generation in sorted(set(buckets) | observed):
        ops = elle_ops(buckets.get(generation, []) + observation_ops(lifecycle, generation))
        if not any(op["type"] == "ok" for op in ops):
            results["elle"].append({"generation": generation, "skipped": "no :ok transactions"})
            continue
        history_path = output / f"gen-{generation:03d}.edn"
        history_path.write_text(elle_history(ops), encoding="utf-8")
        graphs = output / f"graphs-gen-{generation:03d}"
        command = [java, "-jar", str(jar), "-m", "list-append", "-c", "serializable",
                   "--directory", str(graphs), "--max-plot-bytes", "2000000", str(history_path)]
        exit_code, text = check_elle.run(command, output / f"gen-{generation:03d}-elle.log")
        row = {"generation": generation, "ops": len(ops), "command": command, "exit_code": exit_code}
        try:
            row["verdict"] = check_elle.verdict(text, history_path)
        except ValueError as error:
            find(INVALID, f"generation {generation}: {error}")
        verdict = row.get("verdict")
        if verdict == "false":
            row["anomalies"] = elle_anomalies(graphs)
            for anomaly in row["anomalies"]:
                find(CORRECTNESS, f"generation {generation}: Elle rejected the history with {anomaly} "
                                  f"({graphs.name}/{anomaly}.txt)", f"elle:{anomaly}")
            if not row["anomalies"]:
                find(CORRECTNESS, f"generation {generation}: Elle rejected the history (exit {exit_code})")
        elif verdict == "unknown":
            find(INCOMPLETE, f"generation {generation}: Elle could not decide (exit {exit_code})")
        elif verdict == "true" and exit_code != 0:
            find(INVALID, f"generation {generation}: Elle verdict true with exit {exit_code}")
        results["elle"].append(row)
    return results, findings


def verdict_of(findings):
    present = {finding["class"] for finding in findings}
    return next((kind for kind in PRECEDENCE if kind in present), "healthy")


def control_result(control, results):
    """A negative control passes only when its stage step was reached and its
    designated witness is among the recovery findings; any other outcome,
    healthy included, is a missed control."""
    spec = CONTROLS[control]
    observed = sorted({witness for row in results.get("recoveries") or [] for witness in row.get("witnesses", [])}
                      | {finding["witness"] for finding in results.get("findings") or [] if finding.get("witness")})
    if "stage" in spec:
        stage = next((row for row in results.get("coverage") or [] if row["stage"] == spec["stage"]), {})
        reached = stage.get("reached") == stage.get("requested") and stage.get("killed_exit") == -9
    else:
        reached = True  # a live-process fault: no stage step to reach
    return {"name": control, "stage": spec.get("stage"), "witness": spec["witness"], "observed_witnesses": observed,
            "stage_reached": reached, "passed": reached and spec["witness"] in observed}


# ---------------------------------------------------------------------------

def run_campaign(args, output):
    campaign = Campaign(args.barn.resolve(strict=True), args.db, output, socket_transport_factory(), seed=args.seed,
                        stages=args.stages, control=args.control, tasks=args.tasks,
                        effects="effects" in args.workloads,
                        shaped=[name for name in args.workloads if name in SHAPED], dwell=args.dwell)
    adapter = None
    try:
        (output / "provenance.json").write_text(json.dumps(provenance(args, output), indent=2, default=str) + "\n",
                                                encoding="utf-8")
        campaign.launch("initial")
        adapter = AdapterServer(campaign)
        threading.Thread(target=adapter.serve_forever, daemon=True).start()
        command = [args.java, JVM_HEAP, "-Djava.awt.headless=true", "-jar", str(args.jepsen_jar.resolve(strict=True)),
                   "--adapter-port", str(adapter.server_address[1]), "--history", str(output / "history.jsonl"),
                   "--phase1", str(args.phase1), "--phase3", str(args.phase3),
                   "--phase-live", str(args.phase_live), "--rate", str(args.rate),
                   "--stages", str(len(args.stages)), "--stage-seconds", str(args.stage_seconds),
                   "--concurrency", str(args.concurrency), "--seed", str(args.seed),
                   "--families", json.dumps([dict(FAMILIES[name], name=name) for name in args.workloads])]
        campaign.record({"event": "jepsen", "command": command})
        with (output / "jepsen.log").open("wb") as log:
            # Jepsen writes its store/ (history.edn, jepsen.log) under its cwd.
            code = subprocess.run(command, cwd=output, stdout=log, stderr=subprocess.STDOUT,
                                  timeout=args.jepsen_timeout, check=False).returncode
        if code:
            campaign.fail("jepsen", RuntimeError(f"exited {code}; see jepsen.log"))
    except Exception as error:
        campaign.fail("campaign", error)
    finally:
        if adapter is not None:
            adapter.shutdown()
            adapter.server_close()
        campaign.finish()


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--output", type=Path, required=True, help="new evidence directory; existing ones are refused")
    parser.add_argument("--elle-jar", type=Path, required=True, help="built pinned elle-cli jar (see .github/workflows/elle.yml)")
    parser.add_argument("--java", default="java")
    parser.add_argument("--analyze", type=Path, help="re-check an existing campaign directory without launching anything")
    parser.add_argument("--barn", type=Path, help="Linux Barn executable")
    parser.add_argument("--db", type=Path, help="public Test.db fixture")
    parser.add_argument("--jepsen-jar", type=Path, help="tests/jepsen-checkpoint uberjar")
    parser.add_argument("--phase1", type=int, default=60)
    parser.add_argument("--phase3", type=int, default=90)
    parser.add_argument("--phase-live", type=int, default=60,
                        help="seconds of live checkpoints and checkpoint-crash recoveries; 0 disables the lane")
    parser.add_argument("--rate", type=int, default=20, help="total transactions per second")
    parser.add_argument("--concurrency", type=int, default=8)
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument("--jepsen-timeout", type=int, default=900)
    parser.add_argument("--workloads", default="smoke",
                        help=f"comma-separated row families to mix: {', '.join(FAMILIES)}")
    parser.add_argument("--stages", default="",
                        help="elle build only: 'all' or comma-separated STAGE_SCHEDULE steps "
                             f"({', '.join(STAGE_SCHEDULE)}); replaces the fixed and live phases")
    parser.add_argument("--stage-seconds", type=int, default=10, help="client seconds around each stage step")
    parser.add_argument("--tasks", action="store_true",
                        help="task lane: plant held test tasks before every graceful checkpoint and before each "
                             "generation's first live checkpoint, release them after the restart's recovery read")
    parser.add_argument("--dwell", type=int, default=0,
                        help="iterations of a tick loop between each authority/topology verb's reads and writes, "
                             "so concurrent transactions overlap")
    parser.add_argument("--control", choices=sorted(CONTROLS),
                        help="elle build only: run the negative control's fault and stage step; exit 0 only if "
                             "its designated witness is reported")
    args = parser.parse_args(argv)
    args.workloads = args.workloads.split(",")
    unknown = [name for name in args.workloads if name not in FAMILIES]
    if unknown or len(set(args.workloads)) != len(args.workloads):
        parser.error(f"--workloads must name distinct families from {', '.join(FAMILIES)}")
    args.stages = list(STAGE_SCHEDULE) if args.stages == "all" else [name for name in args.stages.split(",") if name]
    if [name for name in args.stages if name not in STAGE_SCHEDULE] or len(set(args.stages)) != len(args.stages):
        parser.error(f"--stages must be 'all' or distinct steps from {', '.join(STAGE_SCHEDULE)}")
    if args.control:
        if args.stages:
            parser.error("--control selects its own stage step; do not combine it with --stages")
        args.stages = [CONTROLS[args.control]["stage"]] if "stage" in CONTROLS[args.control] else []
        family = CONTROLS[args.control].get("family")
        if family and family not in args.workloads:
            parser.error(f"--control {args.control} needs the {family} row family in --workloads")
    if not args.elle_jar.is_file():
        parser.error("--elle-jar must name the built pinned elle-cli jar")
    if args.analyze is None and not (args.barn and args.db and args.jepsen_jar):
        parser.error("a campaign needs --barn, --db and --jepsen-jar (or --analyze DIR)")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    inputs = output
    if args.analyze is None:
        run_campaign(args, output)
    else:
        inputs = args.analyze.resolve(strict=True)
    try:
        results, findings = analyze(inputs, output, args.elle_jar.resolve(), args.java)
    except FileNotFoundError as error:
        results, findings = {}, [{"class": INVALID, "message": f"campaign directory is incomplete: {error}"}]
    verdict = verdict_of(findings)
    results.update(verdict=verdict, classes=[kind for kind in PRECEDENCE
                                             if any(f["class"] == kind for f in findings)],
                   findings=findings, argv=sys.argv if argv is None else argv, inputs=str(inputs),
                   analyze_command=[sys.executable, str(Path(__file__).resolve()), "--analyze", str(inputs),
                                    "--output", str(inputs) + "-analysis", "--elle-jar", str(args.elle_jar),
                                    "--java", args.java])
    (output / "results.json").write_text(json.dumps(results, indent=2, default=str) + "\n", encoding="utf-8")
    print(json.dumps({key: results.get(key) for key in ("outcomes", "split", "reconciliation")}, default=str),
          flush=True)
    for row in results.get("checkpoints", []):
        print(f"checkpoint comparison generation {row['generation']} ({row['kind']}): "
              f"{len(row['violations'])} violations", flush=True)
    for row in results.get("elle", []):
        print(f"elle generation {row['generation']}: {row.get('verdict', row.get('skipped'))}", flush=True)
    for family, counts in (results.get("families") or {}).get("totals", {}).items():
        print(f"family {family}: {counts['ok_reads']} successful transactions with a read, "
              f"{counts['ok_writes']} with a write", flush=True)
    print(f"checkpoint-crash stages: {results.get('checkpoint_stages')}", flush=True)
    for row in results.get("recoveries", []):
        print(f"recovery after generation {row['generation']} (stage {row.get('stage')}): "
              f"{row.get('selected_cut', row.get('skipped', 'no permitted cut'))}", flush=True)
    for row in results.get("tasks", []):
        print(f"task plant {row['plant']} ({row['point']}, generation {row['generation']}): launches "
              f"{[(item['generation'], item['status']) for item in row['launches']]}, releases "
              f"{[(item['generation'], item['complete']) for item in row['releases']]}, witnesses {row['witnesses']}",
              flush=True)
    if results.get("effects"):
        print(f"effects: {json.dumps(results['effects'])}", flush=True)
    for family, row in (results.get("shaped") or {}).items():
        print(f"{family}: {row['checked']} checked transactions, {row['denied']} denied micro-ops, missing "
              f"{row['missing']}, witnesses {row['witnesses']}, coverage {json.dumps(row['coverage'])}", flush=True)
    for row in results.get("coverage", []):
        print(f"stage {row['stage']}: requested {row['requested']}, reached {row.get('reached')}, "
              f"state {row.get('stopped_state')}, frozen {row.get('frozen_stage')}, exit {row.get('killed_exit')}, "
              f"cut {row.get('selected_cut') or row.get('candidate_cuts')}: {row['verdict']}", flush=True)
    for finding in findings:
        print(f"{finding['class']}: {finding['message']}", file=sys.stderr)
    print(f"verdict: {verdict}", flush=True)
    control = (results.get("provenance") or {}).get("parameters", {}).get("control")
    if control:
        # A negative control is expected to fail with its witness; that is its pass.
        results["control"] = control_result(control, results)
        (output / "results.json").write_text(json.dumps(results, indent=2, default=str) + "\n", encoding="utf-8")
        print(f"control {control}: witness {results['control']['witness']} "
              f"{'reported' if results['control']['passed'] else 'MISSING'} "
              f"(observed {results['control']['observed_witnesses']})", flush=True)
        return 0 if results["control"]["passed"] else CONTROL_MISSED_EXIT
    return EXIT_CODES[verdict]


if __name__ == "__main__":
    sys.exit(main())
