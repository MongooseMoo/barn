package server

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
)

// Transport is the interface for connection I/O
type Transport interface {
	ReadLine() (string, error)
	WriteLine(string) error
	Close() error
	RemoteAddr() string
}

// TCPTransport wraps a net.Conn for TCP socket communication
type TCPTransport struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	mu     sync.Mutex
}

// NewTCPTransport creates a new TCP transport from a net.Conn
func NewTCPTransport(conn net.Conn) *TCPTransport {
	return &TCPTransport{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
	}
}

// ReadLine reads a line from the connection (blocks until newline or EOF)
func (t *TCPTransport) ReadLine() (string, error) {
	line, err := t.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	// Trim trailing newline and carriage return
	line = strings.TrimRight(line, "\r\n")
	return line, nil
}

// WriteLine writes a line to the connection with newline
func (t *TCPTransport) WriteLine(msg string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	_, err := t.writer.WriteString(msg + "\r\n")
	if err != nil {
		return err
	}
	return t.writer.Flush()
}

// Close closes the underlying connection
func (t *TCPTransport) Close() error {
	return t.conn.Close()
}

// RemoteAddr returns the remote address as a string
func (t *TCPTransport) RemoteAddr() string {
	return t.conn.RemoteAddr().String()
}

// PipeTransport is an in-memory transport for testing
type PipeTransport struct {
	input   chan string // Lines to feed to server (from test)
	output  chan string // Lines received from server (to test)
	closed  bool
	closeMu sync.Mutex
}

// NewPipeTransport creates a new pipe transport for testing
func NewPipeTransport() *PipeTransport {
	return &PipeTransport{
		input:  make(chan string, 100),
		output: make(chan string, 100),
		closed: false,
	}
}

// ReadLine reads a line from the input channel (blocks)
func (t *PipeTransport) ReadLine() (string, error) {
	t.closeMu.Lock()
	if t.closed {
		t.closeMu.Unlock()
		return "", io.EOF
	}
	t.closeMu.Unlock()

	line, ok := <-t.input
	if !ok {
		return "", io.EOF
	}
	return line, nil
}

// WriteLine writes a line to the output channel
func (t *PipeTransport) WriteLine(msg string) error {
	t.closeMu.Lock()
	if t.closed {
		t.closeMu.Unlock()
		return errors.New("transport closed")
	}
	t.closeMu.Unlock()

	t.output <- msg
	return nil
}

// Close closes the transport
func (t *PipeTransport) Close() error {
	t.closeMu.Lock()
	defer t.closeMu.Unlock()

	if !t.closed {
		t.closed = true
		close(t.input)
		close(t.output)
	}
	return nil
}

// RemoteAddr returns "test" for pipe transports
func (t *PipeTransport) RemoteAddr() string {
	return "test"
}

// Send sends a line to the server (called by test code)
func (t *PipeTransport) Send(line string) {
	t.input <- line
}

// Receive receives a line from the server (called by test code)
// Returns empty string if channel is closed
func (t *PipeTransport) Receive() string {
	line, ok := <-t.output
	if !ok {
		return ""
	}
	return line
}

// TryReceive attempts to receive without blocking
// Returns the line and true if available, empty and false otherwise
func (t *PipeTransport) TryReceive() (string, bool) {
	select {
	case line, ok := <-t.output:
		if !ok {
			return "", false
		}
		return line, true
	default:
		return "", false
	}
}

// DrainOutput reads all available output without blocking
func (t *PipeTransport) DrainOutput() []string {
	var lines []string
	for {
		select {
		case line, ok := <-t.output:
			if !ok {
				return lines
			}
			lines = append(lines, line)
		default:
			return lines
		}
	}
}
