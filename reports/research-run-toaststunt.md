# Research Report: Running ToastStunt with toastcore.db

## Summary

ToastStunt is already built and ready to use. The Windows executable and all dependencies are available in the build directory.

## Binary Location

**Primary executable:**
```
C:/Users/Q/src/toaststunt/build-win/Release/moo.exe
```

**Version:** ToastStunt 2.7.3_2 (64-bit)

**Required DLLs** (already present in same directory):
- `argon2.dll` (32 KB) - Argon2id password hashing
- `nettle-8.dll` (295 KB) - Cryptographic library

## Database Files

ToastStunt includes three database files:

| Database | Size | Description |
|----------|------|-------------|
| `toastcore.db` | 2.0 MB | Full-featured MOO database with admin tools |
| `mongoose.db` | 35 MB | Large production database |
| `Minimal.db` | 485 bytes | Minimal bootstrap database |

**Location:** `C:/Users/Q/src/toaststunt/`

**ToastCore source:** https://github.com/lisdude/toastcore

## Command-Line Usage

### Basic Syntax

```bash
moo.exe [-options] input-db-file output-db-file [-p port-number]
```

### Running ToastStunt with toastcore.db

**Recommended command (run from ToastStunt directory):**

```bash
cd ~/src/toaststunt
./build-win/Release/moo.exe toastcore.db toastcore.db.new -p 9999
```

**Alternative with absolute paths (from any directory):**

```bash
~/src/toaststunt/build-win/Release/moo.exe \
  ~/src/toaststunt/toastcore.db \
  ~/src/toaststunt/toastcore.db.new \
  -p 9999
```

**With logging:**

```bash
cd ~/src/toaststunt
./build-win/Release/moo.exe -l server_9999.log toastcore.db toastcore.db.new -p 9999
```

**Background mode:**

```bash
cd ~/src/toaststunt
./build-win/Release/moo.exe toastcore.db toastcore.db.new -p 9999 > server.log 2>&1 &
```

### Important Command-Line Options

| Option | Description |
|--------|-------------|
| `-p PORT` | Port to listen on (can be used multiple times) |
| `-l FILE` | Redirect output to log file |
| `-e` | Emergency wizard mode |
| `-o` | Enable outbound network connections |
| `-O` | Disable outbound network connections |
| `-f SCRIPT` | Load script file and pass to `#0:do_start_script()` |
| `-c LINE` | Pass line to `#0:do_start_script()` |
| `--no-ipv6` | Don't listen on IPv6 for default ports |
| `-v` | Show version |
| `-h` | Show help |

### Port Selection

- **Default port:** 7777 (if no `-p` specified)
- **Recommended test ports:** 9999, 9998, 9997 (avoid conflicts with other servers)
- **Multiple ports:** Use `-p` multiple times: `-p 9999 -p 9998`

## Startup Output

When started successfully, ToastStunt displays:

```
Dec 28 18:05:25:  _   __           _____                ______
Dec 28 18:05:25: ( `^` ))  ___________  /_____  _________ __  /_
Dec 28 18:05:25: |     ||   __  ___/_  __/_  / / /__  __ \_  __/
Dec 28 18:05:25: |     ||   _(__  ) / /_  / /_/ / _  / / // /_
Dec 28 18:05:25: '-----'`   /____/  \__,_/  /_/ /_/ \__/   v2.7.3_2
Dec 28 18:05:25:
Dec 28 18:05:25: STARTING: Version 2.7.3_2 (64-bit) of the ToastStunt/LambdaMOO server
Dec 28 18:05:25: LOADING: toastcore.db
Dec 28 18:05:25: LOADING: Reading 128 objects ...
Dec 28 18:05:25: LOADING: Reading 1949 MOO verb programs ...
```

## Testing Connection

Once ToastStunt is running, test connection with:

```bash
# Using Barn's moo_client tool
~/code/barn/moo_client.exe -port 9999 -cmd "connect wizard" -cmd "; return 1 + 1;"

# Using netcat (less reliable)
echo "; return 1 + 1;" | nc localhost 9999

# Using telnet
telnet localhost 9999
```

## Database Files Explained

### Input vs Output Database

ToastStunt requires TWO database file arguments:

1. **Input database** - The database to load (read-only during startup)
2. **Output database** - Where to save changes (written periodically)

**Common pattern:** Use same base name with `.new` suffix:
```bash
moo.exe toastcore.db toastcore.db.new -p 9999
```

### What Happens

- Server loads `toastcore.db` at startup
- Runs normally, accepting connections
- Periodically saves changes to `toastcore.db.new`
- On clean shutdown, can rename `.new` to replace original

**Note:** The `.new` file will be created/overwritten. The original input database remains unchanged.

## Gotchas and Requirements

### 1. DLL Dependencies

The executable requires two DLLs in the same directory:
- `argon2.dll` - Already present
- `nettle-8.dll` - Already present

**If you move moo.exe, copy these DLLs too.**

### 2. Working Directory

Best practice: Run from the ToastStunt directory where databases are located:

```bash
cd ~/src/toaststunt
./build-win/Release/moo.exe toastcore.db toastcore.db.new -p 9999
```

This ensures relative paths in the database work correctly.

### 3. Port Conflicts

If you see "Address already in use", the port is taken:
- Check for other running MOO servers: `ps aux | grep moo`
- Kill old processes: `taskkill /F /IM moo.exe` (Windows)
- Use a different port: `-p 9998` instead of `-p 9999`

### 4. Parser Warnings

ToastStunt may show warnings for unknown builtins:
```
PARSER: Warning in #32:valid:
           Line 1:  Unknown built-in function: spellcheck
```

This is normal - toastcore.db uses optional builtins that may not be compiled in.

### 5. HAProxy Detection

ToastStunt has HAProxy source IP rewriting enabled by default. If you see connection issues, this might interfere. See the README for how to disable if needed.

## Build Information

**Already built** - No rebuild needed unless you modify source code.

**Build location:** `~/src/toaststunt/build-win/`

**Build date:** Dec 25, 2024 (moo.exe timestamp: 17:31)

**If rebuild needed:**

```bash
cd ~/src/toaststunt
mkdir -p build-win
cd build-win
cmake ../ -G "Visual Studio 16 2019" -A x64
cmake --build . --config Release
```

**Build logs:**
- `~/src/toaststunt/build-win/final_build.log`
- `~/src/toaststunt/build.log`

## Integration with Barn Testing

### Using ToastStunt as Oracle

ToastStunt can serve as the reference implementation for Barn conformance testing:

1. **Start ToastStunt on port 9999:**
   ```bash
   cd ~/src/toaststunt
   ./build-win/Release/moo.exe toastcore.db toastcore.db.new -p 9999 > toast_server.log 2>&1 &
   ```

2. **Test Barn against ToastStunt behavior:**
   ```bash
   # Compare outputs
   ~/code/barn/moo_client.exe -port 9999 -cmd "connect wizard" -cmd "; return toint(\"abc\");"
   ```

3. **Automated conformance testing:**
   ```bash
   cd ~/code/cow_py
   # Test against ToastStunt
   uv run pytest tests/conformance/ --transport socket --moo-port 9999 -v
   ```

### toast_oracle Tool

Barn already has a `toast_oracle.exe` tool that wraps ToastStunt for testing individual expressions. This report provides the foundation for understanding how that tool works.

## Quick Reference Card

```bash
# Start ToastStunt with toastcore.db on port 9999
cd ~/src/toaststunt
./build-win/Release/moo.exe toastcore.db toastcore.db.new -p 9999

# Start with logging
./build-win/Release/moo.exe -l server.log toastcore.db toastcore.db.new -p 9999

# Start in background
./build-win/Release/moo.exe toastcore.db toastcore.db.new -p 9999 > server.log 2>&1 &

# Test connection
~/code/barn/moo_client.exe -port 9999 -cmd "connect wizard" -cmd "; return 1 + 1;"

# Stop server (Ctrl+C or kill process)
taskkill /F /IM moo.exe

# Show version
./build-win/Release/moo.exe -v

# Show help
./build-win/Release/moo.exe -h
```

## Verification Test

To verify everything works:

```bash
# Terminal 1: Start server
cd ~/src/toaststunt
./build-win/Release/moo.exe toastcore.db toastcore.db.new -p 9999

# Terminal 2: Test connection (after server starts)
~/code/barn/moo_client.exe -port 9999 -cmd "connect wizard" -cmd "; return 2 + 2;"
# Expected output: => 4
```

## Conclusion

ToastStunt is ready to use immediately:
- **Binary:** `~/src/toaststunt/build-win/Release/moo.exe`
- **Database:** `~/src/toaststunt/toastcore.db`
- **Command:** `cd ~/src/toaststunt && ./build-win/Release/moo.exe toastcore.db toastcore.db.new -p 9999`

No build or setup required. Just run and connect.
