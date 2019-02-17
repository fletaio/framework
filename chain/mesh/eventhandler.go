package mesh

import (
	"io"

	"git.fleta.io/fleta/framework/message"
)

// EventHandler is a event handler of the mesh
type EventHandler interface {
	BeforeConnect(p Peer) error
	AfterConnect(p Peer)
	OnRecv(p Peer, r io.Reader, t message.Type) error
	OnClosed(p Peer)
}
