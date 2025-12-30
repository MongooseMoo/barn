# Fix Range Error in messages_in_seq

## Investigation Summary

The `news` command fails with a Range error in `#46:messages_in_seq` at line 6 when the end index exceeds the list length.

## Root Cause

The verb code at line 6 is:
```moo
return caller.messages[msgs[1]..msgs[2] - 1];
```

Where:
- `msgs[1]` = start index (1-based)
- `msgs[2]` = one past the end index (exclusive upper bound)
- `msgs[2] - 1` = actual end index to include

The problem: There's no bounds checking. If `caller.messages` has N items and `msgs[2]` > N+1, then `msgs[2] - 1` may still be > N, causing a Range error.

## Toast Behavior Verification

Toast (reference implementation) enforces strict range bounds:

```bash
$ ./toast_oracle.exe '{"a", "b", "c"}[1..3]'
{"a", "b", "c"}  # Valid: end index equals length

$ ./toast_oracle.exe '{"a", "b", "c"}[1..4]'
=> *Aborted*  # Range error: end index beyond length
```

**Barn's behavior is CORRECT** - it properly raises E_RANGE for out-of-bounds indices.

## The Bug

The bug is in the MOO code, not in Barn's range implementation. The verb should clamp the end index to prevent accessing beyond the list length.

## Recommended Fix

The verb should be modified to clamp the indices:

```moo
:messages_in_seq(msg_seq) => list of messages in msg_seq on folder (caller)";
set_task_perms(caller_perms());
if (typeof(msgs = args[1]) != LIST)
  return caller.messages[msgs];
elseif (length(msgs) == 2)
  start = msgs[1];
  end = min(msgs[2] - 1, length(caller.messages));
  if (start > length(caller.messages))
    return {};
  endif
  return caller.messages[start..end];
else
  return $seq_utils:extract(msgs, caller.messages);
endif
```

Changes:
1. Clamp `end` to `length(caller.messages)`
2. Handle case where `start` is already beyond the list (return empty list)
3. This matches the semantics that `msgs[2]` is meant to be an exclusive upper bound

## Alternative: Check Caller Code

Before modifying `messages_in_seq`, we should check what passes the bounds in the first place. The verb is called from:
- `#45:messages_in_seq` (line 24)
- `#61:news_display_seq_full` (line 7)

The calling code may have a bug in how it calculates the message sequence range. Fixing it upstream might be better than adding defensive code here.

## Test Case

To reproduce:
```moo
; player.messages = {{1, {100, "s", "subj", "body"}}, {2, {200, "s2", "subj2", "body2"}}};
; return player:messages_in_seq({1, 10});  # Should not raise Range error
```

Expected: Returns all messages (or at least doesn't crash)
Actual: Raises Range error because `10 - 1 = 9 > length(messages) = 2`

## Conclusion

This is a **MOO code bug**, not a Barn implementation bug. Barn correctly enforces MOO semantics for range operations. The toastcore database verb needs to be fixed to handle out-of-bounds ranges gracefully.
