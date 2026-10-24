"""Ordinary crew flying a newly constructed Type 6 shuttle in an owned world.

Source contract: Oct12T2349 checkpoint FAB6957ACFFA1DB8BC1C4B911FB53A8E8D6E605F4DE2A31B9F8E4FA874C3200D.
Inspected with moodb: #4160:create/fixture_capture/recycle_test_ship,
#636:create_space_object, #3954:users/man_user/setspeed/heading,
#4158:authorize_action/set_output/allocate/boot_computer,
#19752:bootstrap/grant, #4123:set_speed/cut_engines and #4129:cycle_engines.
Connected ordinary membership is mandatory. Setup grants access only to the
new ship. Flight never changes velocity/fuel directly or calls a physics cycle.
"""
from __future__ import annotations

import json
import math
import uuid

from mongoose_scenario_support import (
    ScenarioFailure, actor_command, connect_actor, object_ref, require, wait_until,
)

_connections = {}

# Fixed wire order keeps the native literal below the eval printer limit.
# All admission, kinematic, fuel and component fields still share one eval.
_SNAPSHOT_FIELDS = (
    "actor_is_player", "actor_wizard", "actor_connected", "actor_location",
    "actor_manning", "console_feature", "operate_allowed", "console_ship",
    "universe_member", "cycle", "position", "velocity", "heading",
    "desired_heading", "desired_speed", "engines_cut", "online",
    "computer_online", "mass", "tanks", "engines", "thrusters", "reactors",
)


def _decode_snapshot(wire, native_output):
    evidence = {"snapshot_wire": wire, "native_output": native_output,
                "field_order": list(_SNAPSHOT_FIELDS)}
    require(isinstance(wire, list) and len(wire) == len(_SNAPSHOT_FIELDS),
            "flight snapshot must be a complete 23-field native list", evidence)
    state = dict(zip(_SNAPSHOT_FIELDS, wire))

    def integer(value):
        return isinstance(value, int) and not isinstance(value, bool) and value >= 0

    def number(value):
        return (isinstance(value, (int, float)) and not isinstance(value, bool)
                and math.isfinite(value))

    for key in ("actor_connected", "actor_location", "actor_manning", "console_feature",
                "console_ship", "universe_member", "cycle"):
        require(integer(state[key]), "invalid flight snapshot integer: " + key, evidence)
    for key in ("actor_is_player", "actor_wizard", "operate_allowed", "engines_cut",
                "online", "computer_online"):
        require(isinstance(state[key], (int, bool)) and state[key] in (0, 1),
                "invalid flight snapshot flag: " + key, evidence)
    for key in ("position", "velocity", "heading", "desired_heading"):
        vector = state[key]
        require(isinstance(vector, list) and len(vector) == 3 and all(map(number, vector)),
                "invalid flight snapshot vector: " + key, evidence)
    for key in ("desired_speed", "mass"):
        require(number(state[key]) and state[key] >= 0,
                "invalid flight snapshot scalar: " + key, evidence)
    for key, width in (("tanks", 5), ("engines", 6), ("thrusters", 6), ("reactors", 5)):
        rows = state[key]
        require(isinstance(rows, list), "invalid flight snapshot components: " + key, evidence)
        for row in rows:
            require(isinstance(row, list) and len(row) == width and
                    integer(row[0]) and row[0] > 0 and integer(row[1]),
                    "invalid flight snapshot component row: " + key, evidence)
            if key != "reactors":
                require(isinstance(row[2], str),
                        "invalid flight snapshot component name/type: " + key, evidence)
            start = 2 if key == "reactors" else 3
            require(all(number(value) and value >= 0 for value in row[start:]),
                    "invalid flight snapshot component quantities: " + key, evidence)
    return state


def _evaluate(control, code, timeout=60):
    value, _ = control.eval(code, timeout=timeout)
    return value


def _length(vector):
    require(isinstance(vector, list) and len(vector) == 3,
            "flight requires a three-dimensional vector", vector)
    require(all(isinstance(x, (int, float)) and math.isfinite(x) for x in vector),
            "flight vector must be finite", vector)
    return math.sqrt(sum(x * x for x in vector))


def _distance(a, b):
    _length(a)
    _length(b)
    return _length([x - y for x, y in zip(a, b)])


def _fuel(state):
    return sum(row[3] for row in state["tanks"])


def _require_station(state, fixture):
    require(state["actor_is_player"] == 1 and state["actor_wizard"] == 0,
            "flight actor must remain an ordinary player", state)
    require(state["actor_connected"] > 0 and state["actor_location"] == fixture["room"],
            "ordinary flight actor lost its connected station context", state)
    require(state["actor_manning"] > 0 and state["console_feature"] > 0,
            "ordinary actor is not actually manning the console", state)
    require(state["operate_allowed"] == 1 and state["console_ship"] == fixture["ship"],
            "console admission or vessel binding was lost", state)
    require(state["universe_member"] > 0, "ship left the natural universe", state)


def _motion_completed(before, after, target_speed, target_heading):
    """Desired values alone, drifting and fuel-independent motion cannot pass."""
    return (after["cycle"] > before["cycle"]
            and _distance(after["position"], before["position"]) > 1e-9
            and _distance(after["velocity"], before["velocity"]) > 1e-9
            and abs(_length(after["velocity"]) - target_speed) <= 1e-6
            and _distance(after["heading"], target_heading) <= 1e-6
            and _distance(after["heading"], before["heading"]) > 1e-9
            and _fuel(after) < _fuel(before)
            and not after["engines_cut"])


def _brake_completed(before, after):
    return (after["cycle"] > before["cycle"]
            and after["desired_speed"] == 0
            and _length(before["velocity"]) > 1e-9
            and _length(after["velocity"]) <= 1e-9
            and _fuel(after) < _fuel(before)
            and not after["engines_cut"])


def snapshot(control, fixture):
    ship, actor, console = (object_ref(fixture[k]) for k in ("ship", "actor", "console"))
    wire, native_output = control.eval(f"""
        s = {ship}; a = {actor}; c = {console};
        if ((!valid(s)) || (!valid(a)) || (!valid(c))) raise(E_INVARG); endif
        tanks = {{}}; engines = {{}}; thrusters = {{}}; reactors = {{}};
        for i in [1..length(s.systems)]
          sys = s.systems[i];
          if (isa(sys.class, $space_systems.fuel_tank))
            tanks = {{@tanks, {{i, sys.class, sys.fuel_type, sys.current_fuel, sys.class.max_fuel}}}};
          elseif (isa(sys.class, $space_systems.engine))
            engines = {{@engines, {{i, sys.class, sys.class.name, sys.current_power, sys.class.optimal_power, sys.condition}}}};
          elseif (isa(sys.class, $space_systems.thrusters))
            thrusters = {{@thrusters, {{i, sys.class, sys.class.name, sys.current_power, sys.class.optimal_power, sys.condition}}}};
          elseif (isa(sys.class, $space_systems.reactor))
            reactors = {{@reactors, {{i, sys.class, sys.current_output, sys.desired_output, sys.condition}}}};
          endif
        endfor
        core = $computer_core:of_ship(s);
        return {{is_player(a), a.wizard, (a in connected_players()), a.location,
          (a in c:users()), (c in a.features), core:allows(a, "operate"), c.ship,
          (s in s.universe.objects), s.universe.current_cycle,
          s.position, s.velocity, s.heading, s.desired_heading, s.desired_velocity,
          s.engines_cut, s.online, valid(s:find_online_computer()), s:total_mass(),
          tanks, engines, thrusters, reactors}};
    """, timeout=60)
    return _decode_snapshot(wire, native_output)


def setup(control, options):
    token = uuid.uuid4().hex[:12]
    fixture = {"name": "Gameplay shuttle " + token,
               "actor_name": "Pilot" + token, "blueprint": 4168,
               "scope": "fresh crew and ship in the owned disposable checkpoint copy"}
    try:
        # Scene construction, not measured flight: the normal factory fills
        # tanks and binds the cloned cockpit, and the existing test ownership
        # contract marks exactly this new interior for authenticated disposal.
        value = _evaluate(control, f"""
            baseline = $blueprint:fixture_baseline();
            ship = #4168:create("{fixture['name']}", $nothing);
            $blueprint:fixture_capture(ship, baseline);
            ship.position = {{1.0e12, 1.0e12, 1.0e12}};
            $space_objects.universe:add(ship);
            console = $nothing;
            for c in (ship.consoles)
              if (isa(c, $console_feature) &&
                  ((c.available_commands == "all") ||
                   ((typeof(c.available_commands) == LIST) &&
                    ("heading" in c.available_commands) && ("setspeed" in c.available_commands))))
                console = c; break;
              endif
            endfor
            if (!valid(console)) raise(E_INVARG); endif
            return {{ship, console, console.location, ship.b_cleanup_nonce}};
        """, timeout=options.deadline_seconds)
        require(isinstance(value, list) and len(value) == 4,
                "blueprint did not return a bound helm fixture", value)
        fixture.update(zip(("ship", "console", "room", "cleanup_nonce"), value))
        actor = _evaluate(control, f'return $account_manager:create_player("{fixture["actor_name"]}");')
        fixture["actor"] = actor
        conn = connect_actor(control, actor, options)
        _connections[int(actor)] = conn
        options.flight_connection = conn
        actor_ref, ship_ref, console_ref = (object_ref(fixture[k]) for k in ("actor", "ship", "console"))
        placement = _evaluate(control, f"""
            a = {actor_ref}; s = {ship_ref}; c = {console_ref};
            if ((!is_player(a)) || a.wizard) raise(E_PERM); endif
            core = $computer_core:bootstrap(s);
            core:grant(player, a, "operate"); core:grant(player, a, "query");
            move(a, c.location); a.coords = c.coords;
            return {{a.wizard, is_player(a), a.location, core:allows(a, "operate")}};
        """)
        require(placement == [0, 1, fixture["room"], 1],
                "fresh crew placement or explicit ACL grant failed", placement)
        fixture["setup_commands"] = [actor_command(conn, "man " + console_ref,
                                                     timeout=options.deadline_seconds)]
        state = snapshot(control, fixture)
        _require_station(state, fixture)
        fixture["setup_commands"].append(actor_command(conn, "setoutput all 100%"))
        charged = wait_until(lambda: snapshot(control, fixture),
                             lambda s: s["reactors"] and all(r[2] > 0 and r[2] == r[3] for r in s["reactors"]),
                             options.deadline_seconds, description="natural reactor charge")
        state = charged["state"]
        require(state["engines"] and state["thrusters"] and _fuel(state) > 0,
                "blueprint has no fueled propulsion", state)
        if not state["computer_online"]:
            fixture["setup_commands"].append(actor_command(conn, "boot"))
        wait_until(lambda: snapshot(control, fixture),
                   lambda s: s["computer_online"], options.deadline_seconds,
                   description="ordinary computer boot")
        # Named system groups use the same ordinary manual power-bus path.
        # #4129:find_systems_by_name resolves these registered type names;
        # the /0 suffix selects every instance rather than just the first.
        for name in ("engine/0", "thrusters/0"):
            fixture["setup_commands"].append(actor_command(conn, "allocate " + name + " 100%"))
        ready = wait_until(lambda: snapshot(control, fixture),
                           lambda s: s["computer_online"] and s["online"] and
                           all(r[3] > 0 and r[5] > 0 for r in s["engines"] + s["thrusters"]),
                           options.deadline_seconds, description="ordinary propulsion commissioning")
        _require_station(ready["state"], fixture)
        fixture["commissioned"] = ready["state"]
        return fixture
    except Exception as primary:
        evidence = {"partial_fixture": fixture,
                    "primary_failure_evidence": getattr(primary, "evidence", None)}
        if all(k in fixture for k in ("ship", "actor", "console")):
            try:
                evidence["cleanup"] = cleanup(control, fixture)
            except Exception as secondary:
                evidence["cleanup_failure"] = str(secondary)
        raise ScenarioFailure("flight setup failed: " + str(primary), evidence) from primary


def run(control, fixture, options):
    require(int(fixture["actor"]) in _connections, "flight actor connection is unavailable")
    conn = _connections[int(fixture["actor"])]
    result = {"completed_iterations": 0, "iterations": [],
              "scope": "ordinary loopback commands plus independently observed natural domain completion"}
    try:
        for index in range(options.iterations):
            before = snapshot(control, fixture)
            _require_station(before, fixture)
            # Mongoose heading 0 points +Y, 90 points +X. Cardinal targets
            # independently avoid #91:deg_to_vec's four-decimal truncation.
            angle = (round(math.degrees(math.atan2(before["heading"][0], before["heading"][1])) / 90) * 90 + 90) % 360
            elevation = 0
            target_heading = {0: [0.0, 1.0, 0.0], 90: [1.0, 0.0, 0.0],
                              180: [0.0, -1.0, 0.0], 270: [-1.0, 0.0, 0.0]}[angle]
            target_speed = float(1 + index % 2)
            row = {"index": index, "status": "incomplete", "before": before, "commands": []}
            result["iterations"].append(row)
            row["commands"].append(actor_command(conn, f"heading {angle} {elevation}"))
            # #4158:set_heading resolves/plays its sound before assigning the
            # computer's target. OUTPUTSUFFIX can arrive at SQL suspension;
            # it is not a completion acknowledgement for this domain request.
            row["heading_request"] = wait_until(
                lambda: snapshot(control, fixture),
                lambda s: _distance(s["desired_heading"], target_heading) < 1e-6,
                options.deadline_seconds, description="typed heading request assignment")
            _require_station(row["heading_request"]["state"], fixture)
            row["commands"].append(actor_command(conn, f"setspeed {target_speed}"))
            ordered = snapshot(control, fixture)
            _require_station(ordered, fixture)
            require(ordered["desired_speed"] == target_speed and
                    _distance(ordered["desired_heading"], target_heading) < 1e-6 and not ordered["engines_cut"],
                    "ordinary navigation request was refused", ordered)
            row["moving"] = wait_until(lambda: snapshot(control, fixture),
                                        lambda s: _motion_completed(before, s, target_speed, target_heading),
                                        options.deadline_seconds, description="flight, turn and engine fuel consumption")
            moving = row["moving"]["state"]
            _require_station(moving, fixture)
            row["commands"].append(actor_command(conn, "setspeed 0"))
            row["stopped"] = wait_until(lambda: snapshot(control, fixture),
                                         lambda s: _brake_completed(moving, s),
                                         options.deadline_seconds, description="real engine braking to zero velocity")
            stopped = row["stopped"]["state"]
            _require_station(stopped, fixture)
            row["commands"].append(actor_command(conn, "cutengines"))
            row["after"] = snapshot(control, fixture)
            _require_station(row["after"], fixture)
            require(row["after"]["engines_cut"] and _length(row["after"]["velocity"]) <= 1e-9,
                    "cutengines did not establish the stopped terminal state", row["after"])
            row["status"] = "completed"
            result["completed_iterations"] += 1
        return result
    except Exception as error:
        result["primary_failure_evidence"] = getattr(error, "evidence", None)
        raise ScenarioFailure("flight iteration failed: " + str(error), result) from error


def cleanup(control, fixture):
    """Only authenticated new ship contents are recycled; never a real player."""
    actor, ship, console = (object_ref(fixture[k]) for k in ("actor", "ship", "console"))
    released = _evaluate(control, f"""
        if (valid({console}) && valid({actor})) {console}:unman({actor}); endif
        if (valid({actor})) move({actor}, $player_start); endif
        return {{valid({actor}), valid({console}) ? ({actor} in {console}.users) | 0}};
    """)
    conn = _connections.pop(int(fixture["actor"]), None)
    if conn:
        conn.close()
    disposed = _evaluate(control, f"""
        s = {ship};
        if (valid(s))
          if (s.b_cleanup_nonce != {json.dumps(fixture['cleanup_nonce'])}) raise(E_PERM); endif
          if (valid(s.universe)) s.universe:remove(s); endif
          $blueprint:recycle_test_ship(s);
        endif
        return {{valid(s), valid({console})}};
    """, timeout=120)
    require(disposed == [0, 0], "owned ship or console survived authenticated disposal", disposed)
    return {"station_release": released, "ship_console_valid": disposed,
            "ordinary_connection_closed": conn is not None,
            "actor_account_lifetime": "fresh owned player/account remain until disposable server shutdown"}
