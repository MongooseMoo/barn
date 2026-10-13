# Toast source research: WAIF, primitive dispatch, perms, resource limits

Source tree: `/root/src/toaststunt/src/` (WSL). All line numbers from that tree.
Facts only, no design proposals.

---

## Q5 — Type tag numbering (`include/structures.h:99-119`)

```c
typedef enum {
    TYPE_INT, TYPE_OBJ, _TYPE_STR, TYPE_ERR, _TYPE_LIST, /* user-visible */
    TYPE_CLEAR,     /* in clear properties' value slot */
    TYPE_NONE,      /* in uninitialized MOO variables */
    TYPE_CATCH,     /* on-stack marker for an exception handler */
    TYPE_FINALLY,   /* on-stack marker for a TRY-FINALLY clause */
    _TYPE_FLOAT,    /* floating-point number; user-visible */
    _TYPE_MAP,      /* map; user-visible */
    _TYPE_ITER,     /* map iterator; not visible */
    _TYPE_ANON,     /* anonymous object; user-visible */
    _TYPE_WAIF,     /* lightweight object; user-visible */
    TYPE_BOOL,      /* Experimental boolean type */
    /* THE END - complex aliases come next */
    TYPE_STR   = (_TYPE_STR   | TYPE_COMPLEX_FLAG),
    TYPE_FLOAT = (_TYPE_FLOAT),
    TYPE_LIST  = (_TYPE_LIST  | TYPE_COMPLEX_FLAG),
    TYPE_MAP   = (_TYPE_MAP   | TYPE_COMPLEX_FLAG),
    TYPE_ITER  = (_TYPE_ITER  | TYPE_COMPLEX_FLAG),
    TYPE_ANON  = (_TYPE_ANON  | TYPE_COMPLEX_FLAG),
    TYPE_WAIF  = (_TYPE_WAIF  | TYPE_COMPLEX_FLAG),
} var_type;
```

Numeric values: INT 0, OBJ 1, STR 2, ERR 3, LIST 4, CLEAR 5, NONE 6, CATCH 7,
FINALLY 8, FLOAT 9, MAP 10, ITER 11, ANON 12, WAIF 13, BOOL 14.
So `typeof(waif) == 13`. Note BOOL is the one type with **no** complex alias — it
is a plain value (`v.v.truth`), so `TYPE_BOOL == 14` is both the DB value and the
in-memory value.

`structures.h:84-85`:
```c
#define TYPE_COMPLEX_FLAG   0x80
#define TYPE_DB_MASK        0x7f
```
Comment at `structures.h:76-83`: the complex flag "is only set in memory. The
original `_TYPE_XYZ` values are used in the database file and returned to verbs
calling typeof()."

Extension history readable from the enum order: everything through `TYPE_FINALLY`
is LambdaMOO; `_TYPE_FLOAT` was appended by LambdaMOO 1.8; `_TYPE_MAP`/`_TYPE_ITER`
(Stunt), `_TYPE_ANON` (Stunt), `_TYPE_WAIF` (Jackson's waif patch, folded into
Toast), `TYPE_BOOL` (Toast, still marked "Experimental"). The scheme is strictly
append-only. `eval_env.cc:95-118` gates the built-in constants on DB version:
```c
if (version >= DBV_Anon) { v.v.num = (int) _TYPE_ANON; env[SLOT_ANON] = var_ref(v); }
if (version >= DBV_Waif) { v.v.num = (int) _TYPE_WAIF; env[SLOT_WAIF] = var_ref(v); }
if (version >= DBV_Bool) { v.type = TYPE_INT; v.v.num = (int) TYPE_BOOL; env[SLOT_BOOL] = ... }
```
so `WAIF`/`BOOL` as MOO identifiers only exist for DBs new enough.

The header itself enumerates the maintenance burden of a new type
(`structures.h:88-98`):

> Don't forget to add an English name for new types to unparse.cc. ... When you
> add a new type, if applicable, add it to the PUSH_TYPE_MISMATCH call in the
> following places (terrible, I know): execute.cc: OP_MAP_INSERT, OP_PUSH_REF,
> OP_RANGE_REF, EOP_RANGESET

There is also a live example of renumbering pain: `server.cc:1934`
`int waif_conversion_type = _TYPE_WAIF;  /* For shame. We can remove this someday. */`
with `db_io.cc:158`
```c
if (l == waif_conversion_type && waif_conversion_type != _TYPE_WAIF)
    return read_waif();
```
and `server.cc:2216-2217` logging `"(Using type '%i' for waifs; will convert to
'%i' at next checkpoint)"`. That machinery exists because waifs were once written
under a different tag number and DBs had to be migrated.

---

## Q1 — WAIF as a non-OBJ value receiving verb calls

### Constants (`include/waif.h:25-26, 32-35`)
```c
#define WAIF_PROP_PREFIX    ':'
#define WAIF_VERB_PREFIX    ':'
#ifdef WAIF_DICT
#define WAIF_INDEX_VERB ":_index"
#define WAIF_INDEXSET_VERB ":_set_index"
#endif
```
A waif's verbs and properties live on its **class object**, named with a leading
`:`. So `mywaif:foo()` runs `#class::foo` and `mywaif.bar` reads `#class.:bar`.
The prefix keeps waif members in a namespace that ordinary object verbs/props
cannot collide with (and that ordinary `obj:foo()` cannot reach, since a leading
colon is not parseable as a verb name in normal code).

### Verb dispatch (`execute.cc:2115-2195`, `OP_CALL_VERB`)
One opcode handles OBJ, WAIF and primitives. It pops `args`, `verb`, `obj`
(`obj` = "could be anything", comment at 2123).

WAIF branch, `execute.cc:2127-2137`:
```c
else if (obj.type == TYPE_WAIF) {
    char *str = (char *)mymalloc(strlen(verb.v.str) + 2, M_STRING);
    _class = obj.v.waif->_class;
    str[0] = WAIF_VERB_PREFIX;
    strcpy(str + 1, verb.v.str);
    free_str(verb.v.str);
    verb.v.str = str;
    STORE_STATE_VARIABLES();
    err = call_verb2(_class, verb.v.str, obj, args, 0, DEFAULT_THREAD_MODE);
    LOAD_STATE_VARIABLES();
}
```
Note the two distinct roles: the **receiver objid** for verb lookup is
`obj.v.waif->_class`, while `_this` passed to `call_verb2` is the waif Var itself.
Inside the verb, `this` is the waif; `verb` is the *unprefixed* name, because
`call_verb2` strips it back off (`execute.cc:773-777`):
```c
v.type = TYPE_STR;
if (vname[0] == WAIF_VERB_PREFIX)
    v.v.str = str_dup(vname + 1);
else
    v.v.str = str_ref(vname);
set_rt_env_var(env, SLOT_VERB, v);
```

`call_verb2` (`execute.cc:663-782`) itself is type-agnostic apart from the ANON
special case (`execute.cc:719-724`):
```c
if (TYPE_ANON == _this.type && is_valid(_this))
    h = db_find_callable_verb(_this, vname);
else if (valid(recv))
    h = db_find_callable_verb(Var::new_obj(recv), vname);
else
    return E_INVIND;
```
It sets `RUN_ACTIV._this = var_ref(_this)` (line 730) and
`RUN_ACTIV.recv = recv` (line 735) separately — that split is what makes a
non-OBJ receiver possible at all.

`do_server_program_task` (`execute.cc:3299-3307`) recomputes the same mapping for
server-initiated tasks:
```c
switch (_this.type) {
    case TYPE_OBJ:  receiver = _this.v.obj; break;
    case TYPE_WAIF: receiver = _this.v.waif->_class; break;
    default:        receiver = NOTHING; break;
}
```
and `tasks.cc:1858` does it once more for `do_server_verb_task` entry:
```c
h = db_find_callable_verb(what.type == TYPE_WAIF ? Var::new_obj(what.v.waif->_class) : what, verb);
```

### Property read/write (`execute.cc:1861-1972`, `waif.cc:651-827`)
Three interpreter sites hand off to waif.cc, each passing `RUN_ACTIV.progr`:
- `execute.cc:1861-1864` (OP_GET_PROP), `1919-1922` (OP_PUSH_GET_PROP):
  `err = waif_get_prop(obj.v.waif, propname.v.str, &prop, RUN_ACTIV.progr);`
- `execute.cc:1966-1969` (OP_PUT_PROP):
  `err = waif_put_prop(obj.v.waif, propname.v.str, rhs, RUN_ACTIV.progr);`

`waif_get_prop` (`waif.cc:665-732`) synthesizes four pseudo-properties before
touching the class:
```c
if (!strcasecmp(name, "owner"))   { prop->type = TYPE_OBJ; prop->v.obj = w->owner;  return E_NONE; }
else if (!strcasecmp(name, "class")) { prop->type = TYPE_OBJ; prop->v.obj = w->_class; return E_NONE; }
else if (!strcasecmp(name, "wizard"))     { *prop = zero; return E_NONE; }
else if (!strcasecmp(name, "programmer")) { *prop = zero; return E_NONE; }
else if (!valid(w->_class)) return E_INVIND;
```
Then it builds the prefixed name and looks it up on the class
(`waif.cc:691-698`):
```c
stream_add_char(s, WAIF_PROP_PREFIX);
stream_add_string(s, name);
name = reset_stream(s);
```
Storage is a flat `w->propvals[]` array indexed by an offset derived from the
class's propdefs (`find_propval_offset`, `waif.cc:704`); a `-2` result is
`E_PROPNF` and `-1` means clear. Permission check `waif.cc:727`:
`else if (!waif_property_allows(w, h, progr, PF_READ)) return E_PERM;`

`waif_property_allows` (`waif.cc:651-658`):
```c
int flags = db_property_flags(h);
return (flags & flag) ||
       ((flags & PF_CHOWN) ? w->owner : db_property_owner(h)) == progr ||
       is_wizard(progr);
```
So a `c` (chown) property on the class is owned, for permission purposes, by the
**waif's** owner rather than the property definition's owner — that is the whole
mechanism by which per-waif ownership works.

`waif_put_prop` (`waif.cc:736-827`) makes `owner`, `class`, `wizard`,
`programmer` all `E_PERM` (lines 746-758), checks `PF_WRITE` the same way
(line 799), then blocks cycles (`waif.cc:805-812`):
```c
me.type = TYPE_WAIF; me.v.waif = w;
if (refers_to(val, me, true)) return E_RECMOVE;
```

`update_waif_propdefs(w)` is called at the top of both (lines 672, 744) — that is
how a class gaining/losing properties propagates lazily to existing waifs
(the comment at `waif.cc:747-753` notes `.class` reassignment *would* work like
chparent via this mechanism, but is disallowed anyway).

### Indexing (`WAIF_DICT`, optional)
`mywaif[k]` and `mywaif[k] = v` dispatch to `:_index` / `:_set_index` verbs on the
class: `execute.cc:1184-1215` (indexset), `1594-1623` and `1683-1727` (index),
each via `call_verb2(_class, waif_index_verb, list, args, 0, ...)`. The verb-name
strings are interned once at `execute.cc:3792-3795`. There is also a mutation
carve-out at `execute.cc:1683-1684`:
```c
/* Skip duplication for waifs since they handle their own mutation via _set_index */
if (NEXT_TOP_RT_VALUE.type != TYPE_WAIF && var_refcount(NEXT_TOP_RT_VALUE) > 1) {
```

### Everything adding TYPE_WAIF had to touch
Complete list of `TYPE_WAIF` / `_TYPE_WAIF` sites outside waif.cc and execute.cc:

| Concern | Site |
|---|---|
| DB read | `db_io.cc:209` `case _TYPE_WAIF: r = read_waif();` |
| DB write | `db_io.cc:391` `case TYPE_WAIF: write_waif(v);` |
| DB tag migration | `db_io.cc:158`, `server.cc:1934`, `server.cc:2216-2217` |
| `typeof` constant | `eval_env.cc:107` (gated on `DBV_Waif`) |
| JSON | `json.cc:406` |
| `tostr` | `list.cc:404` → `"[[waif]]"` |
| `toliteral` | `list.cc:511` → `stream_printf(s, "[[class = #%d, owner = #%d]]", v.v.waif->_class, v.v.waif->owner)` |
| `sort()` exclusion | `list.cc:971` — waifs (like lists/maps/anons) are not sortable |
| `toint`/`tonum` | `numbers.cc:152` → `E_TYPE` |
| `tofloat` | `numbers.cc:183` → `E_TYPE` |
| `isa()`/`is_a` | `objects.cc:1066` — substitutes `Var::new_obj(object.v.waif->_class)` |
| GC / destroyed set | `server.cc:768-777`, `destroyed_waifs` map declared in `waif.h:28` |
| Forked-task activation cleanup | `tasks.cc:616` `if (t->t.forked.a._this.type == TYPE_WAIF) free_var(...)` |
| Server-verb entry | `tasks.cc:1858` |
| English type name | `unparse.cc:213-214` `case TYPE_WAIF: return "waif";` |
| free/ref/dup/refcount/is_true/value_bytes | `utils.cc:178, 238, 282, 312, 340, 373, 626` (the `complex_free_var`/`complex_var_ref`/`complex_var_dup`/`var_refcount`/`is_true`/`value_bytes` switches) |
| `compare()` | `utils.cc:430` |
| `equality()` | `utils.cc:477` |
| `is_object()` predicate | `structures.h:196` `return TYPE_OBJ == type \|\| TYPE_ANON == type \|\| TYPE_WAIF == type;` |
| Var constructor | `structures.h:250` |

Identity semantics are **pointer identity**, not structural
(`utils.cc:428-431, 476-477`):
```c
case TYPE_WAIF: return lhs.v.waif == rhs.v.waif ? 0 : 1;   /* compare()  */
case TYPE_WAIF: return lhs.v.waif == rhs.v.waif;           /* equality() */
```
`compare()` returns only 0 or 1 for waifs — no ordering — which is why
`list.cc:971` excludes them from `sort()`.

**Map keys:** waifs *are* legal map keys. `OP_MAP_INSERT` (`execute.cc:1100-1105`)
rejects only collections:
```c
if (map.type != TYPE_MAP || (key.is_collection() && TYPE_ANON != key.type)) {
    ...
    PUSH_TYPE_MISMATCH(8, key.type, TYPE_STR, TYPE_INT, TYPE_OBJ, TYPE_ERR, TYPE_FLOAT, TYPE_ANON, TYPE_WAIF, TYPE_BOOL);
```
and `is_collection()` (`structures.h:191`) is
`TYPE_LIST == type || TYPE_MAP == type || TYPE_ANON == type` — WAIF is absent, so
it passes. Keying is therefore by waif pointer identity.

---

## Q2 — Primitive-type verb dispatch

Same opcode, the `else` branch (`execute.cc:2138-2180`):

```c
Objid recv = NOTHING;
db_prop_handle h;
Var p;

/* If it's an object, we're good.
 * Otherwise, look for a property on the system
 * object that points us to the prototype/handler
 * for the primitive type.
 */
Var system = Var::new_obj(SYSTEM_OBJECT);

#define MATCH_TYPE(t1, t2)                              \
else if (obj.type == TYPE_##t1) {                       \
    h = db_find_property(system, #t2 "_proto", &p);     \
    if (h.ptr && p.type == TYPE_OBJ && valid(p.v.obj))  \
        recv = p.v.obj;                                 \
}
                    if (obj.type == TYPE_ANON)
                        recv = NOTHING;
                    else if (obj.is_object() && is_valid(obj))
                        recv = obj.v.obj;
                    MATCH_TYPE(INT, int)
                    MATCH_TYPE(OBJ, obj)
                    MATCH_TYPE(FLOAT, float)
                    MATCH_TYPE(STR, str)
                    MATCH_TYPE(ERR, err)
                    MATCH_TYPE(LIST, list)
                    MATCH_TYPE(MAP, map)
#undef MATCH_TYPE

free_var(system);

if (obj.is_object() || recv != NOTHING) {
    STORE_STATE_VARIABLES();
    err = call_verb2(recv, verb.v.str, obj, args, 0, DEFAULT_THREAD_MODE);
    LOAD_STATE_VARIABLES();
}
else
    err = E_TYPE;
```

Points of note:
- The proxy object is found by reading `#0.int_proto`, `#0.str_proto`,
  `#0.float_proto`, `#0.err_proto`, `#0.list_proto`, `#0.map_proto`,
  `#0.obj_proto` — a plain property on `#0`, **not** `$int_proto` resolved through
  the usual `$` mechanism at compile time. `obj_proto` covers verb calls on
  *invalid* object numbers (a valid OBJ short-circuits on the
  `obj.is_object() && is_valid(obj)` branch above).
- No name mangling here — unlike waifs, the verb name is passed through as-is.
  So a proto object's verbs have ordinary names.
- `_this` is again the original primitive value, so inside `$str_proto:length()`,
  `this` is the string.
- ANON is explicitly excluded from proto lookup (`recv = NOTHING`) and handled by
  `call_verb2`'s own ANON branch instead.
- Types with no proto entry at all: BOOL, WAIF (handled above), NONE/CLEAR etc.
  Missing or invalid proto → `recv` stays `NOTHING` → `E_TYPE`.
- Historical note in `tasks.cc:1274-1277` comment: "It cost me long hours of
  debugging to track this down when adding support for verb calls on primitive
  types" — the forked-task path had to be adjusted for non-OBJ `_this`.

---

## Q3 — Permissions

### Frame perms are the verb's owner
`call_verb2`, `execute.cc:730-741`:
```c
RUN_ACTIV.prog     = program_ref(program);
RUN_ACTIV._this    = var_ref(_this);
RUN_ACTIV.progr    = db_verb_owner(h);     /* <-- line 734 */
RUN_ACTIV.recv     = recv;
RUN_ACTIV.vloc     = var_ref(db_verb_definer(h));
RUN_ACTIV.verb     = str_ref(vname);
RUN_ACTIV.verbname = str_ref(db_verb_names(h));
RUN_ACTIV.debug    = (db_verb_flags(h) & VF_DEBUG);
```
Unconditional — no inheritance from the caller. This is the same for both the
waif and primitive dispatch paths above, since both go through `call_verb2`.

All five `progr` assignments in execute.cc:
| Line | Context | Value |
|---|---|---|
| 734 | `call_verb2` (all `obj:verb()` calls, and `pass()`) | `db_verb_owner(h)` |
| 3316 | `do_server_program_task` | `progr` argument; from `do_server_verb_task` (3285) that is `db_verb_owner(h)` |
| 3351 | task resumption from a verb handle | `db_verb_owner(vh)` |
| 3413 | `Input to EVAL` frame setup | `CALLER_ACTIV.progr` — eval inherits |
| 3705 | `bf_set_task_perms` | the requested objid |

`RUN_ACTIV`/`CALLER_ACTIV` are just stack indices (`execute.cc:102-103`):
```c
#define RUN_ACTIV     activ_stack[top_activ_stack]
#define CALLER_ACTIV  activ_stack[top_activ_stack - 1]
```

Note `pass()` also routes through `call_verb2` with `do_pass=1`
(`execute.cc:3681`), so it too re-derives perms from the parent verb's owner —
it does not carry the current frame's perms.

### set_task_perms (`execute.cc:3694-3706`)
```c
bf_set_task_perms(Var arglist, Byte next, void *vdata, Objid progr)
{   /* (player) */
    /* warning!!  modifies top activation */
    Objid oid = arglist.v.list[1].v.obj;
    free_var(arglist);
    if (progr != oid && !is_wizard(progr))
        return make_error_pack(E_PERM);
    RUN_ACTIV.progr = oid;
    return no_var_pack();
}
```
Only a wizard can set perms to someone else; anyone can set perms to themselves
(a no-op unless perms were previously lowered). Builtins receive `progr` from the
calling frame: `execute.cc:2238` `p = call_bi_func(func_id, args, 1, RUN_ACTIV.progr, nullptr);`
and on resumption `execute.cc:331, 383` `call_bi_func(bi_func_id, value, bi_func_pc, a->progr, ...)`.

### task_perms / caller_perms (`execute.cc:3709-3730`)
```c
bf_task_perms(...) { r.type = TYPE_OBJ; r.v.obj = RUN_ACTIV.progr; ... }

bf_caller_perms(...)
{
    r.type = TYPE_OBJ;
    if (top_activ_stack == 0)
        r.v.obj = NOTHING;
    else
        r.v.obj = activ_stack[top_activ_stack - 1].progr;
    ...
}
```
`caller_perms()` reads the *caller frame's current* `progr`, i.e. it observes any
`set_task_perms` the caller performed before calling down. At the bottom of the
stack it is `#-1`, which is the idiomatic "called from the server, not from MOO
code" signal.

### Does task perms outlive a verb-call frame change?
No — `set_task_perms` writes `RUN_ACTIV.progr`, a per-activation field, and every
`obj:verb()` pushes a fresh activation whose `progr` is overwritten with
`db_verb_owner(h)` at line 734. But three qualifications:

1. **Within a frame it is sticky.** After a nested call returns, the frame's
   modified `progr` is still in place; it lasts until the frame pops.
2. **Suspension preserves it.** Perms live in the saved activation stack, so
   `suspend()`/`resume()` and `read()` round-trip the modified `progr`.
3. **Forking captures it.** `OP_FORK` (`execute.cc:2107`) passes the whole current
   activation: `e = enqueue_forked_task2(RUN_ACTIV, f_index, when, ...)`, and the
   forked task's owner for quota purposes is `a.progr` (`tasks.cc:1265`). So a
   `set_task_perms` before a `fork` does propagate into the forked task.
4. **`eval()` inherits** (`execute.cc:3413` `RUN_ACTIV.progr = CALLER_ACTIV.progr;`)
   rather than resetting — the one MOO-visible frame push that does not re-derive
   perms from a verb owner.

---

## Q4 — Server resource controls

### Per-task execution limits (`execute.cc:3054-3082`, `include/options.h:125-131`)
```c
#define DEFAULT_MAX_STACK_DEPTH  50
#define DEFAULT_FG_TICKS         60000
#define DEFAULT_BG_TICKS         30000
#define DEFAULT_FG_SECONDS       5
#define DEFAULT_BG_SECONDS       3
#define DEFAULT_LAG_THRESHOLD    5.0
```
```c
static Timer_ID
setup_task_execution_limits(int seconds, int ticks)
{
    task_alarm_id = set_virtual_timer(seconds < 1 ? 1 : seconds, task_timeout, nullptr);
    task_timed_out = 0;
    ticks_remaining = (ticks < 100 ? 100 : ticks);
    return task_alarm_id;
}
```
Called once per `run_interpreter` (`execute.cc:3076-3082`), choosing fg/bg via
`server_int_option("fg_seconds", DEFAULT_FG_SECONDS)` etc. Note the hard floors:
seconds is clamped up to 1 and ticks up to 100, so a DB cannot starve a task
completely. Tick decrement and timeout check are at `execute.cc:978-983`
(and `2288` for the builtin-call path). Stack depth
(`execute.cc:3211-3218`) has the same floor behavior:
```c
static int current_max_stack_size(void) {
    int max = server_int_option("max_stack_depth", DEFAULT_MAX_STACK_DEPTH);
    if (max < DEFAULT_MAX_STACK_DEPTH) max = DEFAULT_MAX_STACK_DEPTH;
    return max;
}
```
i.e. `max_stack_depth` can only *raise* the limit, never lower it. Exceeding it is
`E_MAXREC` from `push_activation()` (`execute.cc:735-736`).

`ticks_left()`/`seconds_left()` expose the counters (`execute.cc:3673`), and
`suspend_if_needed`-style logic in `bf_suspend`'s neighbor at
`execute.cc:3565-3576` validates that a caller's requested `min_ticks`/
`min_seconds` are strictly below the configured fg limits.

### Per-player queued task quota (`tasks.cc:1234-1254`)
```c
int
check_user_task_limit(Objid user)
{
    tqueue *tq = find_tqueue(user, 0);
    int limit = -1;
    Var v;

    if (valid(user)
            && db_find_property(Var::new_obj(user), "queued_task_limit", &v).ptr
            && v.type == TYPE_INT)
        limit = v.v.num;

    if (limit < 0)
        limit = server_int_option("queued_task_limit", -1);

    if (limit < 0)
        return 1;
    else if ((tq ? tq->num_bg_tasks : 0) >= limit)
        return 0;
    else
        return 1;
}
```
This is the closest thing Toast has to a per-principal quota: a per-player
property overrides a server-wide `$server_options.queued_task_limit`, default
unlimited. Enforced on fork (`tasks.cc:1264-1265`, returns `E_QUOTA`) and on
suspend (`tasks.cc:1310-1320`, `E_QUOTA`). The counter is `tq->num_bg_tasks` on
the player's task queue — so it counts *background* tasks only, and the queue is
keyed by `a.progr` / `progr_of_cur_verb(the_vm)`, i.e. by **task perms**, not by
the connected player.

### Output queue and flushing (`network.cc:1337-1380`, `options.h:245-257`)
```c
#define MAX_QUEUED_OUTPUT         65536
#define MAX_QUEUED_INPUT          MAX_QUEUED_OUTPUT
#define MAX_LINE_BYTES            5242880
#define DEFAULT_CONNECT_TIMEOUT   300
#define MIN_MAX_QUEUED_OUTPUT     2048
```
`enqueue_output` (`network.cc:1347-1372`):
```c
if (h->output_length != 0 && h->output_length + length > server_flag_option_cached(SVO_MAX_QUEUED_OUTPUT)) {   /* must flush... */
    ...
    to_flush = h->output_length + length - server_flag_option_cached(SVO_MAX_QUEUED_OUTPUT);
    if (to_flush > 0 && !flush_ok)
        ...
    while (to_flush > 0 && (b = next)) {
        ...
        to_flush -= b->length;
        h->output_lines_flushed++;
    }
```
Overflow drops the *oldest* buffered lines and counts them; the count is later
reported to the connection (`network.cc:274-291`, the "N lines have been lost"
message) and can trigger `push_network_buffer_overflow` (`network.cc:374`).
`notify()` with `no-flush` set (`flush_ok == 0`) instead fails rather than
dropping. `MAX_LINE_BYTES` (5 MB) closes a connection outright:
`network.cc:476-478` `"Connection `%s` closed for exceeding MAX_LINE_BYTES!"`.

### Input queue backpressure (`tasks.cc:226-227, 1124-1141`)
```c
#define INPUT_HIWAT MAX_QUEUED_INPUT
#define INPUT_LOWAT (INPUT_HIWAT / 2)
```
```c
if (!tq->input_suspended
        && tq->connected
        && tq->total_input_length > INPUT_HIWAT) {
    server_suspend_input(tq->player);
    tq->input_suspended = 1;
}
```
Resumption at `tasks.cc:576-580` when the queue drains below `INPUT_LOWAT`. There
is no per-player override for this one — it is a compile-time constant.
`flush_input` (`tasks.cc:1144-1168`) discards pending input, triggered by the
connection's `flush_cmd` (`tasks.cc:1176`) or the `flush_input()` builtin
(`tasks.cc:2900-2912`). Input suspension state is visible in `queued_tasks()`
output as `input_suspended` (`tasks.cc:2261, 2283`).

### Value-size limits (`include/server.h:197-243`, `options.h:367-380`)
Cached `$server_options` via the `SERVER_OPTIONS_CACHED_MISC` X-macro:

| SVO | `$server_options` property | Default | Clamp |
|---|---|---|---|
| `SVO_MAX_LIST_VALUE_BYTES` | `max_list_value_bytes` | 64537861 | floor `MIN_LIST_VALUE_BYTES_LIMIT` 1021 |
| `SVO_MAX_MAP_VALUE_BYTES` | `max_list_value_bytes` (sic — same property name in the table) | 64537861 | floor 1021 |
| `SVO_MAX_STRING_CONCAT` | `max_string_concat` | 64537861 | floor 1021, ceiling `MAX_STRING`; also sets `stream_alloc_maximum` |
| `SVO_MAX_CONCAT_CATCHABLE` | `max_concat_catchable` | flag, 0 | — |
| `SVO_INCLUDE_RT_VARS` | `include_rt_vars` | flag, 0 | — |
| `SVO_MAX_QUEUED_OUTPUT` | `max_queued_output` | 65536 | floor 2048 |

Enforced e.g. at `execute.cc:1107-1111` — a map insert that exceeds
`SVO_MAX_MAP_VALUE_BYTES` raises `E_QUOTA`.

### Full set of `$server_options` names read from C++
Grepped from all `server_int_option(...)` / `server_flag_option(...)` call sites:

`bg_seconds`, `bg_ticks`, `dump_interval`, `fg_seconds`, `fg_ticks`,
`file_io_max_files`, `finished_tasks_limit`, `json_max_parse_depth`,
`max_background_threads`, `max_stack_depth`, `no_name_lookup`,
`outbound_connect_timeout`, `pcre_match_max_iterations`, `player_huh`,
`queued_task_limit`, `sqlite_max_handles`, `support_numeric_verbname_strings`,
plus the `protect_<property>` / `protect_<builtin>` family
(`server.h:249-251`, `execute.cc:786-788`):
```c
#define bi_prop_protected(prop, progr) ((!is_wizard(progr)) && server_flag_option_cached(prop))
```
and the six cached ones in the table above.

So the resource controls with a genuinely *per-principal* dimension are exactly
two: `player.queued_task_limit` (per-owner background task count) and
`player.ownership_quota`. Everything else is global or per-connection.

### Object ownership quota (`quota.cc:27-72`)
```c
static const char *quota_name = "ownership_quota";
```
`decr_quota(Objid player)` reads `player.ownership_quota`; returns 1 (allow) if
the player is invalid, the property is missing, or it is not an INT; returns 0 if
`v.v.num <= 0`; otherwise decrements and stores. `incr_quota` is the inverse with
no floor. The whole file is `#ifdef OWNERSHIP_QUOTA` — when that is off, both are
no-ops that always allow. Callers: `objects.cc:383` and `486` (create, returning
`E_QUOTA`), `objects.cc:919, 936` (recycle), `server.cc:652` and
`utils.cc:200, 251` (anonymous-object / GC reclamation).

### Other caps
- `finished_tasks_limit` — bounds retained finished-task records.
- `max_background_threads` (`background.cc`) — threaded-builtin pool size.
- `file_io_max_files` (`fileio.cc`), `sqlite_max_handles` (`sqlite.cc`) — per-server
  handle caps.
- `json_max_parse_depth`, `pcre_match_max_iterations` — bound individual builtin
  calls.
- `outbound_connect_timeout` / `DEFAULT_CONNECT_TIMEOUT` 300 — `open_network_connection`.
