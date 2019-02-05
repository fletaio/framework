package mesh

import (
	"io"

	"git.fleta.io/fleta/framework/message"
)

// EventHandler is a event handler of the mesh
type EventHandler interface {
	BeforeConnect(p Peer) error
	AfterConnect(p Peer)
	OnRecv(p Peer, msg message.Type, r io.Reader) error
	OnClosed(p Peer)
}
