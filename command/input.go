package command

import "github.com/MongooseMoo/barn/types"

// InputEvent represents a line of input, or a disconnect, from a connection.
type InputEvent struct {
	ConnID       int64
	Player       types.ObjID
	Line         string
	IsOutOfBand  bool
	IsDisconnect bool
	IsTimeout    bool
	Done         chan struct{}
}
