# Task: Trace Why "The First Room" Doesn't Show on Login

## The Problem

Toast shows "The First Room" after "connect wizard". Barn doesn't.

Something errors during login but the traceback is hidden (either caught by try/except or happening before connection setup).

## What Toast Shows
```
*** Connected ***
The First Room
This is all there is right now.
```

## What Barn Shows
```
Welcome to the ToastCore database.
Type 'connect wizard' to log in.
```

No room name, no room description.

## Investigation Steps

1. Find where room description gets shown on login in MOO code
   - Probably in `$login:connect` or similar
   - Or `user_connected` hook
   - Or player's `:look()` being called

2. Use dump_verb.exe to trace the login flow:
   ```bash
   ./dump_verb.exe 0 do_login_command
   ./dump_verb.exe <login_obj> connect
   ./dump_verb.exe 0 user_connected
   ```

3. Add temporary logging to Barn if needed - or check if there's an error being caught

4. The try/except in do_login_command (lines 22-29) might be swallowing an error

## Test Commands
```bash
cd /c/Users/Q/code/barn
./barn_test.exe -db toastcore_barn.db -port 9300 2>&1 | tee server.log &
sleep 3
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard"
cat server.log
```

## Output

Write to `./reports/trace-login-room-missing.md`:
1. The MOO code path that should show the room
2. Where it fails or gets skipped in Barn
3. The specific error or missing functionality
4. Suggested fix (or implement if simple)
