package peer

import (
	"sync"

	peerMessage "git.fleta.io/fleta/framework/peer/message"
)

//NodeStore is the structure of the connection information.
type NodeStore struct {
	l sync.Mutex
	a []peerMessage.ConnectInfo
	m map[string]peerMessage.ConnectInfo
}

//NewNodeStore is creator of NodeStore
func NewNodeStore(TempMockID string) (*NodeStore, error) {
	n := &NodeStore{}
	return n, nil
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (n *NodeStore) LoadOrStore(key string, value peerMessage.ConnectInfo) peerMessage.ConnectInfo {
	n.l.Lock()
	defer n.l.Unlock()
	if 0 == len(n.m) {
		n.m = map[string]peerMessage.ConnectInfo{
			key: value,
		}
		n.a = []peerMessage.ConnectInfo{value}
	} else {
		v, has := n.m[key]
		if has {
			return v
		}
		n.m[key] = value
		n.a = append(n.a, value)
	}
	return value
}

// Store sets the value for a key.
func (n *NodeStore) Store(key string, value peerMessage.ConnectInfo) {
	n.l.Lock()
	if 0 == len(n.m) {
		n.m = map[string]peerMessage.ConnectInfo{
			key: value,
		}
		n.a = []peerMessage.ConnectInfo{value}
	} else {
		n.m[key] = value
		n.a = append(n.a, value)
	}
	n.l.Unlock()

}

// Get returns the value stored in the array for a index
func (n *NodeStore) Get(i int) peerMessage.ConnectInfo {
	if i < 0 || i > len(n.a) {
		return peerMessage.ConnectInfo{}
	}
	return n.a[i]
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (n *NodeStore) Load(key string) (peerMessage.ConnectInfo, bool) {
	n.l.Lock()
	defer n.l.Unlock()
	v, has := n.m[key]
	return v, has
}

// Delete deletes the value for a key.
func (n *NodeStore) Delete(key string) {
	n.l.Lock()
	defer n.l.Unlock()

	delete(n.m, key)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
func (n *NodeStore) Range(f func(string, peerMessage.ConnectInfo) bool) {
	n.l.Lock()
	defer n.l.Unlock()

	for key, value := range n.m {
		if !f(key, value) {
			break
		}
	}
}

// Len returns the length of this map.
func (n *NodeStore) Len() int {
	n.l.Lock()
	defer n.l.Unlock()
	return len(n.m)
}

//ConnectMap is the structure of peer list
type ConnectMap struct {
	l sync.Mutex
	m map[string]*Peer
}

// Len returns to ConnectMap length
func (n *ConnectMap) Len() int {
	n.l.Lock()
	defer n.l.Unlock()
	return len(n.m)
}

// Store sets the value for a key.
func (n *ConnectMap) Store(key string, value *Peer) {
	n.l.Lock()
	if 0 == len(n.m) {
		n.m = map[string]*Peer{
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
func (n *ConnectMap) Load(key string) (*Peer, bool) {
	n.l.Lock()
	defer n.l.Unlock()
	v, has := n.m[key]
	return v, has
}

// Delete deletes the value for a key.
func (n *ConnectMap) Delete(key string) {
	n.l.Lock()
	defer n.l.Unlock()

	delete(n.m, key)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
func (n *ConnectMap) Range(f func(string, *Peer) bool) {
	n.l.Lock()
	defer n.l.Unlock()

	for key, value := range n.m {
		if !f(key, value) {
			break
		}
	}
}

//CandidateMap is the structure of candidate list
type CandidateMap struct {
	l sync.Mutex
	m map[string]candidateState
}

// Store sets the value for a key.
func (n *CandidateMap) store(key string, value candidateState) {
	n.l.Lock()
	defer n.l.Unlock()

	if 0 == len(n.m) {
		n.m = map[string]candidateState{
			key: value,
		}
	} else {
		n.m[key] = value
	}
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (n *CandidateMap) load(key string) (candidateState, bool) {
	n.l.Lock()
	defer n.l.Unlock()
	v, has := n.m[key]
	return v, has
}

// Delete deletes the value for a key.
func (n *CandidateMap) delete(key string) {
	n.l.Lock()
	defer n.l.Unlock()

	delete(n.m, key)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
func (n *CandidateMap) rangeMap(f func(string, candidateState) bool) {
	n.l.Lock()
	defer n.l.Unlock()

	for key, value := range n.m {
		if !f(key, value) {
			break
		}
	}
}
