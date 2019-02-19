package mesh

import (
	"io"

	"git.fleta.io/fleta/framework/message"
)

// EventHandler is a event handler of the mesh
type EventHandler interface {
	OnConnected(p Peer)
	OnDisconnected(p Peer)
	OnRecv(p Peer, r io.Reader, t message.Type) error
}
