package server

import (
	"barn/types"
	"context"
	"sync"
	"time"
)

// Connection represents a player connection
type Connection struct {
	ID             int64
	transport      Transport
	player         types.ObjID
	loggedIn       bool
	outputBuffer   []string
	outputPrefix   string // PREFIX/OUTPUTPREFIX command sets this
	outputSuffix   string // SUFFIX/OUTPUTSUFFIX command sets this
	resolvedName   string
	connectedAt    time.Time
	ConnectionTime time.Time // Set when login completes (zero means not yet logged in)
	lastInput      time.Time
	outputCounter  int
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewConnection creates a new connection with a transport
func NewConnection(id int64, transport Transport) *Connection {
	ctx, cancel := context.WithCancel(context.Background())

	return &Connection{
		ID:           id,
		transport:    transport,
		player:       types.ObjID(-1), // Not logged in yet
		loggedIn:     false,
		outputBuffer: make([]string, 0),
		connectedAt:  time.Now(),
		lastInput:    time.Now(),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Send sends a message to the connection immediately
func (c *Connection) Send(message string) error {
	c.mu.Lock()
	c.outputCounter++
	c.mu.Unlock()
	return c.transport.WriteLine(message)
}

// Buffer adds a message to the output buffer (flushed later)
func (c *Connection) Buffer(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outputBuffer = append(c.outputBuffer, message)
	c.outputCounter++
}

// Flush flushes the output buffer
func (c *Connection) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, msg := range c.outputBuffer {
		if err := c.transport.WriteLine(msg); err != nil {
			return err
		}
	}
	c.outputBuffer = c.outputBuffer[:0]
	return nil
}

// ReadLine reads a line of input
func (c *Connection) ReadLine() (string, error) {
	line, err := c.transport.ReadLine()
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.lastInput = time.Now()
	c.mu.Unlock()

	return line, nil
}

// Close closes the connection
func (c *Connection) Close() error {
	c.cancel()
	return c.transport.Close()
}

// RemoteAddr returns the remote address of the connection
func (c *Connection) RemoteAddr() string {
	return c.transport.RemoteAddr()
}

// GetPlayer returns the player ObjID
func (c *Connection) GetPlayer() types.ObjID {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.player
}

// SetPlayer sets the player ObjID and marks as logged in
func (c *Connection) SetPlayer(player types.ObjID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.player = player
	c.loggedIn = true
}

// IsLoggedIn returns whether the connection is logged in
func (c *Connection) IsLoggedIn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loggedIn
}

// GetOutputPrefix returns the connection's output prefix
func (c *Connection) GetOutputPrefix() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outputPrefix
}

// GetOutputSuffix returns the connection's output suffix
func (c *Connection) GetOutputSuffix() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outputSuffix
}

// BufferedOutputLength returns the number of queued output lines.
func (c *Connection) BufferedOutputLength() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outputCounter
}

// ConnectedSeconds returns how long the connection has been active.
func (c *Connection) ConnectedSeconds() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	seconds := time.Since(c.connectedAt).Seconds()
	if seconds < 0 {
		return 0
	}
	return int64(seconds)
}

// IdleSeconds returns how long since the last input was received.
func (c *Connection) IdleSeconds() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	seconds := time.Since(c.lastInput).Seconds()
	if seconds < 0 {
		return 0
	}
	return int64(seconds)
}

func (c *Connection) GetResolvedName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resolvedName
}

func (c *Connection) SetResolvedName(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resolvedName = name
}
