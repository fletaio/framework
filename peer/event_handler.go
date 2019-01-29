package peer

import (
	"io"

	"git.fleta.io/fleta/framework/chain/mesh"
	"git.fleta.io/fleta/framework/message"
)

//BaseEventHandler is empty EventHandler struct
type BaseEventHandler struct {
	mesh.EventHandler
}

//BeforeConnect is empty BaseEventHandler functions
func (b *BaseEventHandler) BeforeConnect(p mesh.Peer) error { return nil }

//AfterConnect is empty BaseEventHandler functions
func (b *BaseEventHandler) AfterConnect(p mesh.Peer) {}

//OnRecv is empty BaseEventHandler functions
func (b *BaseEventHandler) OnRecv(p mesh.Peer, msg message.Type, r io.Reader) error {
	return message.ErrUnhandledMessage
}

//OnClosed is empty BaseEventHandler functions
func (b *BaseEventHandler) OnClosed(p mesh.Peer) {}
