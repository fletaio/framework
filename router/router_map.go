package router

import (
	"net"
	"strconv"
	"sync"

	"git.fleta.io/fleta/common"
)

//ListenerMap is a structure for router waiting to listening.
type ListenerMap struct {
	l sync.Mutex
	m map[RemoteAddr]net.Listener
}

// Store sets the value for a key.
func (n *ListenerMap) Store(key RemoteAddr, value net.Listener) {
	n.l.Lock()
	if 0 == len(n.m) {
		n.m = map[RemoteAddr]net.Listener{
			key: value,
		}
	} else {
		n.m[key] = value
	}
	n.l.Unlock()

}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (n *ListenerMap) Load(key RemoteAddr) (net.Listener, bool) {
	n.l.Lock()
	defer n.l.Unlock()
	v, has := n.m[key]
	return v, has
}

// // Delete deletes the value for a key.
// func (n *ListenerMap) Delete(key RemoteAddr) {
// 	n.l.Lock()
// 	defer n.l.Unlock()

// 	delete(n.m, key)
// }

// // Range calls f sequentially for each key and value present in the map.
// // If f returns false, range stops the iteration.
// func (n *ListenerMap) Range(f func(RemoteAddr, net.Listener) bool) {
// 	n.l.Lock()
// 	defer n.l.Unlock()

// 	for key, value := range n.m {
// 		if !f(key, value) {
// 			break
// 		}
// 	}
// }

//PConnMap is a structure that manages physical connections.
type PConnMap struct {
	l sync.Mutex
	m map[RemoteAddr]*physicalConnection
}

// Store sets the value for a key.
func (n *PConnMap) store(key RemoteAddr, value *physicalConnection) {
	n.l.Lock()
	if 0 == len(n.m) {
		n.m = map[RemoteAddr]*physicalConnection{
			key: value,
		}
	} else {
		n.m[key] = value
	}
	n.l.Unlock()

}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (n *PConnMap) load(key RemoteAddr) (*physicalConnection, bool) {
	n.l.Lock()
	defer n.l.Unlock()
	v, has := n.m[key]
	return v, has
}

// Delete deletes the value for a key.
func (n *PConnMap) delete(key RemoteAddr) {
	n.l.Lock()
	defer n.l.Unlock()

	delete(n.m, key)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
func (n *PConnMap) Range(f func(RemoteAddr, *physicalConnection) bool) {
	n.l.Lock()
	defer n.l.Unlock()

	for key, value := range n.m {
		if !f(key, value) {
			break
		}
	}
}

//LConnMap is a structure that manages logical connections.
type LConnMap struct {
	l sync.Mutex
	m map[common.Coordinate]*logicalConnection
}

// Store sets the value for a key.
func (n *LConnMap) len() int {
	return len(n.m)
}

// Store sets the value for a key.
func (n *LConnMap) store(key common.Coordinate, value *logicalConnection) {
	n.l.Lock()
	if 0 == len(n.m) {
		n.m = map[common.Coordinate]*logicalConnection{
			key: value,
		}
	} else {
		n.m[key] = value
	}
	n.l.Unlock()

}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (n *LConnMap) load(key common.Coordinate) (*logicalConnection, bool) {
	n.l.Lock()
	defer n.l.Unlock()
	v, has := n.m[key]
	return v, has
}

// Delete deletes the value for a key.
func (n *LConnMap) delete(key common.Coordinate) {
	n.l.Lock()
	defer n.l.Unlock()

	delete(n.m, key)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
func (n *LConnMap) Range(f func(common.Coordinate, *logicalConnection) bool) {
	n.l.Lock()
	defer n.l.Unlock()

	for key, value := range n.m {
		if !f(key, value) {
			break
		}
	}
}

//ReceiverChanMap is a structure that manages received channel.
type ReceiverChanMap struct {
	l sync.Mutex
	m map[string]chan Receiver
}

// Load returns the value stored in the map for a key
// if it does not exist then make it
func (n *ReceiverChanMap) load(port int, ChainCoord common.Coordinate) chan Receiver {
	n.l.Lock()
	defer n.l.Unlock()

	key := strconv.Itoa(port) + ":" + string(ChainCoord.Bytes())

	v, has := n.m[key]
	if !has {
		ch := make(chan Receiver, 128)
		if 0 == len(n.m) {
			n.m = map[string]chan Receiver{
				key: ch,
			}
		} else {
			n.m[key] = ch
		}

		v, has = n.m[key]
	}

	if has {
		return v
	}
	return nil
}
