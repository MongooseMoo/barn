# Fix Traceback to Show Correct Verb Location

## Problem

When an error occurred in an inherited verb, Barn's traceback showed the wrong object.

**Before:**
```
#2 <- #2:news (this == #2), line 1:  Range error
```

**Expected (Toast reference):**
```
#6:news, line 45
```

The verb `news` is defined on #6 ("generic player"), but #2 inherits it. The traceback should show where the verb is DEFINED (#6), not just the `this` object (#2).

## Root Cause

The issue was in three places:

1. **server/verbs.go**: `VerbMatch` struct only tracked `This` (the object the verb is called on) but not where the verb is defined
2. **server/verbs.go**: `findVerbOnObject()` returned `objID` (original search target) instead of `currentID` (where verb was actually found)
3. **server/scheduler.go**: Initial activation frame used `t.This` for `VerbLoc` with a TODO comment noting this was wrong

## Solution

### 1. Enhanced VerbMatch struct

Added `VerbLoc` field to track where the verb is defined:

```go
// VerbMatch is the result of verb lookup
type VerbMatch struct {
	Verb    *db.Verb
	This    types.ObjID // Object the verb is called on ('this' in MOO)
	VerbLoc types.ObjID // Object where verb is defined (for traceback)
}
```

### 2. Updated findVerbOnObject()

Changed to track both the original target and where the verb was found:

```go
// Check verbs on this object
for _, verb := range obj.Verbs {
	if verbMatches(verb, cmd, objID) {
		return &VerbMatch{
			Verb:    verb,
			This:    objID,    // Original target object
			VerbLoc: currentID, // Where verb is actually defined
		}
	}
}
```

When searching #2's inheritance chain and finding the verb on #6:
- `This` = #2 (original target, correct value for MOO's `this`)
- `VerbLoc` = #6 (where verb is defined, correct for traceback)

### 3. Added VerbLoc to Task struct

Added field to task/task.go to store the verb location:

```go
// Verb context (set for verb tasks)
VerbName     string
VerbLoc      types.ObjID // Object where verb is defined (for traceback)
This         types.ObjID // Object this verb is called on
```

### 4. Updated CreateVerbTask

Store the VerbLoc from the match:

```go
// Set up verb context
t.VerbName = match.Verb.Name
t.VerbLoc = match.VerbLoc
t.This = match.This
```

### 5. Fixed ActivationFrame creation

Changed from using `t.This` to using `t.VerbLoc`:

```go
// Push initial activation frame for traceback support
t.PushFrame(task.ActivationFrame{
	This:       t.This,
	Player:     t.Owner,
	Programmer: t.Programmer,
	Caller:     t.Caller,
	Verb:       t.VerbName,
	VerbLoc:    t.VerbLoc, // Object where verb is defined
	LineNumber: 1,
})
```

### Other locations verified

Checked other places where `ActivationFrame` is created:
- **vm/verbs.go** (CallVerb): Already correctly uses `defObjID` for VerbLoc
- **vm/builtin_pass.go**: Already correctly uses `defObjID` for VerbLoc

## Testing

### Before Fix
```
$ ./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "news"
It's the current issue of the News, dated Mon May  1,2 200.

#2 <- #2:news (this == #2), line 1:  Range error
#2 <- (End of traceback)
```

### After Fix
```
$ ./moo_client.exe -port 9300 -cmd "connect wizard" -cmd "news"
It's the current issue of the News, dated Mon May  1,2 200.

#2 <- #6:news (this == #2), line 1:  Range error
#2 <- (End of traceback)
```

### Toast Reference
```
$ ./moo_client.exe -port 9400 -cmd "connect wizard" -cmd "news"
#61:description (this == #61), line 1:  Invalid argument
... called from #61:news_display_seq_full (this == #61), line 4
... called from #6:news, line 45
(End of traceback)
```

## Verification

The fix correctly shows:
- **#6:news** - verb location (where verb is defined)
- **(this == #2)** - the `this` object (where verb was called)

This matches the Toast behavior where each traceback line shows the object where the verb is defined, not just the `this` object.

## Files Modified

1. `server/verbs.go` - Added VerbLoc to VerbMatch, updated findVerbOnObject
2. `task/task.go` - Added VerbLoc field to Task struct
3. `server/scheduler.go` - Store VerbLoc in CreateVerbTask, use it in ActivationFrame

## Status

✓ Fixed - Traceback now shows correct verb location (#6:news) instead of this object (#2:news)
