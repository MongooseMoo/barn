# Objects Feature Audit - Gap Report

**Auditor:** Blind Implementor (spec-only perspective)
**Date:** 2025-12-24
**Scope:** Object creation, destruction, properties, verbs, recycling, inheritance

---

## Summary Statistics

- **Total Gaps Identified:** 47
- **Critical (guess):** 18
- **High (assume):** 15
- **Medium (ask):** 10
- **Low (test):** 4

---

## Gap Catalog

### Object Creation and Lifecycle

#### GAP-OBJ-001
- **id:** GAP-OBJ-001
- **feature:** "object creation"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "1.1 create"
- **gap_type:** guess
- **question:** |
    What object ID is assigned when creating a new object? The spec says "Allocate new object ID"
    but doesn't specify the allocation strategy. Is it:
    (a) Next sequential ID after max_object()
    (b) First available recycled slot
    (c) Recycled slots preferred, then sequential
    (d) Some other strategy?
- **impact:** |
    Different strategies affect object ID predictability, recycling behavior, and database
    compaction. Implementors might choose different strategies leading to incompatible behavior.
- **suggested_addition:** |
    Add: "New object IDs are allocated sequentially starting from max_object() + 1. Recycled
    object slots are NOT automatically reused unless recreate() is explicitly called."

#### GAP-OBJ-002
- **id:** GAP-OBJ-002
- **feature:** "object creation"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "1.1 create"
- **gap_type:** guess
- **question:** |
    When create() calls the "initialize" verb, what arguments are passed? The spec says
    "Call initialize verb if defined" but doesn't specify:
    - Arguments passed to initialize
    - Value of context variables (this, player, caller, etc.)
    - Whether initialize can raise errors that abort creation
- **impact:** |
    Initialize verbs need to know what arguments to expect and what context they execute in.
    Error handling during initialization is undefined.
- **suggested_addition:** |
    Add: "The initialize verb is called with no arguments. Context variables: this=newly created
    object, player=creator, caller=creator. If initialize raises an error, the object creation
    is NOT rolled back; the object exists but may be in an invalid state."

#### GAP-OBJ-003
- **id:** GAP-OBJ-003
- **feature:** "object creation"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "1.1 create"
- **gap_type:** assume
- **question:** |
    The spec says "Copy inherited properties" but doesn't clarify whether this copies:
    (a) Only properties defined on the immediate parent(s)
    (b) All properties from entire inheritance chain
    (c) Properties as value copies or references
- **impact:** |
    Deep vs shallow property inheritance affects memory usage and whether property values
    are shared or independent.
- **suggested_addition:** |
    Add: "All properties from the entire inheritance chain are copied to the new object as
    independent values. Clear properties remain clear (inheriting). List and map values
    are copy-on-write."

#### GAP-OBJ-004
- **id:** GAP-OBJ-004
- **feature:** "object creation"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "1.1 create"
- **gap_type:** ask
- **question:** |
    What happens if you create() with an empty parent list: create({})? Does it:
    (a) Raise E_INVARG
    (b) Create object with no parents (orphan)
    (c) Implicitly use $nothing as parent
- **impact:** |
    Affects whether parentless objects are possible and how they behave.
- **suggested_addition:** |
    Add: "Creating with empty parent list {} raises E_INVARG. All objects must have at
    least one parent."

#### GAP-OBJ-005
- **id:** GAP-OBJ-005
- **feature:** "object creation"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "1.1 create"
- **gap_type:** guess
- **question:** |
    When checking if parent is "fertile", does the check apply to ALL parents in multiple
    inheritance, or just the first one? Can you create from non-fertile parents if you're wizard?
- **impact:** |
    Multiple inheritance fertility checking is ambiguous. Wizard bypass unclear.
- **suggested_addition:** |
    Add: "In multiple inheritance, ALL parents must be fertile unless caller is wizard.
    Wizards can create children of non-fertile objects."

#### GAP-OBJ-006
- **id:** GAP-OBJ-006
- **feature:** "object recycling"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "1.3 recycle"
- **gap_type:** guess
- **question:** |
    The spec says recycle() "removes from location's contents" but doesn't specify whether
    this happens before or after the recycle verb is called. If the recycle verb references
    obj.location, is it already #-1/$nothing?
- **impact:** |
    The recycle verb might need to know its own location before being moved.
- **suggested_addition:** |
    Add: "Order of operations: (1) Call recycle verb, (2) Move to $nothing, (3) Clear
    properties, (4) Remove verbs, (5) Mark as recycled. The recycle verb sees the object
    in its pre-destruction state."

#### GAP-OBJ-007
- **id:** GAP-OBJ-007
- **feature:** "object recycling"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "1.3 recycle"
- **gap_type:** ask
- **question:** |
    What happens to references to a recycled object held in properties, lists, or variables?
    Do they:
    (a) Automatically become #-1
    (b) Remain as the old object ID but valid() returns false
    (c) Raise E_INVIND when accessed
- **impact:** |
    Affects memory safety and whether stale references cause crashes or errors.
- **suggested_addition:** |
    Add: "Existing references to recycled objects remain as the old object ID. valid()
    returns false. Any operation on a recycled object (property access, verb call, etc.)
    raises E_INVIND."

#### GAP-OBJ-008
- **id:** GAP-OBJ-008
- **feature:** "object recycling"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "1.3 recycle"
- **gap_type:** guess
- **question:** |
    When recycling removes object from "parent's children", does this update happen
    atomically? Can another task see the object half-recycled?
- **impact:** |
    Concurrency and atomicity guarantees for recycling.
- **suggested_addition:** |
    Add: "Recycling is atomic from the perspective of other tasks. The object transitions
    from valid to invalid in a single step; intermediate states are not visible."

#### GAP-OBJ-009
- **id:** GAP-OBJ-009
- **feature:** "object recreation"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "1.2 recreate"
- **gap_type:** ask
- **question:** |
    Can you recreate() an object ID that was never created in the first place (e.g., #9999
    when max_object is #500)? Or only IDs that were previously created and recycled?
- **impact:** |
    Determines whether recreate() can create sparse object ID spaces.
- **suggested_addition:** |
    Add: "recreate() can only reuse object IDs that were previously allocated and recycled.
    Attempting to recreate an ID that was never created raises E_INVARG."

#### GAP-OBJ-010
- **id:** GAP-OBJ-010
- **feature:** "object recreation"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "1.2 recreate"
- **gap_type:** test
- **question:** |
    Does recreate() call the initialize verb like create() does?
- **impact:** |
    Initialization consistency between create and recreate.
- **suggested_addition:** |
    Add: "recreate() follows the same initialization sequence as create(), including
    calling the initialize verb if defined."

---

### Object Validity and References

#### GAP-OBJ-011
- **id:** GAP-OBJ-011
- **feature:** "object validity"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "2.1 valid"
- **gap_type:** assume
- **question:** |
    The spec says valid(#-1) returns false. What about other negative object IDs like #-2,
    #-3? Are they all invalid, or do special sentinels like $failed_match have special handling?
- **impact:** |
    Sentinel object validity semantics unclear.
- **suggested_addition:** |
    Add: "All negative object IDs return false from valid(), including sentinels like
    $nothing (#-1), $failed_match (#-2), $ambiguous_match (#-3). These are symbolic
    constants, not real objects."

#### GAP-OBJ-012
- **id:** GAP-OBJ-012
- **feature:** "object validity"
- **spec_file:** "spec/objects.md"
- **spec_section:** "8.1 Checking Validity"
- **gap_type:** guess
- **question:** |
    When does an object become invalid? Only after recycle(), or are there other conditions?
    What about anonymous objects that are garbage collected?
- **impact:** |
    Full set of conditions that make an object invalid.
- **suggested_addition:** |
    Add: "An object is invalid if: (1) it was recycled via recycle(), (2) it is an
    anonymous object that was garbage collected, (3) the object ID is negative, or
    (4) the object ID exceeds max_object()."

---

### Property Inheritance and Clear

#### GAP-OBJ-013
- **id:** GAP-OBJ-013
- **feature:** "property inheritance"
- **spec_file:** "spec/objects.md"
- **spec_section:** "3.3 Inheritance Resolution"
- **gap_type:** guess
- **question:** |
    The spec says inheritance is "breadth-first" but the example suggests "depth-first"
    ("First path wins (left-to-right, depth-first)"). Which is it? If obj has parents
    {A, B}, and A has parent X, B has parent Y, is the order:
    (a) obj → A → B → X → Y (breadth-first)
    (b) obj → A → X → B → Y (depth-first)
- **impact:** |
    Diamond inheritance resolution order is critical for property/verb lookup.
- **suggested_addition:** |
    Add: "Inheritance search is depth-first, left-to-right. For parents {A, B}, search
    order is: obj → A → A's parents (recursively) → B → B's parents (recursively).
    First match wins. Cycles are detected via visited tracking."

#### GAP-OBJ-014
- **id:** GAP-OBJ-014
- **feature:** "property clear"
- **spec_file:** "spec/builtins/properties.md"
- **spec_section:** "5.1 clear_property"
- **gap_type:** ask
- **question:** |
    If you clear_property() on a property that is already clear (inherited), what happens?
    (a) No-op (succeeds silently)
    (b) Raises E_PROPNF
    (c) Raises some other error
- **impact:** |
    Idempotency of clear_property operation.
- **suggested_addition:** |
    Add: "clear_property() on an already-clear property is a no-op (succeeds silently).
    Only raises E_PROPNF if the property doesn't exist anywhere in the inheritance chain."

#### GAP-OBJ-015
- **id:** GAP-OBJ-015
- **feature:** "property clear"
- **spec_file:** "spec/builtins/properties.md"
- **spec_section:** "5.1 clear_property"
- **gap_type:** guess
- **question:** |
    When a property is cleared, reading it returns the parent's value. But what if the
    parent's property is also clear? Does it continue up the chain, or return an error?
- **impact:** |
    Multi-level clear property inheritance.
- **suggested_addition:** |
    Add: "Clear properties inherit recursively. If obj.prop is clear, reading obj.prop
    searches up the parent chain until a non-clear value is found. If all ancestors have
    clear props, raises E_PROPNF."

#### GAP-OBJ-016
- **id:** GAP-OBJ-016
- **feature:** "property clear"
- **spec_file:** "spec/objects.md"
- **spec_section:** "4.4 Clear Properties"
- **gap_type:** assume
- **question:** |
    Can you write to a clear property? If so, does it:
    (a) Set the value locally (un-clearing it)
    (b) Raise an error
    (c) Write through to the parent
- **impact:** |
    Write semantics for clear properties.
- **suggested_addition:** |
    Add: "Writing to a clear property sets the value locally, un-clearing it. The property
    now has its own value instead of inheriting."

#### GAP-OBJ-017
- **id:** GAP-OBJ-017
- **feature:** "property addition"
- **spec_file:** "spec/builtins/properties.md"
- **spec_section:** "4.1 add_property"
- **gap_type:** guess
- **question:** |
    What happens if you add_property() with a name that already exists in the inheritance
    chain? Does it:
    (a) Raise E_INVARG (property exists)
    (b) Succeed (shadows parent property)
    (c) Depend on whether the property is clear or not
- **impact:** |
    Property shadowing vs. uniqueness constraints.
- **suggested_addition:** |
    Add: "add_property() raises E_INVARG if a property with that name already exists on
    the object itself (not inherited). Inherited properties CAN be shadowed by adding a
    local property with the same name."

#### GAP-OBJ-018
- **id:** GAP-OBJ-018
- **feature:** "property deletion"
- **spec_file:** "spec/builtins/properties.md"
- **spec_section:** "4.2 delete_property"
- **gap_type:** ask
- **question:** |
    The spec says "Cannot delete inherited properties; use clear_property instead." But
    what if the property is defined locally AND also exists on parent? Does delete_property():
    (a) Remove local definition (revealing parent's version)
    (b) Raise E_PROPNF
- **impact:** |
    Deletion vs. clearing for shadowed properties.
- **suggested_addition:** |
    Add: "delete_property() removes the local property definition. If the same property
    exists on a parent, it becomes visible (not deleted). Only raises E_PROPNF if the
    property isn't defined on the object itself."

---

### Property Permissions

#### GAP-OBJ-019
- **id:** GAP-OBJ-019
- **feature:** "property permissions"
- **spec_file:** "spec/builtins/properties.md"
- **spec_section:** "9.1 Read Permission"
- **gap_type:** guess
- **question:** |
    If a property is inherited, whose permissions apply? The property owner's permissions,
    or the object owner's permissions, or both?
- **impact:** |
    Inherited property permission model.
- **suggested_addition:** |
    Add: "Inherited properties use the permissions defined where the property is defined.
    Reading obj.prop checks permissions on the ancestor where prop is defined, not obj itself."

#### GAP-OBJ-020
- **id:** GAP-OBJ-020
- **feature:** "property permissions"
- **spec_file:** "spec/builtins/properties.md"
- **spec_section:** "3.1 property_info"
- **gap_type:** ask
- **question:** |
    The "c" permission means "chown allowed". Does this mean:
    (a) The property owner can be changed via set_property_info
    (b) The property value can be transferred to a different owner
    (c) Something else entirely
- **impact:** |
    Meaning of chown permission is unclear.
- **suggested_addition:** |
    Add: "The 'c' permission allows changing the property's owner field via set_property_info().
    Without 'c', only wizards can change property ownership."

#### GAP-OBJ-021
- **id:** GAP-OBJ-021
- **feature:** "property permissions"
- **spec_file:** "spec/objects.md"
- **spec_section:** "4.2 Property Permissions"
- **gap_type:** guess
- **question:** |
    What are the default permissions for a newly added property? The spec doesn't specify
    what happens if you call add_property(obj, "name", value, "rw") - are permissions
    stored as string or parsed into bits?
- **impact:** |
    Permission storage format and defaults.
- **suggested_addition:** |
    Add: "The info parameter can be either {owner, perms_string} or just perms_string.
    If only perms_string is provided, owner defaults to caller. perms_string is parsed
    into permission bits: 'r' = PROP_READ, 'w' = PROP_WRITE, 'c' = PROP_CHOWN."

---

### Verb Dispatch and Inheritance

#### GAP-OBJ-022
- **id:** GAP-OBJ-022
- **feature:** "verb dispatch"
- **spec_file:** "spec/objects.md"
- **spec_section:** "5.4 Verb Dispatch"
- **gap_type:** guess
- **question:** |
    The verb dispatch algorithm mentions checking "verb argument specifiers match" but
    doesn't specify what happens if they DON'T match. Does it:
    (a) Raise E_VERBNF
    (b) Continue searching up the inheritance chain
    (c) Raise E_ARGS
- **impact:** |
    Error handling for mismatched verb argument specs.
- **suggested_addition:** |
    Add: "If a verb is found but argument specifiers don't match, search continues up
    the inheritance chain. E_VERBNF is raised only if no matching verb is found anywhere."

#### GAP-OBJ-023
- **id:** GAP-OBJ-023
- **feature:** "verb dispatch"
- **spec_file:** "spec/objects.md"
- **spec_section:** "5.4 Verb Dispatch"
- **gap_type:** ask
- **question:** |
    When parsing "verb dobj prep iobj", what is the delimiter? Spaces? How are quoted
    strings handled? Is "put \"magic sword\" in box" parsed as dobj="magic sword"?
- **impact:** |
    Command parsing is underspecified.
- **suggested_addition:** |
    Add: "Command parsing splits on whitespace. Quoted strings (double quotes) are treated
    as single tokens. Prepositions are matched from a predefined list (spec/prepositions.md).
    This applies to command-line parsing, not direct verb calls (obj:verb())."

#### GAP-OBJ-024
- **id:** GAP-OBJ-024
- **feature:** "verb execution"
- **spec_file:** "spec/objects.md"
- **spec_section:** "5.5 Context Variables"
- **gap_type:** assume
- **question:** |
    The spec lists context variables but doesn't say whether they are read-only or writable.
    Can verb code do: this = #123; player = other_player; ?
- **impact:** |
    Mutability of context variables.
- **suggested_addition:** |
    Add: "All context variables (this, player, caller, verb, args, etc.) are read-only.
    Attempting to assign to them raises E_VARNF or is a compile-time error."

#### GAP-OBJ-025
- **id:** GAP-OBJ-025
- **feature:** "verb pass"
- **spec_file:** "spec/builtins/verbs.md"
- **spec_section:** "1.2 pass"
- **gap_type:** guess
- **question:** |
    When pass() is called, which parent's verb is invoked in multiple inheritance?
    First parent in the parents list? The same parent where current verb was found?
- **impact:** |
    Multiple inheritance pass() behavior.
- **suggested_addition:** |
    Add: "pass() searches for the same verb name starting from the parent of the object
    where the current verb is defined. In multiple inheritance, follows the same
    depth-first search order. If current verb is on obj inherited from parent A, pass()
    searches A's parents."

#### GAP-OBJ-026
- **id:** GAP-OBJ-026
- **feature:** "verb pass"
- **spec_file:** "spec/builtins/verbs.md"
- **spec_section:** "1.2 pass"
- **gap_type:** ask
- **question:** |
    What happens if pass() is called from code that wasn't invoked as a verb (e.g., from
    eval() or a suspended task continuation)? Does it raise E_VERBNF or something else?
- **impact:** |
    pass() context requirements.
- **suggested_addition:** |
    Add: "pass() can only be called from within a verb execution. Calling pass() from
    eval() or outside verb context raises E_VERBNF."

---

### Verb Names and Aliases

#### GAP-OBJ-027
- **id:** GAP-OBJ-027
- **feature:** "verb names"
- **spec_file:** "spec/builtins/verbs.md"
- **spec_section:** "3.1 verb_info"
- **gap_type:** guess
- **question:** |
    The verb_info return value includes "names" as a string. If a verb has multiple aliases
    (e.g., "get take grab"), how is this represented? Space-separated? Comma-separated?
- **impact:** |
    Verb alias storage format.
- **suggested_addition:** |
    Add: "The names field is a space-separated string of verb aliases. Example: 'get take grab'.
    The first name is the primary name. All aliases invoke the same verb code."

#### GAP-OBJ-028
- **id:** GAP-OBJ-028
- **feature:** "verb names"
- **spec_file:** "spec/builtins/verbs.md"
- **spec_section:** "6.1 add_verb"
- **gap_type:** ask
- **question:** |
    When adding a verb with multiple names, do ALL names need to be unique across the
    object's verbs, or just the primary name?
- **impact:** |
    Verb name collision detection.
- **suggested_addition:** |
    Add: "All verb names (including aliases) must be unique within the object. add_verb()
    raises E_INVARG if any name in the names string conflicts with an existing verb name
    or alias."

---

### Verb Permissions

#### GAP-OBJ-029
- **id:** GAP-OBJ-029
- **feature:** "verb permissions"
- **spec_file:** "spec/builtins/verbs.md"
- **spec_section:** "9.1 Execution"
- **gap_type:** assume
- **question:** |
    If a verb doesn't have 'x' permission, can it still be called via pass() from a child
    verb that does have permission?
- **impact:** |
    Permission inheritance in verb dispatch.
- **suggested_addition:** |
    Add: "Verb execution permission is checked at call time. pass() inherits the caller's
    permissions; if the caller had permission to execute the child verb, they can pass()
    to the parent verb regardless of the parent verb's 'x' flag."

#### GAP-OBJ-030
- **id:** GAP-OBJ-030
- **feature:** "verb permissions"
- **spec_file:** "spec/objects.md"
- **spec_section:** "5.2 Verb Permissions"
- **gap_type:** guess
- **question:** |
    What does the 'd' (debug) permission actually do? The spec mentions it exists but
    doesn't specify its behavior.
- **impact:** |
    Debug permission semantics.
- **suggested_addition:** |
    Add: "The 'd' permission enables debug information for the verb. With 'd', stack
    traces include line numbers and local variable values. Without 'd', stack traces
    show only verb name. This is a privacy/security feature."

---

### Object Flags

#### GAP-OBJ-031
- **id:** GAP-OBJ-031
- **feature:** "object flags"
- **spec_file:** "spec/objects.md"
- **spec_section:** "2.1 Flag Types"
- **gap_type:** ask
- **question:** |
    Can an object have both RECYCLED (1024) and INVALID (512) flags set simultaneously?
    Are these distinct states or is RECYCLED a subset of INVALID?
- **impact:** |
    Flag state machine semantics.
- **suggested_addition:** |
    Add: "RECYCLED implies INVALID. When an object is recycled, both INVALID and RECYCLED
    flags are set. INVALID can be set independently for other reasons (e.g., marking
    temporary invalidity)."

#### GAP-OBJ-032
- **id:** GAP-OBJ-032
- **feature:** "object flags"
- **spec_file:** "spec/objects.md"
- **spec_section:** "2.1 Flag Types"
- **gap_type:** guess
- **question:** |
    The ANONYMOUS flag (256) indicates garbage collection. When exactly is an anonymous
    object collected? Immediately when last reference is gone, or at next GC cycle?
- **impact:** |
    GC timing and determinism.
- **suggested_addition:** |
    Add: "Anonymous objects are collected during periodic garbage collection cycles (not
    reference-counted). There is no guarantee of immediate collection when the last
    reference is gone. GC timing is implementation-dependent."

#### GAP-OBJ-033
- **id:** GAP-OBJ-033
- **feature:** "object flags"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "4.1 Flags"
- **gap_type:** ask
- **question:** |
    Can non-wizards set the wizard flag on objects they own? Or is setting wizard flag
    always restricted to existing wizards?
- **impact:** |
    Privilege escalation prevention.
- **suggested_addition:** |
    Add: "Only wizards can set the wizard flag. set_object_flag(obj, 'wizard', 1) raises
    E_PERM if caller is not a wizard, even if caller owns obj."

---

### Location and Contents

#### GAP-OBJ-034
- **id:** GAP-OBJ-034
- **feature:** "move object"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "5.2 move"
- **gap_type:** guess
- **question:** |
    The spec says move() calls "exitfunc on old location" and "enterfunc on new location".
    What arguments are passed to these functions? What if they raise errors?
- **impact:** |
    Move hook specification.
- **suggested_addition:** |
    Add: "exitfunc is called as: old_location:exitfunc(what). enterfunc is called as:
    new_location:enterfunc(what). If either raises an error, the move is NOT rolled back;
    the object has already moved. Errors propagate to the move() caller."

#### GAP-OBJ-035
- **id:** GAP-OBJ-035
- **feature:** "move object"
- **spec_file:** "spec/objects.md"
- **spec_section:** "7.2 Moving Objects"
- **gap_type:** ask
- **question:** |
    Can you move an object to $nothing? What about from $nothing? Is $nothing a valid
    location?
- **impact:** |
    Null location semantics.
- **suggested_addition:** |
    Add: "Moving to $nothing is valid and removes the object from world (location = #-1,
    not in any contents list). Moving from $nothing is valid. $nothing is a special
    location that has no contents list."

#### GAP-OBJ-036
- **id:** GAP-OBJ-036
- **feature:** "move object"
- **spec_file:** "spec/objects.md"
- **spec_section:** "7.2 Moving Objects"
- **gap_type:** guess
- **question:** |
    What happens if another task modifies location or contents while move() is executing?
    Is move() atomic?
- **impact:** |
    Concurrency safety for move operations.
- **suggested_addition:** |
    Add: "move() operations are atomic. Contents and location updates happen in a
    transaction. Other tasks see either the before-move state or after-move state,
    never intermediate states."

---

### Inheritance Cycle Detection

#### GAP-OBJ-037
- **id:** GAP-OBJ-037
- **feature:** "parent cycles"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "3.6 chparent"
- **gap_type:** assume
- **question:** |
    How is "would create cycle" detected in chparent()? Is it:
    (a) obj cannot be ancestor of new_parent (prevents cycles)
    (b) obj cannot be descendant of new_parent (prevents cycles)
    (c) Both checks
- **impact:** |
    Cycle detection algorithm specification.
- **suggested_addition:** |
    Add: "chparent(obj, new_parent) raises E_INVARG if new_parent is obj itself or any
    descendant of obj. This check walks the descendant tree to ensure no cycles."

#### GAP-OBJ-038
- **id:** GAP-OBJ-038
- **feature:** "property lookup cycles"
- **spec_file:** "spec/objects.md"
- **spec_section:** "13.4 Inheritance Resolution"
- **gap_type:** test
- **question:** |
    The Go implementation shows cycle detection with a visited map. But what if cycles
    exist in the object graph (should be prevented, but bugs happen)? Does lookup:
    (a) Detect and raise error
    (b) Infinite loop (crash)
    (c) Return some default value
- **impact:** |
    Defensive programming for corrupted graphs.
- **suggested_addition:** |
    Add: "Property and verb lookup maintain a visited set during traversal. If a cycle
    is detected (object visited twice), lookup raises E_INVIND to prevent infinite loops."

---

### Built-in Properties

#### GAP-OBJ-039
- **id:** GAP-OBJ-039
- **feature:** "built-in properties"
- **spec_file:** "spec/objects.md"
- **spec_section:** "4.6 Built-in Properties"
- **gap_type:** guess
- **question:** |
    Can built-in properties like .name, .owner, .location be deleted or cleared? Are they
    special-cased?
- **impact:** |
    Immutability of system properties.
- **suggested_addition:** |
    Add: "Built-in properties cannot be deleted via delete_property() (raises E_INVARG).
    They cannot be cleared via clear_property(). They are always present on every object."

#### GAP-OBJ-040
- **id:** GAP-OBJ-040
- **feature:** "built-in properties"
- **spec_file:** "spec/objects.md"
- **spec_section:** "4.6 Built-in Properties"
- **gap_type:** ask
- **question:** |
    The spec lists .contents as a built-in property of type LIST. Is this list writable?
    Can you do: obj.contents = {#1, #2}; or is it read-only (modified only via move())?
- **impact:** |
    Write protection for system-managed properties.
- **suggested_addition:** |
    Add: "The .contents property is read-only. Attempting to write to it raises E_PERM.
    Contents are modified only via move() operations."

#### GAP-OBJ-041
- **id:** GAP-OBJ-041
- **feature:** "built-in properties"
- **spec_file:** "spec/objects.md"
- **spec_section:** "4.6 Built-in Properties"
- **gap_type:** guess
- **question:** |
    The table shows .programmer, .wizard, .r, .w, .f as INT type. Are these 0/1 booleans,
    or can they have other integer values (treating as bitmask)?
- **impact:** |
    Flag property value domain.
- **suggested_addition:** |
    Add: "Flag properties (.programmer, .wizard, .r, .w, .f) are treated as booleans:
    0 = false, non-zero = true. Setting to any non-zero value sets the flag; only 0
    clears it."

---

### Object Quotas

#### GAP-OBJ-042
- **id:** GAP-OBJ-042
- **feature:** "object quota"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "1.1 create"
- **gap_type:** ask
- **question:** |
    The spec says create() can raise E_QUOTA for "Object quota exceeded". Who is the quota
    applied to? The creating player, the new object's owner, or global server limit?
- **impact:** |
    Quota accounting model.
- **suggested_addition:** |
    Add: "E_QUOTA is raised if the new object's owner would exceed their object quota.
    Quota is per-player. Wizards have unlimited quota. Quota is checked against owner,
    not creator (if different)."

#### GAP-OBJ-043
- **id:** GAP-OBJ-043
- **feature:** "object quota"
- **spec_file:** "spec/objects.md"
- **spec_section:** "6.1 Creation"
- **gap_type:** guess
- **question:** |
    When an object is recycled, does the owner's quota count decrease immediately, or
    only after garbage collection?
- **impact:** |
    Quota accounting timing.
- **suggested_addition:** |
    Add: "Recycling an object immediately decrements the owner's quota count. The quota
    is freed even before the object slot is reused."

---

### Waif Objects

#### GAP-OBJ-044
- **id:** GAP-OBJ-044
- **feature:** "waif objects"
- **spec_file:** "spec/objects.md"
- **spec_section:** "10.2 Characteristics"
- **gap_type:** guess
- **question:** |
    Waifs use `.:` syntax for properties. Is this a completely separate namespace from
    regular properties (accessed with `.`), or the same namespace with different syntax?
- **impact:** |
    Waif property isolation.
- **suggested_addition:** |
    Add: "Waif properties (.:name) are a separate namespace from object properties (.name).
    A waif can have both .name and .:name as distinct properties. The .: syntax is only
    for waifs; using it on regular objects raises E_TYPE."

#### GAP-OBJ-045
- **id:** GAP-OBJ-045
- **feature:** "waif objects"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "9.1 new_waif"
- **gap_type:** ask
- **question:** |
    What is the parent/class of a waif created with new_waif()? The spec mentions "waif
    class object" but doesn't specify how it's determined.
- **impact:** |
    Waif inheritance model.
- **suggested_addition:** |
    Add: "new_waif() must be called on an object that serves as the waif class. Example:
    $waif_class:new_waif(). The waif inherits properties and verbs from $waif_class.
    Calling new_waif() outside a verb context raises E_INVARG."

---

### Max Object and ID Management

#### GAP-OBJ-046
- **id:** GAP-OBJ-046
- **feature:** "max_object"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "7.1 max_object"
- **gap_type:** test
- **question:** |
    If objects #1, #2, #3 exist and #2 is recycled, what does max_object() return?
    Still #3, or does it track highest *valid* object?
- **impact:** |
    max_object semantics with recycled objects.
- **suggested_addition:** |
    Add: "max_object() returns the highest object ID ever allocated, regardless of
    whether that object is currently valid or recycled. It represents the high-water
    mark of object allocation."

#### GAP-OBJ-047
- **id:** GAP-OBJ-047
- **feature:** "reset_max_object"
- **spec_file:** "spec/builtins/objects.md"
- **spec_section:** "7.2 reset_max_object"
- **gap_type:** guess
- **question:** |
    What does "reclaims recycled object slots at end" mean? Does reset_max_object():
    (a) Lower max_object if trailing IDs are recycled (#98, #99, #100 recycled → max becomes #97)
    (b) Compact the object table
    (c) Something else
- **impact:** |
    Database compaction semantics.
- **suggested_addition:** |
    Add: "reset_max_object() scans backward from max_object() to find the highest valid
    (non-recycled) object ID and sets max_object to that value. This reclaims trailing
    recycled slots but does not compact gaps in the middle."

---

## Recommended Spec Patches

### High Priority (Blocking Implementation)

1. **GAP-OBJ-001**: Object ID allocation strategy
2. **GAP-OBJ-002**: Initialize verb calling convention
3. **GAP-OBJ-006**: Recycle operation ordering
4. **GAP-OBJ-007**: Recycled object reference behavior
5. **GAP-OBJ-013**: Inheritance search order (breadth vs depth)
6. **GAP-OBJ-019**: Inherited property permission model
7. **GAP-OBJ-022**: Verb dispatch argument mismatch handling
8. **GAP-OBJ-025**: Multiple inheritance pass() behavior
9. **GAP-OBJ-034**: Move hook arguments and error handling
10. **GAP-OBJ-037**: Cycle detection in chparent

### Medium Priority (Clarifications)

11. **GAP-OBJ-003**: Property copying depth
12. **GAP-OBJ-005**: Multi-parent fertility checking
13. **GAP-OBJ-014**: Clear property idempotency
14. **GAP-OBJ-015**: Multi-level clear inheritance
15. **GAP-OBJ-027**: Verb alias storage format
16. **GAP-OBJ-030**: Debug permission behavior
17. **GAP-OBJ-033**: Wizard flag privilege escalation
18. **GAP-OBJ-040**: Contents property mutability
19. **GAP-OBJ-042**: Quota accounting model
20. **GAP-OBJ-045**: Waif class determination

### Low Priority (Edge Cases)

21. **GAP-OBJ-004**: Empty parent list handling
22. **GAP-OBJ-009**: Recreate ID constraints
23. **GAP-OBJ-011**: Negative object ID validity
24. **GAP-OBJ-020**: Chown permission meaning
25. **GAP-OBJ-024**: Context variable mutability

---

## Implementation Blockers

An implementor **cannot** proceed without answers to these questions:

1. **Object lifecycle ordering** (GAP-OBJ-002, GAP-OBJ-006): What order do initialization/destruction steps occur?
2. **Inheritance algorithm** (GAP-OBJ-013): Breadth-first or depth-first search?
3. **Property clear semantics** (GAP-OBJ-015, GAP-OBJ-016): How does multi-level clearing work?
4. **Permission inheritance** (GAP-OBJ-019): Whose permissions apply for inherited properties?
5. **Verb dispatch resolution** (GAP-OBJ-022, GAP-OBJ-025): How are mismatches and pass() handled in multiple inheritance?
6. **Recycled object references** (GAP-OBJ-007): What happens to stale references?
7. **Move atomicity** (GAP-OBJ-034, GAP-OBJ-036): Hook calling and concurrency guarantees?

Without these specifications, different implementors will make different choices, leading to incompatible servers.

---

## Testing Recommendations

To validate the spec after patches are applied, create conformance tests for:

1. **Object lifecycle**: create → initialize → use → recycle → recreate
2. **Inheritance chains**: Single and multiple parent scenarios
3. **Property operations**: add, delete, clear, read, write through inheritance
4. **Verb dispatch**: Argument matching, pass() behavior, permission checks
5. **Move operations**: Location updates, hook calling, cycle prevention
6. **Recycling**: Reference handling, quota updates, slot reuse
7. **Concurrency**: Simultaneous operations on same object
8. **Permission enforcement**: All permission flags across operations

Minimum 100 test cases to cover the identified gaps.
