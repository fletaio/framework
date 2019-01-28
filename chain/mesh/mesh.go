package mesh

import (
	"io"

	"git.fleta.io/fleta/framework/message"
)

// Mesh manages peer network
type Mesh interface {
	Add(netAddr string, doForce bool)
	Remove(netAddr string)
	RemoveByID(ID string)
	Ban(netAddr string, Seconds uint32)
	BanByID(ID string, Seconds uint32)
	Unban(netAddr string)
	Peers() []Peer
}

// EventHandler is a event handler of the mesh
type EventHandler interface {
	BeforeConnect(p Peer) error
	AfterConnect(p Peer)
	OnRecv(p Peer, msg message.Type, r io.Reader) error
	OnClosed(p Peer)
}

// Peer is a connected node with this node
type Peer interface {
	message.Sender
	ID() string
	NetAddr() string
}
