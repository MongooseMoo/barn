# Task: Build MOO Client Testing Tool

## Context
Barn is a MOO server. We need a CLI tool to send commands and capture responses for testing/debugging.

## Objective
Create `cmd/moo_client/main.go` - a simple CLI tool that:
1. Connects to a MOO server on specified host:port
2. Sends a sequence of commands from a file or command line args
3. Captures and prints all server output
4. Disconnects cleanly after a timeout or when done

## Requirements

### Usage
```bash
# Send commands from args
./moo_client -port 9950 -cmd "connect wizard" -cmd "look" -cmd "; return 1 + 1;"

# Send commands from file
./moo_client -port 9950 -file commands.txt

# With custom timeout (default 3 seconds after last command)
./moo_client -port 9950 -timeout 5 -cmd "connect wizard"
```

### Behavior
1. Connect to localhost:port (default 7777)
2. Start goroutine to read and print all server output
3. Send each command with \n terminator, with small delay between commands (100ms)
4. Wait for timeout after last command to collect output
5. Close connection and exit

### Output
- Print all server output to stdout
- Print connection status to stderr
- Exit 0 on success, 1 on connection error

## Files to Create
- `cmd/moo_client/main.go`

## Test
After building, test with:
```bash
go build -o moo_client.exe ./cmd/moo_client/
./moo_client.exe -port 9950 -cmd "connect wizard" -cmd "; return 1 + 1;"
```

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
