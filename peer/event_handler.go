package peer

//EventHandler is callback when peer connected or closed
type EventHandler interface {
	PeerConnected(p Peer)
	PeerClosed(p Peer)
}

//BaseEventHandler is empty EventHandler struct
type BaseEventHandler struct{}

//PeerConnected is empty BaseEventHandler functions
func (b *BaseEventHandler) PeerConnected(p Peer) {}

//PeerClosed is empty BaseEventHandler functions
func (b *BaseEventHandler) PeerClosed(p Peer) {}
