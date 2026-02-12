package server

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

// connFromReader wraps a bytes.Reader to satisfy net.Conn for testing
type fakeConn struct {
	*bytes.Reader
}

func (f *fakeConn) Write(b []byte) (int, error)        { return 0, nil }
func (f *fakeConn) Close() error                        { return nil }
func (f *fakeConn) LocalAddr() net.Addr                 { return nil }
func (f *fakeConn) RemoteAddr() net.Addr                { return &net.TCPAddr{} }
func (f *fakeConn) SetDeadline(t time.Time) error       { return nil }
func (f *fakeConn) SetReadDeadline(t time.Time) error   { return nil }
func (f *fakeConn) SetWriteDeadline(t time.Time) error  { return nil }

func newTestTransport(data []byte) *TCPTransport {
	conn := &fakeConn{bytes.NewReader(data)}
	return NewTCPTransport(conn)
}

func TestReadLinePlainText(t *testing.T) {
	transport := newTestTransport([]byte("hello world\r\n"))
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", line)
	}
}

func TestReadLineLFOnly(t *testing.T) {
	transport := newTestTransport([]byte("hello\n"))
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "hello" {
		t.Errorf("expected %q, got %q", "hello", line)
	}
}

func TestReadLineCROnly(t *testing.T) {
	transport := newTestTransport([]byte("hello\r"))
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "hello" {
		t.Errorf("expected %q, got %q", "hello", line)
	}
}

func TestReadLineCRLFDoesNotDoubleDeliver(t *testing.T) {
	// CR\nLF should produce one line, not two
	transport := newTestTransport([]byte("first\r\nsecond\r\n"))
	line1, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line1 != "first" {
		t.Errorf("expected %q, got %q", "first", line1)
	}

	line2, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line2 != "second" {
		t.Errorf("expected %q, got %q", "second", line2)
	}
}

func TestReadLineStripsIACWILL(t *testing.T) {
	// IAC WILL ECHO followed by normal text
	data := []byte{0xFF, 0xFB, 0x01, 'h', 'e', 'l', 'l', 'o', '\n'}
	transport := newTestTransport(data)
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "hello" {
		t.Errorf("expected %q, got %q", "hello", line)
	}
}

func TestReadLineStripsIACWONT(t *testing.T) {
	// IAC WONT ECHO
	data := []byte{0xFF, 0xFC, 0x01, 'h', 'i', '\n'}
	transport := newTestTransport(data)
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "hi" {
		t.Errorf("expected %q, got %q", "hi", line)
	}
}

func TestReadLineStripsIACDO(t *testing.T) {
	// IAC DO SGA (suppress go ahead)
	data := []byte{0xFF, 0xFD, 0x03, 't', 'e', 's', 't', '\n'}
	transport := newTestTransport(data)
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "test" {
		t.Errorf("expected %q, got %q", "test", line)
	}
}

func TestReadLineStripsIACDONT(t *testing.T) {
	// IAC DONT ECHO
	data := []byte{0xFF, 0xFE, 0x01, 'o', 'k', '\n'}
	transport := newTestTransport(data)
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "ok" {
		t.Errorf("expected %q, got %q", "ok", line)
	}
}

func TestReadLineStripsSubnegotiation(t *testing.T) {
	// IAC SB NAWS <4 bytes> IAC SE followed by text
	data := []byte{
		0xFF, 0xFA, 0x1F, 0x00, 0x50, 0x00, 0x18, 0xFF, 0xF0, // IAC SB NAWS 0 80 0 24 IAC SE
		'h', 'i', '\n',
	}
	transport := newTestTransport(data)
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "hi" {
		t.Errorf("expected %q, got %q", "hi", line)
	}
}

func TestReadLineMultipleIACInRow(t *testing.T) {
	// Multiple IAC sequences before text
	data := []byte{
		0xFF, 0xFB, 0x01, // IAC WILL ECHO
		0xFF, 0xFD, 0x03, // IAC DO SGA
		0xFF, 0xFB, 0x1F, // IAC WILL NAWS
		'c', 'o', 'n', 'n', 'e', 'c', 't', ' ', 'w', 'i', 'z', 'a', 'r', 'd', '\n',
	}
	transport := newTestTransport(data)
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "connect wizard" {
		t.Errorf("expected %q, got %q", "connect wizard", line)
	}
}

func TestReadLineIACMidLine(t *testing.T) {
	// IAC sequence in the middle of a line
	data := []byte{'h', 'e', 0xFF, 0xFB, 0x01, 'l', 'l', 'o', '\n'}
	transport := newTestTransport(data)
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "hello" {
		t.Errorf("expected %q, got %q", "hello", line)
	}
}

func TestReadLineEOFNoNewline(t *testing.T) {
	// Partial line then EOF
	transport := newTestTransport([]byte("partial"))
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "partial" {
		t.Errorf("expected %q, got %q", "partial", line)
	}

	// Next read should give EOF
	_, err = transport.ReadLine()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestReadLineEmptyInput(t *testing.T) {
	transport := newTestTransport([]byte{})
	_, err := transport.ReadLine()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestReadLineOnlyIAC(t *testing.T) {
	// Only IAC sequences with no text, then newline
	data := []byte{0xFF, 0xFB, 0x01, 0xFF, 0xFD, 0x03, '\n'}
	transport := newTestTransport(data)
	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "" {
		t.Errorf("expected empty string, got %q", line)
	}
}
