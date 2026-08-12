package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func mustMapValue(t *testing.T, value types.Value) types.Value {
	t.Helper()
	if value.Type() != types.TYPE_MAP {
		t.Fatalf("expected map value, got %T", value)
	}
	return value
}

func mustStringAt(t *testing.T, m types.Value, key string) string {
	t.Helper()
	value, ok := m.MapGet(types.NewStr(key))
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	if value.Type() != types.TYPE_STR {
		t.Fatalf("expected string at %q, got %T", key, value)
	}
	return value.Str()
}

func mustIntAt(t *testing.T, m types.Value, key string) int64 {
	t.Helper()
	value, ok := m.MapGet(types.NewStr(key))
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	if value.Type() != types.TYPE_INT {
		t.Fatalf("expected int at %q, got %T", key, value)
	}
	return value.Int()
}

func TestParseHTTPRequestContentLength(t *testing.T) {
	value, _, complete := parseHTTPMessage("request", []byte("GET /hello HTTP/1.1\r\nContent-Length: 5\r\n\r\nHELLO"))
	if !complete {
		t.Fatal("expected complete request parse")
	}

	result := mustMapValue(t, value)
	if got := mustStringAt(t, result, "method"); got != "GET" {
		t.Fatalf("got method %q", got)
	}
	if got := mustStringAt(t, result, "uri"); got != "/hello" {
		t.Fatalf("got uri %q", got)
	}
	if got := mustStringAt(t, result, "body"); got != "HELLO" {
		t.Fatalf("got body %q", got)
	}
	headersValue, ok := result.MapGet(types.NewStr("headers"))
	if !ok {
		t.Fatal("missing headers")
	}
	headers := mustMapValue(t, headersValue)
	if got := mustStringAt(t, headers, "Content-Length"); got != "5" {
		t.Fatalf("got content-length %q", got)
	}
}

func TestParseHTTPRequestWithoutVersion(t *testing.T) {
	value, _, complete := parseHTTPMessage("request", []byte("GET /legacy\r\nfoo: bar\r\n\r\n"))
	if !complete {
		t.Fatal("expected complete request parse")
	}

	result := mustMapValue(t, value)
	if got := mustStringAt(t, result, "method"); got != "GET" {
		t.Fatalf("got method %q", got)
	}
	if got := mustStringAt(t, result, "uri"); got != "/legacy" {
		t.Fatalf("got uri %q", got)
	}
	headersValue, ok := result.MapGet(types.NewStr("headers"))
	if !ok {
		t.Fatal("missing headers")
	}
	headers := mustMapValue(t, headersValue)
	if got := mustStringAt(t, headers, "foo"); got != "bar" {
		t.Fatalf("got foo header %q", got)
	}
	if _, ok := result.MapGet(types.NewStr("version")); ok {
		t.Fatal("did not expect version key")
	}
}

func TestParseHTTPRequestFoldedHeaders(t *testing.T) {
	data := []byte("GET / HTTP/1.1\r\nLine1:   abc\r\n\tdef\r\n ghi\r\n\t\tjkl\r\n  mno \r\n\t \tqrs\r\nLine2: \t line2\t\r\n\r\n")
	value, _, complete := parseHTTPMessage("request", data)
	if !complete {
		t.Fatal("expected complete request parse")
	}

	headersValue, ok := mustMapValue(t, value).MapGet(types.NewStr("headers"))
	if !ok {
		t.Fatal("missing headers")
	}
	headers := mustMapValue(t, headersValue)
	if got := mustStringAt(t, headers, "Line1"); got != "abcdefghijklmno qrs" {
		t.Fatalf("got Line1 %q", got)
	}
	if got := mustStringAt(t, headers, "Line2"); got != "line2~09" {
		t.Fatalf("got Line2 %q", got)
	}
}

func TestParseHTTPResponseChunked(t *testing.T) {
	data := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nTransfer-Encoding: chunked\r\n\r\n25  \r\nThis is the data in the first chunk\r\n\r\n1C\r\nand this is the second one\r\n\r\n0  \r\n\r\n")
	value, _, complete := parseHTTPMessage("response", data)
	if !complete {
		t.Fatal("expected complete response parse")
	}

	result := mustMapValue(t, value)
	if got := mustIntAt(t, result, "status"); got != 200 {
		t.Fatalf("got status %d", got)
	}
	if got := mustStringAt(t, result, "body"); got != "This is the data in the first chunk~0D~0Aand this is the second one~0D~0A" {
		t.Fatalf("got body %q", got)
	}
}

func TestParseHTTPChunkedBodyRejectsOverflowingSize(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "maximum int64",
			data: "7FFFFFFFFFFFFFFF\r\nAAAA\r\n0\r\n\r\n",
		},
		{
			name: "larger than int64",
			data: "FFFFFFFFFFFFFFFF\r\nAAAA\r\n0\r\n\r\n",
		},
		{
			name: "one byte beyond available data",
			data: "5\r\nAAAA\r\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if body, consumed, complete := parseHTTPChunkedBody([]byte(test.data), 0); complete {
				t.Fatalf("unexpected complete parse: body %q, consumed %d", body, consumed)
			}
		})
	}
}

func FuzzParseHTTPChunkedBody(f *testing.F) {
	f.Add([]byte("7FFFFFFFFFFFFFFF\r\nAAAA\r\n0\r\n\r\n"))
	f.Add([]byte("4\r\nWiki\r\n0\r\n\r\n"))
	f.Add([]byte("FFFFFFFFFFFFFFFF\r\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		parseHTTPChunkedBody(data, 0)
	})
}

func TestPrepareHTTPReadReturnsZeroAfterInvalidBinaryInput(t *testing.T) {
	player := types.ObjID(7)
	r := NewRegistry()

	r.setConnectionOption(player, "hold-input", types.NewInt(1))
	if handled, _ := r.HandleHeldInput(player, "~ZZ~!!", false); !handled {
		t.Fatal("expected held input to be intercepted")
	}

	readTask := task.NewTask(99, player, 1000, 5)
	value, complete := r.prepareHTTPRead(player, "request", readTask)
	if !complete {
		t.Fatal("expected read to complete immediately")
	}

	if value.Type() != types.TYPE_INT {
		t.Fatalf("expected int result, got %T", value)
	}
	if value.Int() != 0 {
		t.Fatalf("got %d, want 0", value.Int())
	}
}

func TestCloseHeldHTTPInputKillsPendingReadTask(t *testing.T) {
	player := types.ObjID(8)
	r := NewRegistry()

	pending := task.NewTask(101, player, 1000, 5)
	if value, complete := r.prepareHTTPRead(player, "request", pending); complete {
		t.Fatalf("expected incomplete request to suspend, got %v", value)
	}
	task.NewManager().SuspendTask(pending, -1)

	r.CloseHeldHTTPInput(player)

	if got := pending.GetState(); got != task.TaskKilled {
		t.Fatalf("disconnected HTTP read state = %s, want killed", got)
	}
}

func TestKilledHTTPReadClearsBufferAndAllowsFreshParse(t *testing.T) {
	player := types.ObjID(8)
	r := NewRegistry()

	r.setConnectionOption(player, "hold-input", types.NewInt(1))
	if handled, _ := r.HandleHeldInput(player, "GET /1~0D~0Aone: two~0D~0A", false); !handled {
		t.Fatal("expected partial input to be intercepted")
	}

	stalled := task.NewTask(101, player, 1000, 5)
	if value, complete := r.prepareHTTPRead(player, "request", stalled); complete {
		t.Fatalf("expected partial request to suspend, got %v", value)
	}
	task.NewManager().SuspendTask(stalled, -1)
	r.CancelHTTPReadTask(stalled.ID)

	if handled, _ := r.HandleHeldInput(player, "GET /2~0D~0Afoo: bar~0D~0A~0D~0A", false); !handled {
		t.Fatal("expected fresh input to be intercepted")
	}

	fresh := task.NewTask(102, player, 1000, 5)
	value, complete := r.prepareHTTPRead(player, "request", fresh)
	if !complete {
		t.Fatal("expected fresh request to parse immediately")
	}

	result := mustMapValue(t, value)
	if got := mustStringAt(t, result, "method"); got != "GET" {
		t.Fatalf("got method %q", got)
	}
	if got := mustStringAt(t, result, "uri"); got != "/2" {
		t.Fatalf("got uri %q", got)
	}
	headersValue, ok := result.MapGet(types.NewStr("headers"))
	if !ok {
		t.Fatal("missing headers")
	}
	headers := mustMapValue(t, headersValue)
	if got := mustStringAt(t, headers, "foo"); got != "bar" {
		t.Fatalf("got foo header %q", got)
	}
}
