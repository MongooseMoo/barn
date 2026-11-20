package builtins

import (
	"strconv"
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func newReadHTTPTestExecution(t *testing.T, owner, programmer types.ObjID, wizard bool, taskID, lastInputTaskID int64) *Execution {
	t.Helper()
	store := dbstore.NewStore()
	addTxnObject(t, store, 0, 0)
	addTxnObject(t, store, programmer, programmer)
	if owner != programmer {
		addTxnObject(t, store, owner, owner)
	}
	connection := types.ObjID(7)
	addTxnObject(t, store, connection, owner)

	manager := taskManagerStub{}
	session := NewSession(NewRegistry(), NoHost())
	configureTestHost(session, func(host *Host) {
		host.ConnManager = &stubConnManager{conn: &stubConn{id: 7, lastInputTaskID: lastInputTaskID}}
		host.TaskManager = manager
	})
	taskValue := task.NewState(taskID, connection, 1000, 5)
	ctx := session.NewExecution(&task.Context{
		Player:     connection,
		Programmer: programmer,
		IsWizard:   wizard,
		Store:      store,
	}, taskValue)
	return ctx
}

func TestReadHTTPExplicitConnectionAllowsOwner(t *testing.T) {
	ctx := newReadHTTPTestExecution(t, 2, 2, false, 41, 0)

	result := builtinReadHTTP(ctx, []types.Value{types.NewStr("request"), types.NewObj(7)})

	if result.Flow != types.FlowSuspend {
		t.Fatalf("read_http result = %#v, want suspend", result)
	}
	if ctx.Task.GetState() != task.TaskSuspended || !ctx.Session.HasPendingHTTPRead(7) {
		t.Fatal("authorized read_http did not install and suspend its reader")
	}
}

func TestReadHTTPExplicitConnectionDeniesUnrelatedProgrammerWithoutSideEffects(t *testing.T) {
	ctx := newReadHTTPTestExecution(t, 2, 3, false, 42, 0)

	result := builtinReadHTTP(ctx, []types.Value{types.NewStr("request"), types.NewObj(7)})

	if !result.IsError() || result.Error != types.E_PERM {
		t.Fatalf("read_http result = %#v, want E_PERM", result)
	}
	if ctx.Task.GetState() == task.TaskSuspended || ctx.Session.HasPendingHTTPRead(7) || ctx.Task.IsHTTPReadSuspended {
		t.Fatal("denied read_http installed or suspended a reader")
	}
}

func TestReadHTTPImplicitConnectionAllowsWizardLastInputTask(t *testing.T) {
	ctx := newReadHTTPTestExecution(t, 2, 2, true, 43, 43)

	result := builtinReadHTTP(ctx, []types.Value{types.NewStr("request")})

	if result.Flow != types.FlowSuspend {
		t.Fatalf("read_http result = %#v, want suspend", result)
	}
	if ctx.Task.GetState() != task.TaskSuspended || !ctx.Session.HasPendingHTTPRead(7) {
		t.Fatal("authorized read_http did not install and suspend its reader")
	}
}

func TestReadHTTPImplicitConnectionDeniesUnrelatedWizardTaskWithoutSideEffects(t *testing.T) {
	ctx := newReadHTTPTestExecution(t, 2, 2, true, 44, 99)

	result := builtinReadHTTP(ctx, []types.Value{types.NewStr("request")})

	if !result.IsError() || result.Error != types.E_PERM {
		t.Fatalf("read_http result = %#v, want E_PERM", result)
	}
	if ctx.Task.GetState() == task.TaskSuspended || ctx.Session.HasPendingHTTPRead(7) || ctx.Task.IsHTTPReadSuspended {
		t.Fatal("denied read_http installed or suspended a reader")
	}
}

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

// Style bits for buildValidChunkedEncoding.
const (
	chunkStyleUpperHex      = 1 << iota // A-F instead of a-f in chunk sizes
	chunkStyleLeadingZeros              // "00" before every chunk size
	chunkStyleExtensions                // ";ext=v" after every data chunk size
	chunkStyleLastExtension             // ";last" after the zero-size line's size
	chunkStyleTrailer                   // one trailer field before the final CRLF
)

// buildValidChunkedEncoding encodes body as an HTTP/1.1 chunked body
// (RFC 9112 section 7.1). Each byte of cuts sets the next chunk's length to
// byte+1 (clamped to what is left); once cuts runs out the rest of body is one
// chunk. Only forms accepted by both parseHTTPChunkedBody and Toast's
// http_parser.c are produced: hex size, optional ";" extension, CRLF, data,
// CRLF, then "0", optional trailer fields, and the final CRLF.
func buildValidChunkedEncoding(body, cuts []byte, style uint8) []byte {
	sizeText := func(n int) string {
		s := strconv.FormatInt(int64(n), 16)
		if style&chunkStyleUpperHex != 0 {
			s = strings.ToUpper(s)
		}
		if style&chunkStyleLeadingZeros != 0 {
			s = "00" + s
		}
		return s
	}

	var enc []byte
	rest := body
	for i := 0; len(rest) > 0; i++ {
		n := len(rest)
		if i < len(cuts) {
			n = min(int(cuts[i])+1, len(rest))
		}
		enc = append(enc, sizeText(n)...)
		if style&chunkStyleExtensions != 0 {
			enc = append(enc, ";ext=v"...)
		}
		enc = append(enc, "\r\n"...)
		enc = append(enc, rest[:n]...)
		enc = append(enc, "\r\n"...)
		rest = rest[n:]
	}
	enc = append(enc, sizeText(0)...)
	if style&chunkStyleLastExtension != 0 {
		enc = append(enc, ";last"...)
	}
	enc = append(enc, "\r\n"...)
	if style&chunkStyleTrailer != 0 {
		enc = append(enc, "Trailer-Field: value\r\n"...)
	}
	return append(enc, "\r\n"...)
}

// FuzzParseHTTPChunkedBodyValidEncoding checks decoding, not just termination:
// a valid chunked encoding placed after arbitrary bytes (start offset) and
// followed by arbitrary bytes (pipelined input) must decode to the original
// body, report the absolute end offset of the encoding as consumed, and be
// complete; every strict prefix of it must be reported not complete.
func FuzzParseHTTPChunkedBodyValidEncoding(f *testing.F) {
	f.Add([]byte(""), []byte("Wikipedia in\r\n\r\nchunks."), []byte{3, 4, 13}, []byte(""), uint8(0))
	f.Add([]byte("HTTP/1.1 200 OK\r\n\r\n"), []byte("hello"), []byte{}, []byte("GET / HTTP/1.1\r\n"), uint8(0x1f))
	f.Add([]byte("x"), []byte(""), []byte{}, []byte("0\r\n\r\n"), uint8(chunkStyleTrailer))
	f.Add([]byte(""), []byte("\x00\xff\r\n0\r\n\r\n~"), []byte{0, 0, 255}, []byte(""), uint8(chunkStyleExtensions|chunkStyleUpperHex))

	f.Fuzz(func(t *testing.T, prefix, body, cuts, suffix []byte, style uint8) {
		// The strict-prefix loop is quadratic in the encoding length.
		if len(body) > 1024 || len(prefix) > 64 || len(suffix) > 64 {
			return
		}
		enc := buildValidChunkedEncoding(body, cuts, style)
		data := make([]byte, 0, len(prefix)+len(enc)+len(suffix))
		data = append(data, prefix...)
		data = append(data, enc...)
		data = append(data, suffix...)
		start := len(prefix)
		end := start + len(enc)

		got, consumed, complete := parseHTTPChunkedBody(data, start)
		if !complete {
			t.Fatalf("valid encoding %q at offset %d not complete", enc, start)
		}
		if want := EncodeRawToBinary(body); got != want {
			t.Fatalf("encoding %q decoded to %q, want %q", enc, got, want)
		}
		if consumed != end {
			t.Fatalf("encoding %q at offset %d consumed %d, want %d", enc, start, consumed, end)
		}

		for n := start; n < end; n++ {
			if got, consumed, complete := parseHTTPChunkedBody(data[:n], start); complete {
				t.Fatalf("strict prefix %q of encoding %q reported complete: body %q, consumed %d", data[start:n], enc, got, consumed)
			}
		}
	})
}

func TestPrepareHTTPReadReturnsZeroAfterInvalidBinaryInput(t *testing.T) {
	player := types.ObjID(7)
	r := ctxWithConnManager(&stubConnManager{conn: &stubConn{id: 7}}).Session

	r.setConnectionOption(int64(player), nil, "hold-input", types.NewInt(1))
	if handled, _ := r.HandleHeldInput(player, int64(player), "~ZZ~!!", false); !handled {
		t.Fatal("expected held input to be intercepted")
	}

	readTask := task.NewState(99, player, 1000, 5)
	value, complete := r.prepareHTTPRead(player, int64(player), "request", readTask)
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
	r := NewSession(NewRegistry(), NoHost())

	pending := task.NewState(101, player, 1000, 5)
	if value, complete := r.prepareHTTPRead(player, int64(player), "request", pending); complete {
		t.Fatalf("expected incomplete request to suspend, got %v", value)
	}
	pending.SuspendIndefinite()

	r.CloseHeldHTTPInput(player)

	if got := pending.GetState(); got != task.TaskKilled {
		t.Fatalf("disconnected HTTP read state = %s, want killed", got)
	}
}

func TestKilledHTTPReadClearsBufferAndAllowsFreshParse(t *testing.T) {
	player := types.ObjID(8)
	r := ctxWithConnManager(&stubConnManager{conn: &stubConn{id: 8}}).Session

	r.setConnectionOption(int64(player), nil, "hold-input", types.NewInt(1))
	if handled, _ := r.HandleHeldInput(player, int64(player), "GET /1~0D~0Aone: two~0D~0A", false); !handled {
		t.Fatal("expected partial input to be intercepted")
	}

	stalled := task.NewState(101, player, 1000, 5)
	if value, complete := r.prepareHTTPRead(player, int64(player), "request", stalled); complete {
		t.Fatalf("expected partial request to suspend, got %v", value)
	}
	stalled.SuspendIndefinite()
	r.CancelHTTPReadTask(stalled.ID)

	if handled, _ := r.HandleHeldInput(player, int64(player), "GET /2~0D~0Afoo: bar~0D~0A~0D~0A", false); !handled {
		t.Fatal("expected fresh input to be intercepted")
	}

	fresh := task.NewState(102, player, 1000, 5)
	value, complete := r.prepareHTTPRead(player, int64(player), "request", fresh)
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
