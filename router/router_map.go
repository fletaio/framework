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
	m map[RemoteAddr]ListenerState
}

//ListenState is type of listener's status
type ListenState int8

const (
	nothing   ListenState = 0
	listening ListenState = 1
	pause     ListenState = 2
)

//ListenerState is a structure that stores listener and states.
type ListenerState struct {
	l  net.Listener
	s  ListenState
	cl map[common.Coordinate]ListenState
}

// Store sets the value for a key.
func (n *ListenerMap) Store(key RemoteAddr, chain *common.Coordinate, value net.Listener) {
	n.l.Lock()
	defer n.l.Unlock()
	if 0 == len(n.m) {
		n.m = map[RemoteAddr]ListenerState{
			key: ListenerState{l: value, s: listening, cl: map[common.Coordinate]ListenState{*chain: listening}},
		}
	} else {
		if l, has := n.m[key]; has {
			l.cl[*chain] = listening
		} else {
			n.m[key] = ListenerState{l: value, s: listening, cl: map[common.Coordinate]ListenState{*chain: listening}}
		}
	}

}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (n *ListenerMap) Load(key RemoteAddr) (ListenerState, bool) {
	n.l.Lock()
	defer n.l.Unlock()
	v, has := n.m[key]
	return v, has
}

// State returns the entire status.
func (ls *ListenerState) State() ListenState {
	return ls.s
}

// UpdateState updates the status of the lower listener and returns the entire status.
// the "entire status" is calculated as the AND of the lower listener
func (n *ListenerMap) UpdateState(key RemoteAddr, chain *common.Coordinate, state ListenState) (ListenState, error) {
	n.l.Lock()
	defer n.l.Unlock()
	if v, has := n.m[key]; has {
		v.cl[*chain] = state
		for _, state := range v.cl {
			if state == listening {
				v.s = listening
				return listening, nil
			}
		}
		v.s = pause
		return pause, nil
	}
	return nothing, ErrNotFoundListener
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
