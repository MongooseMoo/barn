# MOO Network Built-ins

## Overview

Functions for network operations including HTTP, connections, and DNS.

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

### 1.3 connection_info() [Not Implemented]

**Signature:** `connection_info(player) → MAP`

> **Note:** This function is documented but not implemented in ToastStunt or Barn.

**Description:** Returns detailed connection information.

**Returns:**
```moo
["ip" -> "192.168.1.1",
 "port" -> 1234,
 "connected_at" -> 1703419200,
 "last_input" -> 1703419300,
 "bytes_in" -> 1024,
 "bytes_out" -> 2048]
```

---

### 1.4 boot_player

**Signature:** `boot_player(player) → none`

**Description:** Disconnects a player.

**Wizard only.**

---

## 2. Output

### 2.1 notify

**Signature:** `notify(player, message [, no_flush]) → none`

**Description:** Sends message to player's connection.

**Parameters:**
- `player`: Target player object
- `message`: Text to send
- `no_flush`: If true, buffer instead of immediate send

**Examples:**
```moo
notify(player, "Hello, world!");
```

---

### 2.2 notify_list (ToastStunt)

**Signature:** `notify_list(player, lines) → none`

**Description:** Sends multiple lines efficiently.

**Examples:**
```moo
notify_list(player, {"Line 1", "Line 2", "Line 3"});
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

### 4.1 curl() [Not Implemented]

**Signature:** `curl(url [, options]) → LIST`

> **Note:** This function is documented but not implemented in ToastStunt or Barn.

**Description:** Makes HTTP request.

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

## 5. DNS

### 5.1 dns_lookup (ToastStunt)

**Signature:** `dns_lookup(hostname) → LIST`

**Description:** Resolves hostname to IP addresses.

**Returns:** List of IP address strings.

**Examples:**
```moo
dns_lookup("example.com")
// => {"93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946"}
```

---

### 5.2 reverse_dns (ToastStunt)

**Signature:** `reverse_dns(ip_address) → STR`

**Description:** Reverse DNS lookup.

**Examples:**
```moo
reverse_dns("8.8.8.8")   => "dns.google"
```

---

## 6. Connection Management

### 6.1 listen

**Signature:** `listen(object, point [, print_messages]) → none`

**Description:** Sets up listening point for connections.

**Parameters:**
- `object`: Handler object
- `point`: Port number or descriptor
- `print_messages`: Show connection messages

**Wizard only.**

---

### 6.2 unlisten

**Signature:** `unlisten(point) → none`

**Description:** Stops listening on point.

---

### 6.3 listeners

**Signature:** `listeners() → LIST`

**Description:** Returns active listening points.

---

### 6.4 open_network_connection

**Signature:** `open_network_connection(host, port [, listener]) → OBJ`

**Description:** Opens outbound connection.

**Returns:** Connection object.

**Wizard only (usually).**

---

## 7. Binary Protocol

### 7.1 set_connection_option

**Signature:** `set_connection_option(player, option, value) → none`

**Description:** Sets connection option.

**Options:**
| Option | Values | Description |
|--------|--------|-------------|
| "hold-input" | 0/1 | Buffer input |
| "disable-oob" | 0/1 | Disable out-of-band |
| "binary" | 0/1 | Binary mode |

---

### 7.2 connection_options

**Signature:** `connection_options(player) → MAP`

**Description:** Returns current connection options.

---

## 8. Timeouts

### 8.1 idle_seconds

**Signature:** `idle_seconds(player) → INT`

**Description:** Returns seconds since last input.

---

### 8.2 connected_seconds

**Signature:** `connected_seconds(player) → INT`

**Description:** Returns connection duration.

---

### 8.3 set_connection_timeout (ToastStunt)

**Signature:** `set_connection_timeout(player, seconds) → none`

**Description:** Sets idle timeout for connection.

---

## 9. Error Handling

| Error | Condition |
|-------|-----------|
| E_INVARG | Invalid player/host |
| E_PERM | Permission denied |
| E_FILE | Network error |
| E_ARGS | Wrong arguments |

---

## 10. Go Implementation Notes

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
