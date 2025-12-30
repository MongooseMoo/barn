# Task: Implement Anonymous Object Parent Invalidation

## Context
Barn is a Go MOO server. Anonymous objects (created with `create(parent, 1)`) should become invalid when their parent hierarchy changes. This is ToastStunt behavior.

## Problem
Currently, anonymous objects remain valid even after their parent is recycled or modified. The following tests fail:

1. `recycling_parent_invalidates_anonymous` - recycle parent, child should be invalid
2. `chparents_invalidates_anonymous` - change parent's parents, child invalid
3. `add_property_to_parent_invalidates_anonymous` - add property to parent, child invalid
4. `delete_property_from_parent_invalidates_anonymous` - delete property from parent, child invalid
5. `renumber_parent_invalidates_anonymous` - renumber parent, child invalid
6. `replace_parent_does_not_corrupt_anonymous` - this one should PASS (just checking typeof)

## Test Cases
```moo
// recycling_parent_invalidates_anonymous
p = create($nothing);
a = create(p, 1);
recycle(p);
return valid(a);  // Should return 0

// chparents_invalidates_anonymous
p1 = create($nothing);
p2 = create($nothing);
a = create(p1, 1);
chparents(p1, {p2});
return valid(a);  // Should return 0

// add_property_to_parent_invalidates_anonymous
p = create($nothing);
a = create(p, 1);
add_property(p, "foo", 123, {player, ""});
return valid(a);  // Should return 0
```

## Implementation Approach

Anonymous objects need to track their "schema version" or similar. When the parent hierarchy changes in ways that would affect inherited properties/verbs, anonymous children become invalid.

Key files to examine:
- `db/object.go` - Object struct, may need to track anonymous children
- `db/store.go` - Store operations, recycle, chparents
- `builtins/objects.go` - recycle(), chparents(), add_property(), delete_property()
- `builtins/properties.go` - add_property, delete_property

Options:
1. **Track anonymous children on parent** - Each object tracks its anonymous children, invalidate them on change
2. **Schema versioning** - Anonymous objects store parent schema version, check on access
3. **Invalidation flag** - Set invalid flag on anonymous object when parent changes

Option 1 is probably simplest - when recycle/chparents/add_property/delete_property is called on an object, invalidate all its anonymous children.

## Key Changes Needed

1. Add `AnonymousChildren []ObjID` or similar to Object struct
2. When creating anonymous object, register it with parent
3. In recycle(), chparents(), add_property(), delete_property() - invalidate anonymous children
4. "Invalidate" means set a flag that makes valid() return 0

## Test Command
```bash
cd ~/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9935 -k "invalidates_anonymous or replace_parent" -v
```

## Output
Write findings/status to `./reports/fix-anon-parent-invalidation.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
