package peer

//EventHandler is callback when peer connected or closed
type EventHandler interface {
	PeerConnected(p Peer)
	PeerDisconnected(p Peer)
}

//BaseEventHandler is empty EventHandler struct
type BaseEventHandler struct{}

//PeerConnected is empty BaseEventHandler functions
func (b *BaseEventHandler) PeerConnected(p Peer) {}

//PeerDisconnected is empty BaseEventHandler functions
func (b *BaseEventHandler) PeerDisconnected(p Peer) {}
