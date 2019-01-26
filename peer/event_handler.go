package peer

import (
	"io"

	"git.fleta.io/fleta/framework/message"
)

//EventHandler is callback when peer connected or closed
type EventHandler interface {
	BeforeConnect(p Peer) error
	AfterConnect(p Peer)
	OnRecv(p Peer, t message.Type, r io.Reader) error
	OnClosed(p Peer)
}

//BaseEventHandler is empty EventHandler struct
type BaseEventHandler struct{}

//BeforeConnect is empty BaseEventHandler functions
func (b *BaseEventHandler) BeforeConnect(p Peer) error { return nil }

//AfterConnect is empty BaseEventHandler functions
func (b *BaseEventHandler) AfterConnect(p Peer) {}

//OnRecv is empty BaseEventHandler functions
func (b *BaseEventHandler) OnRecv(p Peer, t message.Type, r io.Reader) error { return nil }

//OnClosed is empty BaseEventHandler functions
func (b *BaseEventHandler) OnClosed(p Peer) {}
