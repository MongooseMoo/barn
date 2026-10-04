package builtins

import (
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestParseHTTPHeadersReturnPaths(t *testing.T) {
	for _, test := range []struct {
		name          string
		data          string
		start         int
		state         string
		bodyStart     int
		contentLength int
		chunked       bool
		headers       [][2]string
	}{
		{name: "empty complete", data: "\r\nBODY", state: "complete", bodyStart: 2, contentLength: -1},
		{name: "nonzero start", data: "prefix\r\nBODY", start: 6, state: "complete", bodyStart: 8, contentLength: -1},
		{name: "folded header", data: "X: a\r\n\tb\r\n c \r\n\r\nBODY", state: "complete", bodyStart: 17, contentLength: -1, headers: [][2]string{{"X", "abc "}}},
		{name: "binary value", data: "X: \x01\t\r\n\r\n", state: "complete", bodyStart: 9, contentLength: -1, headers: [][2]string{{"X", "~01~09"}}},
		{name: "zero length", data: "Content-Length: 0\r\n\r\n", state: "complete", bodyStart: 21, contentLength: 0, headers: [][2]string{{"Content-Length", "0"}}},
		{name: "length and chunked", data: "content-length: 4\r\nTransfer-Encoding: CHUNKED\r\n\r\n", state: "complete", bodyStart: 49, contentLength: 4, chunked: true, headers: [][2]string{{"content-length", "4"}, {"Transfer-Encoding", "CHUNKED"}}},
		{name: "invalid length ignored", data: "Content-Length: nope\r\n\r\n", state: "complete", bodyStart: 24, contentLength: -1, headers: [][2]string{{"Content-Length", "nope"}}},
		{name: "negative length ignored", data: "Content-Length: -1\r\n\r\n", state: "complete", bodyStart: 22, contentLength: -1, headers: [][2]string{{"Content-Length", "-1"}}},
		{name: "incomplete empty", state: "incomplete"},
		{name: "incomplete CRLF", data: "X: a\r", state: "incomplete"},
		{name: "incomplete blank line", data: "X: a\r\n\r", state: "incomplete"},
		{name: "incomplete folded line", data: "X: a\r\n b", state: "incomplete"},
		{name: "invalid first fold", data: " a\r\n\r\n", state: "invalid", bodyStart: 4},
		{name: "invalid missing colon", data: "X\r\n\r\n", state: "invalid", bodyStart: 3},
		{name: "invalid empty name", data: ": a\r\n\r\n", state: "invalid", bodyStart: 5},
		{name: "invalid name token", data: "Bad Name: a\r\n\r\n", state: "invalid", bodyStart: 13},
		{name: "invalid after valid header", data: "X: a\r\nBad Name: b\r\n\r\n", state: "invalid", bodyStart: 19},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := parseHTTPHeaders([]byte(test.data), test.start)
			wantState := map[string]httpHeaderParseState{
				"complete": httpHeadersComplete, "incomplete": httpHeadersIncomplete, "invalid": httpHeadersInvalid,
			}[test.state]
			if result.state != wantState || result.bodyStart != test.bodyStart || result.contentLength != test.contentLength || result.chunked != test.chunked {
				t.Fatalf("result=%+v, want state=%s bodyStart=%d contentLength=%d chunked=%v", result, test.state, test.bodyStart, test.contentLength, test.chunked)
			}
			var gotHeaders [][2]string
			for _, header := range result.headers {
				gotHeaders = append(gotHeaders, [2]string{header[0].Str(), header[1].Str()})
			}
			if !reflect.DeepEqual(gotHeaders, test.headers) {
				t.Fatalf("headers=%q, want %q", gotHeaders, test.headers)
			}
		})
	}
}

func TestParseHTTPMessageHeaderStatesAndConsumption(t *testing.T) {
	for _, kind := range []string{"request", "response"} {
		t.Run(kind, func(t *testing.T) {
			firstLine := "GET / HTTP/1.1\r\n"
			if kind == "response" {
				firstLine = "HTTP/1.1 200 OK\r\n"
			}
			for _, test := range []struct {
				name      string
				suffix    string
				consumed  int
				complete  bool
				errorCode string
				body      string
				hasBody   bool
			}{
				{name: "incomplete header", suffix: "X: a\r"},
				{name: "incomplete body", suffix: "Content-Length: 4\r\n\r\nabc"},
				{name: "invalid header", suffix: "Bad Name: a\r\nREST", consumed: 13, complete: true, errorCode: "INVALID_HEADER_TOKEN"},
				{name: "no body leaves suffix", suffix: "X: a\r\n\r\nREST", consumed: 8, complete: true},
				{name: "length body leaves suffix", suffix: "Content-Length: 4\r\n\r\nDATAREST", consumed: 25, complete: true, body: "DATA", hasBody: true},
				{name: "chunked body takes priority", suffix: "Content-Length: 99\r\nTransfer-Encoding: chunked\r\n\r\n1\r\nx\r\n0\r\n\r\nREST", consumed: 61, complete: true, body: "x", hasBody: true},
			} {
				t.Run(test.name, func(t *testing.T) {
					value, consumed, complete := parseHTTPMessage(kind, []byte(firstLine+test.suffix))
					wantConsumed := test.consumed
					if complete {
						wantConsumed += len(firstLine)
					}
					if complete != test.complete || consumed != wantConsumed {
						t.Fatalf("complete=%v consumed=%d, want %v %d", complete, consumed, test.complete, wantConsumed)
					}
					if !complete {
						if value != types.None {
							t.Fatalf("incomplete result=%v, want none", value)
						}
						return
					}
					result := mustMapValue(t, value)
					if test.errorCode != "" {
						errorValue, ok := result.MapGet(types.NewStr("error"))
						if !ok || errorValue.Type() != types.TYPE_LIST || len(errorValue.Elements()) != 1 || errorValue.Get(1).Str() != test.errorCode {
							t.Fatalf("error result=%v, want %s", result, test.errorCode)
						}
						return
					}
					body, hasBody := result.MapGet(types.NewStr("body"))
					if hasBody != test.hasBody || hasBody && body.Str() != test.body {
						t.Fatalf("hasBody=%v body=%v, want %v %q", hasBody, body, test.hasBody, test.body)
					}
				})
			}
		})
	}
}
