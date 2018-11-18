package peermessage

import (
	"io"

	"git.fleta.io/fleta/framework/message"
)

// Pong is struct of pong message
type Pong struct {
	Ping
}

// PongCreator is pong creator
func PongCreator(r io.Reader) message.Message {
	p := &Pong{}
	p.ReadFrom(r)
	return p
}

// PongMessageType is unique message type.
var PongMessageType message.Type

func init() {
	PongMessageType = message.DefineType("pong")
}

// SendPong creates a "pong" and sends a message.
func SendPong(p message.SendInterface, ping *Ping) {
	pong := &Pong{
		Ping: *ping,
	}
	p.Send(pong)
}

// Type is the basic function of "message".
// Returns the type of message.
func (pong *Pong) Type() message.Type {
	return PongMessageType
}
