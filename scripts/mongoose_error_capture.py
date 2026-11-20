"""Bounded, payload-free Mongoose error history observations in owned worlds."""
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import re
import stat
import time
import uuid

EVENT_CAP, FRAME_CAP, VERB_CAP, HEADER_CAP = 15, 32, 256, 1024
BYTE_CAP, REQUEST_TIMEOUT, RETRIES = 262144, 4, 3
FRAME_FIELDS = ("receiver", "verb", "programmer", "definer", "player", "line")
COUNTERS = ("malformed", "omitted", "unsupported_anonymous", "unsupported_receiver", "markers", "unknown_fields", "truncated_headers")
SCHEMA_VERSION = 2
# Stable native ordinal order: structures.h enum error / types/errorcode.go.
# Convert only typed ERR values; neither error messages nor string spellings
# supplied by a history row can become an enum through this table.
ERROR_NAMES = ("E_NONE", "E_TYPE", "E_DIV", "E_PERM", "E_PROPNF", "E_VERBNF", "E_VARNF", "E_INVIND",
               "E_RECMOVE", "E_MAXREC", "E_RANGE", "E_ARGS", "E_NACC", "E_INVARG", "E_QUOTA",
               "E_FLOAT", "E_FILE", "E_EXEC", "E_INTRPT")


def projection_request(actor, filename):
    if not re.fullmatch(r"#[0-9]+", actor) or not re.fullmatch(r"error-[a-f0-9]{32}\.json", filename):
        raise ValueError("invalid owned metadata request reference")
    # Only this fixed projection reaches FileIO. Rows/messages/values/lines and
    # runtime variables remain inside the server and never enter the transcript.
    return r'''
a = ACTOR; h = $wiz_utils.traceback_log; recent = $issues.recent_tracebacks;
error_names = ERROR_ENUMS;
out = ["schema"->2, "actor"->toint(a), "history_total"->0, "history_omitted"->0,
       "history_malformed"->0, "events"->{}, "issue_status"->"missing",
       "event_player"->"unavailable", "issue_time"->"unavailable", "history_matches"->{},
       "correlation"->"unavailable", "issue_unknown_fields"->0, "output_truncated"->0,
       "error_code"->"unavailable"];
il = {}; issueok = 0;
if (typeof(recent) != MAP)
  out["issue_status"] = "malformed";
elseif (maphaskey(recent,a))
  issue = recent[a]; out["issue_status"] = "malformed";
  if (typeof(issue) == MAP)
    for key in (mapkeys(issue)) if (!(key in {"who","time","lines"}))
      out["issue_unknown_fields"] = out["issue_unknown_fields"] + 1; endif endfor
    if (maphaskey(issue,"who") && maphaskey(issue,"time") && maphaskey(issue,"lines") &&
        typeof(issue["who"]) == OBJ && issue["who"] == a &&
        typeof(issue["time"]) == INT && issue["time"] >= 0 && typeof(issue["lines"]) == LIST)
      out["issue_status"] = "available"; out["event_player"] = toint(a);
      out["issue_time"] = issue["time"]; il = issue["lines"];
      issueok = length(il) > 0 && length(il) <= 32;
      for n in [1..min(length(il),32)]
        if (typeof(il[n]) != STR || length(il[n]) > 1024) issueok = 0; endif
      endfor
    endif
  endif
endif
sources = {}; indices = {}; kinds = {}; codes = {}; matched_codes = {};
if (typeof(h) != LIST)
  out["history_malformed"] = 1;
else
  out["history_total"] = length(h); out["history_omitted"] = max(0,length(h)-15);
  for i in [max(1,length(h)-14)..length(h)]
    r = h[i]; kind = "malformed"; source = {}; formatted = {}; code = "unavailable";
    if (typeof(r) == LIST && length(r) == 4 && typeof(r[4]) == LIST)
      kind = "formatted"; source = r[4]; formatted = r[4];
    elseif (typeof(r) == LIST && length(r) == 5 && typeof(r[4]) == LIST && typeof(r[5]) == LIST)
      kind = "structured"; source = r[4]; formatted = r[5];
    else
      out["history_malformed"] = out["history_malformed"] + 1;
    endif
    if (kind != "malformed")
      if (typeof(r[1]) == ERR)
        ordinal = toint(r[1]);
        if (ordinal >= 0 && ordinal < length(error_names)) code = error_names[ordinal+1]; endif
      endif
      candidateok = length(formatted) > 0 && length(formatted) <= 32;
      for n in [1..min(length(formatted),32)]
        if (typeof(formatted[n]) != STR || length(formatted[n]) > 1024) candidateok = 0; endif
      endfor
      if (issueok && candidateok && equal(il,formatted))
        out["history_matches"] = {@out["history_matches"],i};
        matched_codes = {@matched_codes,code};
      endif
    endif
    sources = {@sources,source}; indices = {@indices,i}; kinds = {@kinds,kind}; codes = {@codes,code};
  endfor
endif
if (issueok)
  out["correlation"] = length(out["history_matches"]) == 1 ? "unique_bounded_exact_list_match" |
                       (length(out["history_matches"]) ? "ambiguous" | "no_match");
  if (length(matched_codes) == 1) out["error_code"] = matched_codes[1]; endif
endif
sources = {@sources,il}; indices = {@indices,0}; kinds = {@kinds,"formatted"}; codes = {@codes,"unavailable"};
for q in [1..length(sources)]
  source = sources[q]; frames = {};
  counts = ["malformed"->0,"omitted"->max(0,length(source)-32),"unsupported_anonymous"->0,
            "unsupported_receiver"->0,"markers"->0,"unknown_fields"->0,"truncated_headers"->0];
  for j in [1..min(length(source),32)]
    f = ["receiver"->"unavailable","verb"->"unavailable","programmer"->"unavailable",
         "definer"->"unavailable","player"->"unavailable","line"->"unavailable"];
    if (kinds[q] == "structured")
      fr = source[j];
      if (typeof(fr) != LIST || length(fr) < 6)
        counts["malformed"] = counts["malformed"] + 1;
      else
        counts["unknown_fields"] = counts["unknown_fields"] + max(0,length(fr)-6);
        for k in [1..6]
          field = {"receiver","verb","programmer","definer","player","line"}[k]; v = fr[k];
          if (k == 2 && typeof(v) == STR && length(v) > 0 && length(v) <= 256 &&
              match(v,"^[A-Za-z0-9_ *.-]+$",1)) f[field] = v;
          elseif (k == 6 && typeof(v) == INT && v >= 0) f[field] = v;
          elseif (k != 2 && k != 6 && typeof(v) == OBJ) f[field] = toint(v);
          else counts["malformed"] = counts["malformed"] + 1;
          endif
        endfor
        frames = {@frames,f};
      endif
    else
      text = source[j];
      if (typeof(text) != STR)
        counts["malformed"] = counts["malformed"] + 1; continue;
      endif
      if (length(text) > 1024) counts["truncated_headers"] = counts["truncated_headers"] + 1; endif
      text = text[1..min(length(text),1024)];
      if (index(text,"... called from ",1) == 1) text = text[17..$]; endif
      if (text == "(End of traceback)" || index(text,"built-in function ",1) == 1 ||
          index(text,"(no stack)",1) == 1)
        counts["markers"] = counts["markers"] + 1; continue;
      endif
      if (index(text,"*anonymous*:",1) == 1)
        counts["unsupported_anonymous"] = counts["unsupported_anonymous"] + 1; continue;
      endif
      split = index(text,", line ",1); colon = index(text,":",1);
      if (!split || colon < 3 || colon >= split || text[1] != "#")
        counts["malformed"] = counts["malformed"] + 1; continue;
      endif
      number = text[2..colon-1]; vn = text[colon+1..split-1]; tail = text[split+7..$];
      digits = 0;
      while (digits < length(tail) && index("0123456789",tail[digits+1],1)) digits = digits + 1; endwhile
      if (length(number) > 18 || !match(number,"^-?[0-9]+$",1) || !digits || digits > 18 ||
          (digits < length(tail) && !(j == 1 && tail[digits+1] == ":")))
        counts["malformed"] = counts["malformed"] + 1; continue;
      endif
      receiver = index(vn," (this == ",1);
      if (receiver)
        rendered = vn[receiver+10..$]; vn = vn[1..receiver-1];
        if (length(rendered) >= 3 && length(rendered) <= 20 && rendered[1] == "#" && rendered[$] == ")" &&
            match(rendered[2..$-1],"^-?[0-9]+$",1))
          f["receiver"] = toint(rendered[2..$-1]);
        else counts["unsupported_receiver"] = counts["unsupported_receiver"] + 1;
        endif
      endif
      if (!length(vn) || length(vn) > 256 || !match(vn,"^[A-Za-z0-9_ *.-]+$",1))
        counts["malformed"] = counts["malformed"] + 1; continue;
      endif
      f["definer"] = toint(number); f["verb"] = vn; f["line"] = toint(tail[1..digits]);
      frames = {@frames,f};
    endif
  endfor
  event = ["history_index"->indices[q],"representation"->kinds[q],"frames"->frames,"counts"->counts,
           "error_code"->codes[q]];
  if (q == length(sources)) out["issue"] = event;
  else out["events"] = {@out["events"],event}; endif
endfor
payload = generate_json(out);
if (length(payload) > 262144)
  out["output_truncated"] = 1;
  payload = generate_json(["schema"->2,"actor"->toint(a),"output_truncated"->1]);
endif
fd = file_open("FILENAME","w-tn");
try file_writeline(fd,payload); finally file_close(fd); endtry
return {1,length(payload),out["output_truncated"]};
'''.replace("ACTOR", actor).replace("FILENAME", filename).replace(
        "ERROR_ENUMS", "{" + ",".join(json.dumps(name) for name in ERROR_NAMES) + "}")


TOP_FIELDS = {"schema", "actor", "history_total", "history_omitted", "history_malformed", "events", "issue_status",
              "event_player", "issue_time", "history_matches", "correlation", "issue_unknown_fields", "output_truncated", "issue", "error_code"}


def valid_error_code(value):
    return isinstance(value, str) and (value == "unavailable" or value in ERROR_NAMES)


def validate_projection(raw, actor):
    """Fail closed on schema drift; never persist unknown strings or keys."""
    if len(raw) > BYTE_CAP + 1:
        raise ValueError("projection byte cap exceeded")
    row = json.loads(raw)
    if (not isinstance(row, dict) or set(row) - TOP_FIELDS or type(row.get("schema")) is not int or
            row.get("schema") != SCHEMA_VERSION or type(row.get("actor")) is not int or row.get("actor") != actor or
            type(row.get("output_truncated")) is not int):
        raise ValueError("unknown projection schema/fields/actor")
    if row.get("output_truncated") == 1:
        if set(row) != {"schema", "actor", "output_truncated"}:
            raise ValueError("invalid truncated projection")
        return row
    if set(row) != TOP_FIELDS or row.get("output_truncated") != 0:
        raise ValueError("incomplete projection schema")
    if not valid_error_code(row["error_code"]):
        raise ValueError("unknown primary error enum")
    for key in ("history_total", "history_omitted", "history_malformed", "issue_unknown_fields"):
        if type(row[key]) is not int or row[key] < 0:
            raise ValueError("invalid bounded counter")
    if row["issue_status"] not in ("missing", "malformed", "available") or row["correlation"] not in (
            "unavailable", "unique_bounded_exact_list_match", "ambiguous", "no_match"):
        raise ValueError("unknown issue/correlation state")
    for key in ("event_player", "issue_time"):
        if row[key] == "unavailable":
            row[key] = None
        if row[key] is not None and (type(row[key]) is not int or (key == "issue_time" and row[key] < 0)):
            raise ValueError("invalid issue identity/time")
    if row["event_player"] not in (None, actor):
        raise ValueError("actor-specific event identity mismatch")
    if not isinstance(row["events"], list) or len(row["events"]) > EVENT_CAP:
        raise ValueError("event cap exceeded")
    if (not isinstance(row["history_matches"], list) or len(row["history_matches"]) > EVENT_CAP or
            any(type(value) is not int or value <= 0 or value > row["history_total"] for value in row["history_matches"])):
        raise ValueError("invalid bounded correlation indices")
    for event in row["events"] + [row["issue"]]:
        if (not isinstance(event, dict) or set(event) != {"history_index", "representation", "frames", "counts", "error_code"} or
                type(event["history_index"]) is not int or not 0 <= event["history_index"] <= row["history_total"] or
                event["representation"] not in ("formatted", "structured", "malformed") or
                not valid_error_code(event["error_code"]) or
                not isinstance(event["frames"], list) or len(event["frames"]) > FRAME_CAP or
                not isinstance(event["counts"], dict) or set(event["counts"]) != set(COUNTERS) or
                any(type(value) is not int or value < 0 for value in event["counts"].values())):
            raise ValueError("invalid event/frame/counter schema")
        for frame in event["frames"]:
            if not isinstance(frame, dict) or set(frame) != set(FRAME_FIELDS):
                raise ValueError("unknown frame fields")
            for field, value in frame.items():
                if value == "unavailable":
                    frame[field] = value = None
                if value is None:
                    continue
                if field == "verb":
                    if not isinstance(value, str) or len(value) > VERB_CAP or not re.fullmatch(r"[A-Za-z0-9_ *.-]+", value):
                        raise ValueError("invalid bounded verb metadata")
                elif type(value) is not int or (field == "line" and value < 0):
                    raise ValueError("invalid frame identity/line")
            if event["representation"] == "formatted" and (frame["programmer"] is not None or frame["player"] is not None):
                raise ValueError("formatted frame inferred an unavailable identity")
        if event["representation"] == "malformed" and event["error_code"] != "unavailable":
            raise ValueError("malformed history supplied an error enum")
    events = {event["history_index"]: event for event in row["events"]}
    expected_indices = list(range(max(1, row["history_total"] - EVENT_CAP + 1), row["history_total"] + 1))
    if ([event["history_index"] for event in row["events"]] != expected_indices or
            row["history_omitted"] != max(0, row["history_total"] - EVENT_CAP) or
            row["issue"]["history_index"] != 0 or row["issue"]["representation"] != "formatted" or
            row["issue"]["error_code"] != "unavailable"):
        raise ValueError("inconsistent bounded history/issue identities")
    matches = row["history_matches"]
    if (len(matches) != len(set(matches)) or any(index not in events for index in matches) or
            matches != sorted(matches)):
        raise ValueError("ambiguous or missing correlation indices")
    available = row["issue_status"] == "available"
    if ((available and (row["event_player"] != actor or row["issue_time"] is None)) or
            (not available and (row["event_player"] is not None or row["issue_time"] is not None or
                                row["correlation"] != "unavailable"))):
        raise ValueError("inconsistent actor issue availability")
    match_counts = {"unavailable": len(matches) == 0, "no_match": len(matches) == 0,
                    "unique_bounded_exact_list_match": len(matches) == 1, "ambiguous": len(matches) > 1}
    if not match_counts[row["correlation"]] or (not available and matches):
        raise ValueError("inconsistent whole-list correlation")
    primary = events[matches[0]]["error_code"] if row["correlation"] == "unique_bounded_exact_list_match" else "unavailable"
    if row["error_code"] != primary:
        raise ValueError("primary error enum lacks a unique actor history match")
    return row


def read_owned_projection(path, identity):
    with os.fdopen(os.open(path, os.O_RDONLY | os.O_NOFOLLOW), "rb") as stream:
        before = os.fstat(stream.fileno())
        if not stat.S_ISREG(before.st_mode) or (before.st_dev, before.st_ino) != identity:
            raise ValueError("owned metadata file identity changed")
        raw = stream.read(BYTE_CAP + 2)
        after = os.fstat(stream.fileno())
        current = path.lstat()
        if (not stat.S_ISREG(current.st_mode) or (current.st_dev, current.st_ino) != identity or
                (before.st_size, before.st_mtime_ns, before.st_ctime_ns) !=
                (after.st_size, after.st_mtime_ns, after.st_ctime_ns) or current != after):
            raise ValueError("owned metadata file changed while reading")
        return raw


def remove_owned_projection(path, identity):
    try:
        current = path.lstat()
    except FileNotFoundError:
        return
    if not stat.S_ISREG(current.st_mode) or (current.st_dev, current.st_ino) != identity:
        raise ValueError("refusing to remove replaced metadata evidence")
    path.unlink()


def capture(control, server, actor, directory, phase, *, baseline=None):
    """Capture failures are diagnostic results and never replace gameplay errors."""
    result = {"status": "unavailable", "phase": phase, "attempts": [], "error_code": "unavailable",
              "scope": "actor latest issue context and bounded global history; command causality/task ID unknown"}
    incoming = None
    try:
        actor_ref = str(actor) if isinstance(actor, str) else (f"#{int(actor)}" if isinstance(actor, int) and not isinstance(actor, bool) else "")
        if not re.fullmatch(r"#[0-9]+", actor_ref) or phase not in ("baseline", "failure_before_cleanup"):
            raise ValueError("owned actor/phase unavailable")
        result["actor"] = actor_id = int(actor_ref[1:])
        if type(server.proc.pid) is not int or type(server.start_ticks) is not int:
            raise ValueError("owned process birth identity unavailable")
        result.update(pid=server.proc.pid, start_ticks=server.start_ticks)
        previous = (baseline or {}).get("projection", {})
        for attempt in range(RETRIES if phase == "failure_before_cleanup" else 1):
            server.verify_owned()
            filename = "error-" + uuid.uuid4().hex + ".json"
            candidate = directory.parent / "files" / filename
            reservation = os.open(candidate, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            try:
                reserved = os.fstat(reservation)
                incoming_identity = (reserved.st_dev, reserved.st_ino)
                incoming = candidate  # Only a successful exclusive reservation is ours.
            finally:
                os.close(reservation)
            code = projection_request(actor_ref, filename)
            timing = {"started_monotonic_ns": time.monotonic_ns(), "request_sha256": hashlib.sha256(code.encode()).hexdigest()}
            try:
                ack, _ = control.eval_metadata(code, timeout=REQUEST_TIMEOUT)
                if not isinstance(ack, list) or len(ack) != 3 or any(type(item) is not int for item in ack) or ack[0] != 1:
                    raise ValueError("invalid compact metadata acknowledgement")
                server.verify_owned()
                raw = read_owned_projection(incoming, incoming_identity)
                row = validate_projection(raw, actor_id)
                if ack[1] != len(raw.rstrip(b"\n")) or ack[2] != row["output_truncated"]:
                    raise ValueError("metadata acknowledgement disagrees with owned file")
            finally:
                timing["finished_monotonic_ns"] = time.monotonic_ns()
                timing["latency_ns"] = timing["finished_monotonic_ns"] - timing["started_monotonic_ns"]
                result["attempts"].append(timing)
                remove_owned_projection(incoming, incoming_identity)
                incoming = None
            result.update(status="collected", projection=row)
            result["comparison"] = {"baseline_available": bool(previous),
                                    "issue_time_changed": row.get("issue_time") != previous.get("issue_time"),
                                    "new_event_count": None, "command_exclusive_causality": "unknown"}
            if (phase == "baseline" or row.get("output_truncated") or
                    (row.get("issue_status") == "available" and row.get("issue_time") != previous.get("issue_time") and
                     row.get("correlation") in ("unique_bounded_exact_list_match", "ambiguous"))):
                break
            if attempt + 1 < RETRIES:
                time.sleep(0.1)
        result["error_code"] = row.get("error_code", "unavailable")
        path = directory / ("error-metadata-" + phase + ".json")
        with path.open("x", encoding="utf-8") as stream:
            json.dump(result, stream, sort_keys=True)
            stream.write("\n")
        result["artifact"] = str(path)
        result["artifact_sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
    except Exception as error:
        result.update(status="failed", error_code="unavailable", failure_type=type(error).__name__, reason="bounded metadata capture unavailable")
        # Never stringify a native exception: it may contain messages/values.
    finally:
        if incoming is not None:
            try:
                remove_owned_projection(incoming, incoming_identity)
            except Exception as error:
                result.update(status="failed", cleanup_failure_type=type(error).__name__)
    return result
