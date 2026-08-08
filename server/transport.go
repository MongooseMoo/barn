package server

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/MongooseMoo/barn/builtins"
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

type BinaryTransport interface {
	ReadChunk() (string, error)
}

type InputTransport interface {
	ReadInput() (string, bool, error)
}

// TCPTransport wraps a net.Conn for TCP socket communication
type TCPTransport struct {
	conn      net.Conn
	reader    *bufio.Reader
	writer    *bufio.Writer
	mu        sync.Mutex
	tState    telnetState
	tCommand  []byte
	lineBuf   strings.Builder
	lastWasCR bool
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

func (t *TCPTransport) ReadLine() (string, error) {
	for {
		line, isOOB, err := t.ReadInput()
		if err != nil {
			return line, err
		}
		if !isOOB {
			return line, nil
		}
	}
}

// ReadInput reads either a complete line or a complete telnet IAC command.
// Telnet commands are returned as out-of-band input immediately, without
// waiting for a CR/LF terminator.
func (t *TCPTransport) ReadInput() (string, bool, error) {
	for {
		b, err := t.reader.ReadByte()
		if err != nil {
			// If we have partial data and hit EOF, return what we have
			if err == io.EOF && t.lineBuf.Len() > 0 {
				line := t.lineBuf.String()
				t.lineBuf.Reset()
				return line, false, nil
			}
			return "", false, err
		}

		switch t.tState {
		case telnetStateNormal:
			if b == tnIAC {
				// Start of a telnet command - enter IAC state
				t.tState = telnetStateIAC
				t.tCommand = append(t.tCommand[:0], b)
			} else if b == '\r' {
				// CR terminates a line
				t.lastWasCR = true
				line := t.lineBuf.String()
				t.lineBuf.Reset()
				return line, false, nil
			} else if b == '\n' {
				if t.lastWasCR {
					// LF after CR - ignore (CR already delivered the line)
					t.lastWasCR = false
					continue
				}
				// Bare LF also terminates a line
				line := t.lineBuf.String()
				t.lineBuf.Reset()
				return line, false, nil
			} else {
				t.lastWasCR = false
				// Normal printable character or high byte - add to line
				// Accept printable ASCII, space, tab, and high bytes (128-254)
				if (b >= 32 && b <= 126) || b == '\t' || (b >= 128 && b <= 254) {
					t.lineBuf.WriteByte(b)
				}
				// Control characters other than CR/LF/TAB are silently dropped
			}

		case telnetStateIAC:
			t.tCommand = append(t.tCommand, b)
			if b == tnIAC {
				// Escaped IAC (0xFF 0xFF) -> literal 0xFF in input
				t.tState = telnetStateNormal
				t.tCommand = t.tCommand[:0]
				// Don't add to line - literal 0xFF in text is unusual
			} else if b == tnSB {
				// Start of subnegotiation
				t.tState = telnetStateSubneg
			} else if b == tnWILL || b == tnWONT || b == tnDO || b == tnDONT {
				// Two-byte command (WILL/WONT/DO/DONT + option byte)
				t.tState = telnetStateCommand
			} else {
				// Unknown command byte - consume and return to normal
				oob := formatTelnetCommand(t.tCommand)
				t.tCommand = t.tCommand[:0]
				t.tState = telnetStateNormal
				return oob, true, nil
			}

		case telnetStateCommand:
			// This is the option byte after WILL/WONT/DO/DONT - consume it
			// and return to normal state
			t.tCommand = append(t.tCommand, b)
			oob := formatTelnetCommand(t.tCommand)
			t.tCommand = t.tCommand[:0]
			t.tState = telnetStateNormal
			return oob, true, nil

		case telnetStateSubneg:
			t.tCommand = append(t.tCommand, b)
			// Inside subnegotiation - consume bytes until IAC SE
			if b == tnIAC {
				t.tState = telnetStateSubnegIAC
			}
			// All other bytes in subnegotiation are silently consumed

		case telnetStateSubnegIAC:
			t.tCommand = append(t.tCommand, b)
			if b == tnSE {
				// End of subnegotiation
				oob := formatTelnetCommand(t.tCommand)
				t.tCommand = t.tCommand[:0]
				t.tState = telnetStateNormal
				return oob, true, nil
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

// formatTelnetCommand renders a captured telnet IAC sequence as a MOO
// binary string ("~FF~F1"), matching Toast: OOB command dispatch sees the
// encoded text, never raw protocol bytes (gap_followups_toast_oracle pins).
func formatTelnetCommand(command []byte) string {
	return builtins.EncodeRawToBinary(command)
}

func (t *TCPTransport) ReadChunk() (string, error) {
	var chunk strings.Builder
	b, err := t.reader.ReadByte()
	if err != nil {
		return "", err
	}
	chunk.WriteByte(b)

	for t.reader.Buffered() > 0 {
		b, err = t.reader.ReadByte()
		if err != nil {
			break
		}
		chunk.WriteByte(b)
	}
	return chunk.String(), nil
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

func (t *TCPTransport) SetReadDeadline(deadline time.Time) error {
	return t.conn.SetReadDeadline(deadline)
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
