"""Actual learned Lumos/Nox, through an ordinary player's real command socket.

Pinned source: FAB6957ACFFA1DB8BC1C4B911FB53A8E8D6E605F4DE2A31B9F8E4FA874C3200D.
Staff setup authorizes only a fresh portrait and teaches the ORIGINAL catalogue
objects. Native training/rolls/recovery remain intact; no spell/spec/skill edits.
"""
from __future__ import annotations

import json
import math
from pathlib import Path
import time
import uuid

from bench_players import MooObj

from mongoose_scenario_support import (ScenarioFailure, actor_command,
                                       connect_actor, object_ref, require,
                                       wait_until)

SPELLS = {"lumos": "#20835", "nox": "#20959"}
MAX_ATTEMPTS_PER_SPELL = 8
SNAPSHOT_FIELDS = ("actor", "wizard", "is_player", "location", "wand", "wand_location",
                   "lit", "wand_emission", "room_light", "joules", "settled_at", "capacity_j",
                   "refill_w", "joules_per_focus", "ready_at", "pending", "receipt",
                   "receipt_count", "debit_count", "matching_debits", "now")
RECEIPT_FIELDS = ("id", "phase", "spell", "quoted_focus", "paid_focus", "roll", "quality",
                  "formed", "mastered_utility", "outcome", "result", "at", "worker", "reason")


def _read(control, code, timeout=30):
    value, _ = control.eval(code, timeout=timeout)
    return _references(value)


def _references(value):
    """Preserve integer quantities while making actual parsed object IDs durable."""
    if isinstance(value, MooObj):
        return f"#{int(value)}"
    if isinstance(value, dict):
        return {_references(key): _references(item) for key, item in value.items()}
    if isinstance(value, list):
        return [_references(item) for item in value]
    return value


def _record(options, stage, evidence):
    with (Path(options.run_dir) / "magic-actions.jsonl").open("a", encoding="utf-8") as stream:
        stream.write(json.dumps({"stage": stage, **evidence}, allow_nan=False) + "\n")


def _prerequisite(control, options, name, code):
    _record(options, "prerequisite-issued", {"name": name, "observer": code})
    value = _read(control, code)
    _record(options, "prerequisite-returned", {"name": name, "value": value})
    return value


def _remaining(deadline):
    remaining = deadline - time.monotonic()
    require(remaining > 0, "magic workload deadline expired")
    return remaining


def _terminal_output(connection):
    """Retain asynchronous caster text emitted outside the input bracket."""
    lines = []
    deadline = time.monotonic() + 0.05
    while True:
        line = connection.read_line(deadline)
        if line is None:
            return lines
        lines.append(line)
        require(len(lines) <= 1000, "unbounded asynchronous actor output", lines)


def _same_effect(before, after):
    return (before["lit"] == after["lit"] and
            math.isclose(before["room_light"], after["room_light"], abs_tol=1e-8) and
            math.isclose(before["wand_emission"], after["wand_emission"], abs_tol=1e-8))


def validate_terminal(spell, before, after, seen_ids):
    """Independent effect/resource oracle; command acknowledgements cannot pass."""
    require(before["wizard"] == after["wizard"] == 0 and
            before["is_player"] == after["is_player"] == 1 and
            all(before[key] == after[key] for key in ("actor", "location", "wand", "wand_location")),
            "ordinary actor/focus identity changed during casting", after)
    receipt = after["receipt"]
    require(receipt.get("phase") == "terminal" and not after["pending"],
            "magic attempt did not reach a terminal receipt", after)
    cast_id = receipt.get("id")
    require(isinstance(cast_id, str) and cast_id and cast_id not in seen_ids and
            cast_id != before["receipt"].get("id"), "missing/reused cast identity", receipt)
    require(object_ref(receipt.get("spell")) == SPELLS[spell],
            "terminal receipt belongs to another catalogue spell", receipt)
    outcome = receipt.get("outcome")
    require(outcome in {"applied", "fizzle"}, "unexpected magic terminal outcome", receipt)
    formed = receipt.get("formed")
    require(formed == (1 if outcome == "applied" else 0), "formation/outcome mismatch", receipt)
    if not receipt.get("mastered_utility"):
        roll, quality = receipt.get("roll"), receipt.get("quality")
        require(all(isinstance(value, (float, int)) and not isinstance(value, bool) and
                    math.isfinite(value) and 0 <= value <= 100 for value in (roll, quality)) and
                bool(formed) == (roll < quality), "native formation roll mismatch", receipt)
    quote, paid = receipt.get("quoted_focus"), receipt.get("paid_focus")
    require(all(isinstance(v, (int, float)) and not isinstance(v, bool) and
                math.isfinite(v) and v > 0 for v in (quote, paid)),
            "invalid actual quote/payment", receipt)
    require(math.isclose(paid, quote * (1 if outcome == "applied" else 0.5), abs_tol=1e-8),
            "actual payment does not match formed/fizzle policy", receipt)
    debit = after["matching_debits"]
    require(len(debit) == 1 and debit[0]["id"] == cast_id and
            math.isclose(debit[0]["focus"], paid, abs_tol=1e-8),
            "terminal payment lacks one matching reservoir debit", after)
    require(after["debit_count"] == min(32, before["debit_count"] + 1),
            "unexpected debit multiplicity", after)
    require(after["ready_at"] > before["now"], "shared recovery was not started", after)
    require(before["capacity_j"] == after["capacity_j"] and
            before["refill_w"] == after["refill_w"] and
            after["settled_at"] >= before["now"], "reservoir policy/settlement changed", after)
    # Payment settles somewhere between the two actual observations. Preserve
    # refill rather than pretending wall-separated level readings equal spending.
    lower = min(before["capacity_j"], before["joules"] + before["refill_w"] *
                max(0, before["now"] - before["settled_at"]))
    upper = min(before["capacity_j"], before["joules"] + before["refill_w"] *
                max(0, after["settled_at"] - before["settled_at"]))
    consumed = paid * after["joules_per_focus"]
    require(lower - consumed - 0.05 <= after["joules"] <= upper - consumed + 0.05,
            "raw reservoir disagrees with payment plus bounded natural refill", after)
    result = receipt.get("result", {})
    if outcome == "fizzle":
        require(result.get("effect_applied") == 0 and _same_effect(before, after),
                "fizzle changed physical illumination", {"before": before, "after": after})
    else:
        require(result.get("effect_applied") == 1,
                "applied receipt lacks actual effect", receipt)
        require(any(after["wand"] in target.get("wands", []) for target in result.get("targets", [])),
                "applied receipt does not identify the actual owned wand", receipt)
        if spell == "lumos":
            require(before["lit"] == 0 and after["lit"] == 1 and
                    after["wand_emission"] > 0 and after["room_light"] > before["room_light"],
                    "Lumos did not light the actual wand and room", {"before": before, "after": after})
        else:
            require(before["lit"] == 1 and after["lit"] == 0 and
                    after["wand_emission"] == 0 and after["room_light"] < before["room_light"],
                    "Nox did not extinguish the actual wand and room", {"before": before, "after": after})
    seen_ids.add(cast_id)
    return {"cast_id": cast_id, "outcome": outcome, "paid_focus": paid,
            "resource_interval_j": [lower - consumed, upper - consumed],
            "physical_effect_verified": True}


def setup(control, options):
    try:
        return _setup(control, options)
    except Exception as primary:
        fixture = getattr(options, "magic_fixture", None)
        evidence = {"primary_failure_type": type(primary).__name__,
                    "primary_failure": str(primary),
                    "primary_evidence": getattr(primary, "evidence", None),
                    "native_output": getattr(primary, "native_output", None),
                    "partial_fixture": fixture}
        if fixture:
            try:
                evidence["cleanup"] = cleanup(control, fixture)
            except Exception as secondary:
                evidence.update(cleanup_failure_type=type(secondary).__name__,
                                cleanup_failure=str(secondary),
                                cleanup_evidence=getattr(secondary, "evidence", None),
                                cleanup_native_output=getattr(secondary, "native_output", None))
        raise ScenarioFailure("magic setup failed: " + str(primary), evidence) from primary


def _setup(control, options):
    control.magic_options = options
    path = Path(options.run_dir) / "magic-actions.jsonl"
    with path.open("x", encoding="utf-8"):
        pass
    token = uuid.uuid4().hex[:12]
    fixture = {"scenario": "magic", "name": "Magic" + token,
               "seed": options.seed, "native_rng_reseeded": False,
               "max_attempts_per_spell": MAX_ATTEMPTS_PER_SPELL}
    options.magic_fixture = fixture
    # Factory creates a real ordinary player, self-owned, registered normally.
    fixture["actor"] = object_ref(_read(control,
        f'return $account_manager:create_player({json.dumps(fixture["name"])});'))
    admission = _read(control, f'a={fixture["actor"]}; return {{is_player(a), a.wizard, a.programmer}};')
    require(admission == [1, 0, 0], "production factory did not create an ordinary player", admission)
    _record(options, "setup", {"factory": "$account_manager:create_player", "actor": fixture["actor"], "admission": admission})
    room = _read(control, 'r=$recycler:_create($room, player); r.name="private magic workload room"; '
                 'r.dark=1; r.ambient_light=0.0; r.outdoor=0; return {r,$hp_magic:generation(r)};')
    fixture["room"], fixture["room_generation"] = object_ref(room[0]), room[1]
    portrait = _read(control,
        f't=$recycler:_create($hp_tutor, player); t.name="workload portrait"; t.aliases={{"portrait"}}; '
        f't.teaches={{#20835,#20959}}; move(t,{fixture["room"]}); $hp_training:authorize(t,1); '
        'return {t,$hp_magic:generation(t)};')
    fixture["tutor"], fixture["tutor_generation"] = object_ref(portrait[0]), portrait[1]
    _read(control, f'{fixture["actor"]}:moveto({fixture["room"]}); return 1;')
    options.magic_connection = connect_actor(control, fixture["actor"], options)
    # Login hooks can move the character: place the owned actor after login too.
    _read(control, f'{fixture["actor"]}:moveto({fixture["room"]}); return 1;')
    connection = options.magic_connection
    for command in ("fit wand at portrait", "wield wand", "learn Lumos from portrait", "learn Nox from portrait"):
        _record(options, "setup-command", actor_command(connection, command, options.deadline_seconds))
    wand = _read(control,
        f'a={fixture["actor"]}; w=a:wielded(); '
        'if (!valid(w) || !isa(w,$hp_wand) || w:ready_for(a)!=1) raise(E_INVARG); endif '
        'return {w,$hp_magic:generation(w),$hp_magic:generation(a)};')
    fixture["wand"], fixture["wand_generation"], fixture["actor_generation"] = object_ref(wand[0]), wand[1], wand[2]
    actor = fixture["actor"]
    fixture["account_guard"] = _prerequisite(control, options, "account-identity",
        f'a={actor}; return string_hash(a.account.username);')
    prerequisites = {
        "account_bound": _prerequisite(control, options, "account-membership",
            f'a={actor}; return ((typeof(a.account)==WAIF) && ((a in a.account.players)!=0));'),
        "lessons": _prerequisite(control, options, "stored-lessons",
            f'return {actor}.hp_state["lessons"];'),
        "rank": _prerequisite(control, options, "earned-school-rank",
            f'return {actor}.hp_state["school_ranks"]["charms"];'),
        # Training seeds raw proficiency; quality_for additionally settles effects.
        "proficiency": _prerequisite(control, options, "raw-training-seeds",
            f'a={actor}; return {{$skills:proficiency(a,"hp_spell_lumos"),$skills:proficiency(a,"hp_spell_nox"),'
            '$skills:proficiency(a,"hp_charms"),$skills:proficiency(a,"hp_concentration")};'),
        "real_spells": _prerequisite(control, options, "original-catalogue-spells",
            'return {#20835.hp_spec["id"],#20835.hp_spec["enabled"],#20959.hp_spec["id"],#20959.hp_spec["enabled"]};')}
    require(prerequisites["real_spells"] == ["lumos", 1, "nox", 1] and
            prerequisites["rank"] == 1 and prerequisites["proficiency"] == [60.0] * 4 and
            prerequisites["lessons"]["lumos"]["wand"] == 1 and
            prerequisites["lessons"]["nox"]["wand"] == 1 and prerequisites["account_bound"],
            "real ordinary tutorial prerequisites missing", prerequisites)
    fixture["prerequisites"] = prerequisites
    state = snapshot(control, fixture)
    require(state["lit"] == 0 and state["room_light"] == 0 and not state["pending"],
            "fresh magic fixture is not dark and idle", state)
    fixture["initial_state"] = state
    _record(options, "setup-complete", {"fixture": fixture})
    return fixture


def snapshot(control, fixture):
    a, w, room = (object_ref(fixture[key]) for key in ("actor", "wand", "room"))
    wire = _read(control, f'a={a}; w={w}; room={room}; '
        'if (!valid(a) || !valid(w) || !valid(room)) raise(E_INVARG); endif '
        f'if (!equal(a.hp_generation,{json.dumps(fixture["actor_generation"])}) || '
        f'!equal(w.hp_generation,{json.dumps(fixture["wand_generation"])}) || '
        f'!equal(room.hp_generation,{json.dumps(fixture["room_generation"])})) raise(E_INVARG); endif '
        's=$hp_magic:state(a); r=a.reservoirs[$wizarding]; rc=s["receipts"] ? s["receipts"][$] | []; '
        'debits=r:get("hp_debits",{}); matches={}; for debit in (debits) '
        'if (debit["id"]==rc:get("id","")) matches={@matches,debit}; endif endfor '
        'p=s["pending"]; pending=p ? ["id"->p["id"],"phase"->p["phase"],"task"->p["task"],"deadline"->p["deadline"]] | []; '
        'keys={' + ','.join(json.dumps(key) for key in RECEIPT_FIELDS) + '}; compact_rc={}; '
        'for index in [1..length(keys)] if (maphaskey(rc,keys[index])) '
        'compact_rc={@compact_rc,{index,rc[keys[index]]}}; endif endfor '
        'wire={a,a.wizard,is_player(a),a.location,w,w.location,w:is_lit(),w:light_emission(),'
        'room:light_level(),r["joules"],r["settled_at"],$wizarding:capacity_for(a),'
        '$wizarding:refill_watts(a),$wizarding.joules_per_unit,a.ability_ready_at,'
        'pending,compact_rc,length(s["receipts"]),length(debits),matches,ftime()}; '
        'return {length(toliteral(wire)),wire};')
    return _decode_snapshot(wire)


def _decode_snapshot(wire):
    """One native observation, positional transport with all oracle fields intact.

    Receipt fields are projected; the complete native result map is retained.
    Sparse pairs preserve missing fields versus fields with zero/empty values.
    """
    require(isinstance(wire, list) and len(wire) == 2 and
            type(wire[0]) is int and 0 <= wire[0] <= 900 and
            isinstance(wire[1], list) and len(wire[1]) == len(SNAPSHOT_FIELDS),
            "invalid or oversized coherent magic observation", wire)
    state = dict(zip(SNAPSHOT_FIELDS, wire[1]))
    rows = state["receipt"]
    require(isinstance(rows, list), "invalid compact receipt", rows)
    receipt = {}
    for row in rows:
        require(isinstance(row, list) and len(row) == 2 and type(row[0]) is int and
                1 <= row[0] <= len(RECEIPT_FIELDS), "invalid compact receipt field", row)
        key = RECEIPT_FIELDS[row[0] - 1]
        require(key not in receipt, "duplicate compact receipt field", row)
        receipt[key] = row[1]
    state["receipt"] = receipt
    state["wire_length"] = wire[0]
    state["receipt_fields_observed"] = list(RECEIPT_FIELDS)
    return state


def _ready(control, fixture, deadline):
    return wait_until(lambda: snapshot(control, fixture),
                      lambda state: not state["pending"] and state["now"] >= state["ready_at"],
                      _remaining(deadline), description="native shared ability recovery")["state"]


def _refusal(control, fixture, options, command, expected, deadline):
    before = _ready(control, fixture, deadline)
    response = actor_command(options.magic_connection, command, _remaining(deadline))
    _record(options, "refusal-command", response)
    after = snapshot(control, fixture)
    row = {"command": command, "response": response, "before": before, "after": after}
    _record(options, "refusal-observed", row)
    require(any(expected in line for line in response["output"]), "expected ordinary refusal was absent", row)
    require(not after["pending"] and after["receipt"] == before["receipt"] and
            after["receipt_count"] == before["receipt_count"] and
            after["debit_count"] == before["debit_count"] and
            after["joules"] == before["joules"] and after["settled_at"] == before["settled_at"] and
            after["ready_at"] == before["ready_at"] and _same_effect(before, after),
            "refusal reserved, spent, or changed physical state", row)
    row["no_effect_verified"] = True
    return row


def run(control, fixture, options):
    deadline = time.monotonic() + options.deadline_seconds
    seen = set()
    evidence = {"completed_iterations": 0, "attempts": [], "fizzles": 0,
                "productive_lumos": 0, "productive_nox": 0, "negative_controls": []}
    for command, expected in (("weave nox", "You have no lit wand to put out."),
                              ("weave lumos with 0 focus", "Choose between zero and 10000 focus, subject to the spell's smaller limit.")):
        evidence["negative_controls"].append(_refusal(control, fixture, options, command, expected, deadline))
    for iteration in range(options.iterations):
        for spell in ("lumos", "nox"):
            for attempt in range(MAX_ATTEMPTS_PER_SPELL):
                before = _ready(control, fixture, deadline)
                require(before["wizard"] == 0 and before["is_player"] == 1 and
                        before["location"] == fixture["room"] and before["wand_location"] == fixture["actor"],
                        "ordinary actor/focus left the owned fixture", before)
                response = actor_command(options.magic_connection, "weave " + spell, _remaining(deadline))
                row = {"iteration": iteration + 1, "spell": spell, "attempt": attempt + 1,
                       "before": before, "response": response}
                _record(options, "attempt-issued", row)
                observed = wait_until(lambda: snapshot(control, fixture),
                    lambda state: state["receipt"].get("id") != before["receipt"].get("id") and
                                  state["receipt"].get("phase") == "terminal" and not state["pending"],
                    _remaining(deadline), description=spell + " actual terminal receipt")
                row.update(after=observed["state"], observations=observed["observations"],
                           completion_wall_seconds=observed["elapsed_wall_seconds"],
                           asynchronous_actor_output=_terminal_output(options.magic_connection))
                _record(options, "terminal-observed", row)
                row["validation"] = validate_terminal(spell, before, row["after"], seen)
                evidence["attempts"].append(row)
                _record(options, "attempt-validated", row)
                if row["validation"]["outcome"] == "applied":
                    evidence["productive_" + spell] += 1
                    break
                evidence["fizzles"] += 1
            else:
                raise ScenarioFailure("natural formation exhausted its bounded attempt budget", evidence)
        evidence["completed_iterations"] += 1
        _record(options, "productive-pair-complete", {"completed_iterations": evidence["completed_iterations"]})
    final = snapshot(control, fixture)
    require(final["lit"] == 0 and final["room_light"] == fixture["initial_state"]["room_light"],
            "terminal Nox did not restore actual room illumination", final)
    evidence["final_state"] = final
    evidence["attempt_count"] = len(evidence["attempts"])
    evidence["scope"] = "real typed commands, native preparation/formation/recovery; observer polling included"
    return evidence


def cleanup(control, fixture):
    # Keep the connection/options out of the JSON fixture.
    options = getattr(control, "magic_options", None)
    require(options is not None, "magic cleanup requires its setup options")
    needed = {"actor", "wand", "tutor", "room", "actor_generation", "wand_generation",
              "tutor_generation", "room_generation", "account_guard"}
    missing = sorted(needed.difference(fixture))
    require(not missing, "partial magic fixture lacks proven cleanup identity; retained in owned copy",
            {"missing": missing, "partial_fixture": fixture})
    state = snapshot(control, fixture)
    if state["pending"] and state["pending"]["phase"] == "prepared":
        _record(options, "cleanup-cancel", actor_command(options.magic_connection, "cancelspell", 10))
    idle = wait_until(lambda: snapshot(control, fixture), lambda value: not value["pending"],
                      10, description="owned magic worker natural terminal/cancellation")
    options.magic_connection.close()
    result = _read(control,
        f'a={fixture["actor"]}; w={fixture["wand"]}; t={fixture["tutor"]}; room={fixture["room"]}; '
        'if (!valid(a) || !valid(w) || !valid(t) || !valid(room)) raise(E_INVARG); endif '
        f'if (!equal(a.hp_generation,{json.dumps(fixture["actor_generation"])}) || '
        f'!equal(w.hp_generation,{json.dumps(fixture["wand_generation"])}) || '
        f'!equal(t.hp_generation,{json.dumps(fixture["tutor_generation"])}) || '
        f'!equal(room.hp_generation,{json.dumps(fixture["room_generation"])})) raise(E_INVARG); endif '
        f'if (string_hash(a.account.username)!={json.dumps(fixture["account_guard"])} || !(a in a.account.players)) raise(E_PERM); endif '
        'account=a.account; $hp_training:authorize(t,0); $hp_training:_forget(a); '
        'recycle(w); recycle(t); account:delete_character(a); '
        'if (valid(a) || valid(w) || valid(t) || room.contents) raise(E_INVARG); endif '
        '$test_runner:drain({room},2); recycle(room); '
        'return ["actor_removed"->!valid(a),"wand_removed"->!valid(w),"tutor_removed"->!valid(t),'
        '"room_removed"->!valid(room),"private_account_retained"->1];')
    require(all(result.get(key) == 1 for key in ("actor_removed", "wand_removed", "tutor_removed", "room_removed")),
            "owned magic fixture cleanup incomplete", result)
    result["worker_idle_observations"] = idle["observations"]
    result["account_retention_reason"] = "normal delete_character removed its player; no account registry deletion API established"
    _record(options, "cleanup-complete", result)
    return result
