package server

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
)

// tcpFuzzConn reuses the transport tests' in-memory net.Conn, limiting each
// underlying Read independently of bufio's requested buffer size.
type tcpFuzzConn struct {
	*fakeConn
	chunks []byte
	next   int
	closed bool
}

func (c *tcpFuzzConn) Close() error {
	c.closed = true
	return c.fakeConn.Close()
}

func (c *tcpFuzzConn) Read(p []byte) (int, error) {
	if len(c.chunks) > 0 {
		size := 1 + int(c.chunks[c.next])
		c.next = (c.next + 1) % len(c.chunks)
		p = p[:min(len(p), size)]
	}
	return c.fakeConn.Read(p)
}

type tcpFuzzInput struct {
	line string
	oob  bool
}

func readTCPFuzzInputs(t *testing.T, transport *TCPTransport, size int) ([]tcpFuzzInput, error) {
	t.Helper()
	defer transport.Close()
	var inputs []tcpFuzzInput
	// Each successful call must consume a byte or flush the final partial
	// line at EOF. One more call must report EOF or the production cap error.
	for calls := 0; calls <= size+1; calls++ {
		line, oob, err := transport.ReadInput()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, errMaxLineBytes) {
				t.Fatalf("ReadInput: %v", err)
			}
			return inputs, err
		}
		if strings.ContainsAny(line, "\r\n") {
			t.Fatalf("ReadInput returned CR/LF: %q (OOB=%v)", line, oob)
		}
		if transport.lineBuf.Len() > maxLineBytes || len(transport.tCommand) > maxLineBytes {
			t.Fatal("ReadInput exceeded the pending input cap")
		}
		inputs = append(inputs, tcpFuzzInput{line, oob})
	}
	t.Fatal("ReadInput did not reach a terminal error within the input byte budget")
	return nil, nil
}

// FuzzTCPReadInput exercises bytes supplied by remote clients before login:
// server/input_processor.go calls TCPTransport.ReadInput (transport.go:101).
func FuzzTCPReadInput(f *testing.F) {
	for _, data := range [][]byte{
		{},
		[]byte("connect wizard\n"),
		[]byte("first\r\nsecond\nthird\rfourth"),
		[]byte("\r\n\n\r\r\n"),
		{'a', tnIAC, tnIAC, 'b', '\n'},
		{'a', tnIAC, tnSB, 31, 0, '\r', '\n', tnIAC, tnIAC, tnIAC, tnSE, 'b', '\n'},
		{tnIAC},
		{tnIAC, tnWILL},
		{tnIAC, tnSB},
		{'a', tnIAC, tnSB, 31, tnIAC},
		// BINARY option negotiation is OOB here; logged-in binary input uses
		// ReadChunk instead (server/input_processor.go).
		{tnIAC, tnWILL, 0, tnIAC, tnDO, 0, 'x', '\n', tnIAC, tnWONT, 0, tnIAC, tnDONT, 0},
		{0, 8, 9, 27, 127, 128, 254, '\n'},
	} {
		f.Add(data, []byte{0}) // Every byte is a separate underlying Read.
		f.Add(data, []byte{1, 2, 7, 255})
	}
	f.Fuzz(func(t *testing.T, data, chunks []byte) {
		// The fake connection never blocks. A CPU spin inside ReadInput must
		// also crash the fuzz worker, preserving the input instead of hanging.
		watchdog := time.AfterFunc(10*time.Second, func() { panic("TCP ReadInput did not terminate") })
		defer watchdog.Stop()

		want, wantErr := readTCPFuzzInputs(t, newTestTransport(data), len(data))
		conn := &tcpFuzzConn{
			fakeConn: &fakeConn{bytes.NewReader(data)},
			chunks:   chunks,
		}
		got, gotErr := readTCPFuzzInputs(t, NewTCPTransport(conn), len(data))
		if !errors.Is(gotErr, wantErr) || !slices.Equal(got, want) {
			t.Fatalf("chunked input = %v, %v; contiguous = %v, %v", got, gotErr, want, wantErr)
		}
	})
}

func TestTCPReadInputFuzzCapBoundaries(t *testing.T) {
	// Keep the multi-megabyte boundary cases outside the mutation corpus.
	// PR #568's error is consumed by InputProcessor.HandleConnection's close path;
	// ReadInput itself reports errMaxLineBytes rather than closing net.Conn.
	for _, telnet := range []bool{false, true} {
		for _, extra := range []int{-1, 0, 1} {
			data := bytes.Repeat([]byte{'x'}, maxLineBytes+extra)
			if telnet {
				data[0], data[1] = tnIAC, tnSB
			}
			for _, terminator := range [][]byte{nil, {'\n'}} {
				input := append(bytes.Clone(data), terminator...)
				wantErr := io.EOF
				if len(input) > maxLineBytes {
					wantErr = errMaxLineBytes
				}
				want, err := readTCPFuzzInputs(t, newTestTransport(input), len(input))
				if !errors.Is(err, wantErr) {
					t.Fatalf("telnet=%v extra=%d terminator=%q: %v, want %v", telnet, extra, terminator, err, wantErr)
				}
				conn := &tcpFuzzConn{fakeConn: &fakeConn{bytes.NewReader(input)}, chunks: []byte{255}}
				got, err := readTCPFuzzInputs(t, NewTCPTransport(conn), len(input))
				if !errors.Is(err, wantErr) || !slices.Equal(got, want) {
					t.Fatalf("chunked cap boundary differs: telnet=%v extra=%d terminator=%q", telnet, extra, terminator)
				}
			}
		}
	}
}

func TestTCPReadInputFuzzCapClosesConnection(t *testing.T) {
	for _, telnet := range []bool{false, true} {
		data := bytes.Repeat([]byte{'x'}, maxLineBytes+1)
		if telnet {
			data[0], data[1] = tnIAC, tnSB
		}
		store := dbstore.NewStore()
		rt := engine.NewRuntime(store)
		processor := NewInputProcessor(store, rt)
		cm := NewConnectionManager(7777)
		processor.SetConnectionManager(cm)
		processor.Start()
		conn := &tcpFuzzConn{fakeConn: &fakeConn{bytes.NewReader(data)}, chunks: []byte{255}}
		processor.HandleConnection(cm.NewConnectionFromTransport(NewTCPTransport(conn)))
		processor.Stop()
		rt.Stop()
		if !conn.closed {
			t.Fatalf("cap error did not close the pre-login connection (telnet=%v)", telnet)
		}
	}
}
