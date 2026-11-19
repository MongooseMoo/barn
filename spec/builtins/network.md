# MOO Network Built-ins

## Overview

Functions for network operations including connections, HTTP client, and connection management.

---

## 1. Player Connections

### 1.1 connected_players

**Signature:** `connected_players([include_queued]) → LIST`

**Description:** Returns list of connected player objects.

**Examples:**
```moo
players = connected_players();
// => {#player1, #player2, ...}
```

---

### 1.2 connection_name

**Signature:** `connection_name(player [, method]) → STR`

**Description:** Returns connection identifier.

**Methods:**
| Method | Returns |
|--------|---------|
| "legacy" | "hostname, port" |
| "ip-address" | IP address only |
| "hostname" | Resolved hostname |

**Examples:**
```moo
connection_name(player)              => "192.168.1.1, port 1234"
connection_name(player, "ip-address") => "192.168.1.1"
```

---

### 1.3 connection_info

**Signature:** `connection_info(player) → MAP`

**Description:** Returns detailed connection information.

**Returns:** Map with connection details:
```moo
["ip" -> "192.168.1.1",
 "port" -> 1234,
 "connected_at" -> 1703419200,
 "last_input" -> 1703419300,
 "bytes_in" -> 1024,
 "bytes_out" -> 2048]
```

**Errors:**
- E_INVARG: Player not connected

---

### 1.4 boot_player

**Signature:** `boot_player(player) → none`

**Description:** Disconnects a player.

**Wizard only.**

---

## 2. Output

### 2.1 notify

**Signature:** `notify(conn, message [, no_flush [, no_newline]]) → INT`

**Description:** Queues `message` on the connection. Returns 1 when queued (and
1 when `conn` has no connection); returns 0 only when `no_flush` is true and
the line would overflow `max_queued_output` behind already-queued output. A
caller that is neither a wizard nor `conn` gets `E_PERM`. Full semantics:
[server.md §5.4](server.md#54-notify).

**Examples:**
```moo
notify(player, "Hello, world!")   => 1
```

---

## 3. Input

### 3.1 read

**Signature:** `read([player [, non_blocking]]) → STR`

**Description:** Reads line of input from player.

**Behavior:**
- Suspends task until input received
- Returns input string

**Examples:**
```moo
notify(player, "Enter your name: ");
name = read(player);
```

**Errors:**
- E_INVARG: Not a connected player

---

### 3.2 force_input (ToastStunt)

**Signature:** `force_input(player, line [, at_front]) → none`

**Description:** Queues input as if player typed it.

**Wizard only.**

---

### 3.3 flush_input (ToastStunt)

**Signature:** `flush_input(player) → none`

**Description:** Discards queued input.

---

## 4. HTTP Client (ToastStunt)

### 4.1 curl

**Signature:** `curl(url [, options [, timeout]]) → LIST`

**Description:** Makes HTTP request.

**Note:** This function is defined in ToastStunt source code but may not be available at runtime depending on build configuration. Check if enabled before use.

**Options map:**
| Key | Type | Description |
|-----|------|-------------|
| "method" | STR | "GET", "POST", etc. |
| "headers" | MAP | Request headers |
| "body" | STR | Request body |
| "timeout" | INT | Timeout seconds |
| "follow" | BOOL | Follow redirects |

**Returns:** `{status_code, headers, body}`

**Examples:**
```moo
result = curl("https://api.example.com/data");
{status, headers, body} = result;

result = curl("https://api.example.com/post", [
    "method" -> "POST",
    "headers" -> ["Content-Type" -> "application/json"],
    "body" -> generate_json(data)
]);
```

**Errors:**
- E_INVARG: Invalid URL
- E_FILE: Network error

---

## 5. Connection Management

### 5.1 listen

**Signature:** `listen(object, point [, options]) → ANY`

**Description:** Sets up a listening point for connections and returns its
canonical descriptor. `options` is a map (`"print-messages"`, `"ipv6"`,
`"interface"`, `"tls"`, `"certificate"`, `"key"`; keys matched without regard to
case). Full semantics: [server.md §6.2](server.md#62-listen).

**Wizard only.**

---

### 5.2 unlisten

**Signature:** `unlisten(point) → none`

**Description:** Stops listening on point.

---

### 5.3 listeners

**Signature:** `listeners() → LIST`

**Description:** Returns active listening points.

---

### 5.4 open_network_connection

**Signature:** `open_network_connection(host, port [, options]) → OBJ`

**Description:** Opens an outbound connection and returns its negative
connection object. `options` is a map (`"ipv6"`, `"listener"`, `"tls"`,
`"tls_verify"`). Connect failures raise `E_INVARG` with `"Connection failure"`,
`"getaddrinfo error"` or `"TLS connection failed"`. Full semantics:
[server.md §6.1](server.md#61-open_network_connection).

**Wizard only.**

---

## 6. Binary Protocol

### 6.1 set_connection_option

**Signature:** `set_connection_option(player, option, value) → none`

**Description:** Sets connection option. A caller that is neither a wizard
nor `player` gets `E_PERM`; the permission check comes before the connection
lookup, so it applies even when `player` is not connected.

**Options:**
| Option | Values | Description |
|--------|--------|-------------|
| "hold-input" | 0/1 | Buffer input |
| "disable-oob" | 0/1 | Disable out-of-band |
| "binary" | 0/1 | Binary mode |

---

### 6.2 connection_options

**Signature:** `connection_options(player) → MAP`

**Description:** Returns current connection options.

---

## 7. Timeouts

### 7.1 idle_seconds

**Signature:** `idle_seconds(player) → INT`

**Description:** Returns seconds since last input.

---

### 7.2 connected_seconds

**Signature:** `connected_seconds(player) → INT`

**Description:** Returns connection duration.

---

## 8. Error Handling

| Error | Condition |
|-------|-----------|
| E_INVARG | Invalid player/host |
| E_PERM | Permission denied |
| E_FILE | Network error |
| E_ARGS | Wrong arguments |

---

## 9. Go Implementation Notes

```go
import "net/http"

func builtinNotify(args []Value) (Value, error) {
    playerID := int64(args[0].(ObjValue))
    message := string(args[1].(StringValue))

    conn := connections.Get(playerID)
    if conn == nil {
        return nil, E_INVARG
    }

    noFlush := false
    if len(args) > 2 {
        noFlush = isTruthy(args[2])
    }

    if noFlush {
        conn.Buffer(message + "\n")
    } else {
        conn.Send(message + "\n")
    }

    return nil, nil
}

func builtinCurl(args []Value) (Value, error) {
    url := string(args[0].(StringValue))

    method := "GET"
    var body io.Reader
    headers := make(http.Header)
    timeout := 30 * time.Second

    if len(args) > 1 {
        opts := args[1].(*MOOMap)
        if m := opts.Get("method"); m != nil {
            method = string(m.(StringValue))
        }
        if h := opts.Get("headers"); h != nil {
            for k, v := range h.(*MOOMap).entries {
                headers.Set(k.String(), v.String())
            }
        }
        if b := opts.Get("body"); b != nil {
            body = strings.NewReader(string(b.(StringValue)))
        }
        if t := opts.Get("timeout"); t != nil {
            timeout = time.Duration(int(t.(IntValue))) * time.Second
        }
    }

    client := &http.Client{Timeout: timeout}
    req, err := http.NewRequest(method, url, body)
    if err != nil {
        return nil, E_INVARG
    }
    req.Header = headers

    resp, err := client.Do(req)
    if err != nil {
        return nil, E_FILE
    }
    defer resp.Body.Close()

    respBody, _ := io.ReadAll(resp.Body)

    respHeaders := NewMOOMap()
    for k, v := range resp.Header {
        respHeaders.Set(StringValue(k), StringValue(strings.Join(v, ", ")))
    }

    return &MOOList{data: []Value{
        IntValue(resp.StatusCode),
        respHeaders,
        StringValue(string(respBody)),
    }}, nil
}
```
