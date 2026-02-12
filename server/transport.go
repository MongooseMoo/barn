package server

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
)

// Telnet protocol constants (RFC 854, RFC 855)
const (
	tnIAC  = 255 // Interpret As Command
	tnDONT = 254
	tnDO   = 253
	tnWONT = 252
	tnWILL = 251
	tnSB   = 250 // Subnegotiation Begin
	tnSE   = 240 // Subnegotiation End
)

// Telnet state machine states, matching ToastStunt's implementation
type telnetState int

const (
	telnetStateNormal    telnetState = iota // Processing normal text
	telnetStateIAC                          // Just saw IAC
	telnetStateCommand                      // Reading option byte after WILL/WONT/DO/DONT
	telnetStateSubneg                       // In subnegotiation (after SB)
	telnetStateSubnegIAC                    // Saw IAC while in subnegotiation
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
	conn        net.Conn
	reader      *bufio.Reader
	writer      *bufio.Writer
	mu          sync.Mutex
	tState      telnetState
	lastWasCR   bool
}

// NewTCPTransport creates a new TCP transport from a net.Conn
func NewTCPTransport(conn net.Conn) *TCPTransport {
	return &TCPTransport{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
		tState: telnetStateNormal,
	}
}

// ReadLine reads a line from the connection, stripping telnet IAC sequences.
// Blocks until a complete line (terminated by CR or LF) is available, or EOF.
// This implements the same telnet state machine as ToastStunt's process_telnet_byte.
func (t *TCPTransport) ReadLine() (string, error) {
	var line strings.Builder

	for {
		b, err := t.reader.ReadByte()
		if err != nil {
			// If we have partial data and hit EOF, return what we have
			if err == io.EOF && line.Len() > 0 {
				return line.String(), nil
			}
			return "", err
		}

		switch t.tState {
		case telnetStateNormal:
			if b == tnIAC {
				// Start of a telnet command - enter IAC state
				t.tState = telnetStateIAC
			} else if b == '\r' {
				// CR terminates a line
				t.lastWasCR = true
				return line.String(), nil
			} else if b == '\n' {
				if t.lastWasCR {
					// LF after CR - ignore (CR already delivered the line)
					t.lastWasCR = false
					continue
				}
				// Bare LF also terminates a line
				return line.String(), nil
			} else {
				t.lastWasCR = false
				// Normal printable character or high byte - add to line
				// Accept printable ASCII, space, tab, and high bytes (128-254)
				if (b >= 32 && b <= 126) || b == '\t' || (b >= 128 && b <= 254) {
					line.WriteByte(b)
				}
				// Control characters other than CR/LF/TAB are silently dropped
			}

		case telnetStateIAC:
			if b == tnIAC {
				// Escaped IAC (0xFF 0xFF) -> literal 0xFF in input
				t.tState = telnetStateNormal
				// Don't add to line - literal 0xFF in text is unusual
			} else if b == tnSB {
				// Start of subnegotiation
				t.tState = telnetStateSubneg
			} else if b == tnWILL || b == tnWONT || b == tnDO || b == tnDONT {
				// Two-byte command (WILL/WONT/DO/DONT + option byte)
				t.tState = telnetStateCommand
			} else {
				// Unknown command byte - consume and return to normal
				t.tState = telnetStateNormal
			}

		case telnetStateCommand:
			// This is the option byte after WILL/WONT/DO/DONT - consume it
			// and return to normal state
			t.tState = telnetStateNormal

		case telnetStateSubneg:
			// Inside subnegotiation - consume bytes until IAC SE
			if b == tnIAC {
				t.tState = telnetStateSubnegIAC
			}
			// All other bytes in subnegotiation are silently consumed

		case telnetStateSubnegIAC:
			if b == tnSE {
				// End of subnegotiation
				t.tState = telnetStateNormal
			} else if b == tnIAC {
				// Escaped IAC within subnegotiation - stay in subneg
				t.tState = telnetStateSubneg
			} else {
				// Unexpected byte after IAC in subneg - back to subneg
				t.tState = telnetStateSubneg
			}
		}
	}
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
