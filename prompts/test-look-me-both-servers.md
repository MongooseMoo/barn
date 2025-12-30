# Task: Test "look me" on Toast and Barn

## Objective
Run "connect wizard" then "look me" on BOTH servers using toastcore_barn.db. Capture exact output from each.

## Commands

### Toast (reference server)
```bash
cd /c/Users/Q/code/barn
# Kill any existing
taskkill //F //IM toast_moo.exe 2>/dev/null || true
# Start Toast
./toast_moo.exe toastcore_barn.db toast_output.db -p 9700 &
sleep 3
# Test
./moo_client.exe -port 9700 -timeout 5 -cmd "connect wizard" -cmd "look me"
```

### Barn
```bash
cd /c/Users/Q/code/barn
# Kill any existing
taskkill //F //IM barn_test.exe 2>/dev/null || true
# Build if needed
go build -o barn_test.exe ./cmd/barn/
# Start Barn
./barn_test.exe -db toastcore_barn.db -port 9300 &
sleep 3
# Test
./moo_client.exe -port 9300 -timeout 5 -cmd "connect wizard" -cmd "look me"
```

## Output
Write to `./reports/test-look-me-both-servers.md`:
1. Exact Toast output
2. Exact Barn output
3. One sentence noting any difference

Do NOT analyze root cause. Just capture and note difference.
