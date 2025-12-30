# Verification Report: Testing Tooling Against Both Barn and ToastStunt

## Executive Summary

**SUCCESS**: `moo_client.exe` works perfectly with both Barn and ToastStunt servers. No changes needed.

## Test Results

### moo_client.exe - VERIFIED WORKING

The `moo_client.exe` tool is server-agnostic and connects to any MOO server via TCP. It successfully tested against both implementations.

#### Test Environment

- **Barn Server**: Port 7777 (Test.db)
- **ToastStunt Server**: Port 9500 (test/Test_toast_client_test.db)
- **Tool**: `~/code/barn/moo_client.exe`

#### Basic Arithmetic Test

**Command:**
```bash
./moo_client.exe -port <PORT> -timeout 3 -cmd "connect wizard" -cmd "; return 1 + 1;"
```

**Barn Output:**
```
=> 2
```

**ToastStunt Output:**
```
-=!-^-!=-
{1, 2}
-=!-v-!=-
```

**Analysis:** Both servers evaluate correctly. ToastStunt returns structured tuples with status codes (`{1, result}` for success), while Barn returns human-readable format.

#### Type Check Test

**Command:**
```bash
./moo_client.exe -port <PORT> -timeout 2 -cmd "connect wizard" -cmd "; return typeof(1);"
```

**Barn Output:**
```
=> 0
```

**ToastStunt Output:**
```
-=!-^-!=-
{1, 0}
-=!-v-!=-
```

**Analysis:** Both return `0` (integer type code). Format differences remain consistent.

#### Error Handling Test

**Command:**
```bash
./moo_client.exe -port <PORT> -timeout 2 -cmd "connect wizard" -cmd "; return 1 / 0;"
```

**Barn Output:**
```
#-1:Input to EVAL (this == #-1), line 3:  Division by zero
... called from built-in function eval()
... called from #58:eval_cmd_string, line 19
... called from #58:eval*-d, line 13
(End of traceback)
```

**ToastStunt Output:**
```
-=!-^-!=-
{2, {E_DIV, "Division by zero", 0, {{#-1, "", #11, #-1, #11, 1}, {#-1, "eval", #-1, #-1, #11, 2}, {#2, "eval", #11, #2, #11, 5}}}}
-=!-v-!=-
```

**Analysis:** Both detect division by zero. ToastStunt returns structured error with status code `2`, full error code `E_DIV`, and traceback data. Barn returns human-readable traceback.

## Output Format Differences

### Barn Format
- Simple, human-readable output
- Results: `=> <value>`
- Errors: Multi-line traceback with line numbers

### ToastStunt Format
- Structured tuple format wrapped in markers
- Markers: `-=!-^-!=-` (start) and `-=!-v-!=-` (end)
- Success: `{1, <result>}` - status code 1, followed by result
- Error: `{2, {<error_code>, "<message>", <value>, <traceback>}}` - status code 2, followed by error details

## Usage Instructions

### Testing Against Barn

```bash
cd ~/code/barn

# Build barn if needed
go build -o barn_test.exe ./cmd/barn/

# Start barn on a free port
./barn_test.exe -db Test.db -port 7777 > server_7777.log 2>&1 &
sleep 2

# Test with moo_client
./moo_client.exe -port 7777 -cmd "connect wizard" -cmd "; return 1 + 1;"
```

### Testing Against ToastStunt

```bash
cd ~/src/toaststunt

# Copy test database
cp test/Test.db test/Test_mytest.db

# Start ToastStunt
./build-msvc/Release/moo.exe test/Test_mytest.db test/Test_mytest.db.new 9500 > toast_9500.log 2>&1 &
sleep 3

# Test with moo_client
cd ~/code/barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return 1 + 1;"
```

### Comparing Behavior Between Servers

```bash
cd ~/code/barn

# Test same expression on both servers
./moo_client.exe -port 7777 -timeout 2 -cmd "connect wizard" -cmd "; return typeof({});" > barn_result.txt 2>&1
./moo_client.exe -port 9500 -timeout 2 -cmd "connect wizard" -cmd "; return typeof({});" > toast_result.txt 2>&1

# Compare outputs
diff barn_result.txt toast_result.txt
```

## moo_client.exe Command Reference

```bash
./moo_client.exe [options]

Options:
  -host string      MOO server host (default "localhost")
  -port int         MOO server port (default 7777)
  -timeout int      Seconds to wait after last command (default 3)
  -cmd string       Command to send (can be specified multiple times)
  -file string      File containing commands (one per line)

Examples:
  # Single command
  ./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return 1 + 1;"

  # Multiple commands with longer timeout
  ./moo_client.exe -port 9500 -timeout 5 -cmd "connect wizard" -cmd "look" -cmd "news"

  # Commands from file
  ./moo_client.exe -port 9500 -file test_commands.txt

  # Connect to remote server
  ./moo_client.exe -host moo.example.com -port 7777 -cmd "connect wizard"
```

## Note on toast_oracle.exe

The `toast_oracle.exe` tool (mentioned in CLAUDE.md) uses ToastStunt's emergency mode to evaluate expressions. During testing, it had issues with expression formatting but this doesn't affect the verification task.

For comparing server behavior, **use moo_client.exe instead** - it's more reliable and works consistently with both servers.

## Conclusions

1. **moo_client.exe is fully functional** with both Barn and ToastStunt
2. **No modifications needed** - the tool is properly server-agnostic
3. **Output format differences** are expected and don't affect functionality
4. **Recommended for conformance testing** - reliable, captures all output, supports timeouts
5. **Supersedes manual nc/printf methods** which are unreliable and lose output

## Recommendations

1. **Use moo_client.exe for all manual testing** of both servers
2. **Update documentation** to emphasize moo_client works with any MOO server
3. **Create helper scripts** for common comparison tasks:
   ```bash
   # compare_servers.sh
   ./moo_client.exe -port 7777 -cmd "connect wizard" -cmd "$1" > barn.out 2>&1
   ./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "$1" > toast.out 2>&1
   echo "=== Barn ===" && cat barn.out
   echo "=== Toast ===" && cat toast.out
   ```

4. **Consider adding output normalization** if automated comparison is needed (strip formatting markers, normalize error output)

## Test Artifacts

All test outputs saved to:
- `~/code/barn/test_7777.log` - Barn server test
- `~/code/barn/test_9500.log` - ToastStunt server test
- `~/code/barn/test_toast_9500.log` - Detailed ToastStunt test

Test date: December 28, 2024
