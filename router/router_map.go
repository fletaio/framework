package router

import (
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/fletaio/common"
	"github.com/fletaio/framework/log"
)

//ListenerMap is a structure for router waiting to listening.
type ListenerMap struct {
	l sync.Mutex
	m map[string]ListenerState
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
func (n *ListenerMap) Store(key string, chain *common.Coordinate, value net.Listener) {
	n.l.Lock()
	defer n.l.Unlock()
	if 0 == len(n.m) {
		n.m = map[string]ListenerState{
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
func (n *ListenerMap) Load(key string) (ListenerState, bool) {
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
func (n *ListenerMap) UpdateState(key string, chain *common.Coordinate, state ListenState) (ListenState, error) {
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
// func (n *ListenerMap) Delete(key string) {
// 	n.l.Lock()
// 	defer n.l.Unlock()

// 	delete(n.m, key)
// }

// // Range calls f sequentially for each key and value present in the map.
// // If f returns false, range stops the iteration.
// func (n *ListenerMap) Range(f func(string, net.Listener) bool) {
// 	n.l.Lock()
// 	defer n.l.Unlock()

// 	for key, value := range n.m {
// 		if !f(key, value) {
// 			break
// 		}
// 	}
// }

//PConnMap is a structure that manages logical connections.
type PConnMap struct {
	name string
	l    sync.Mutex
	m    map[string]*physicalConnection
}

func (n *PConnMap) lock(name string) {
	if nm := n.name; nm != "" {
		end := make(chan bool)
		go func(by, new string, end chan bool) {
			deadTimer := time.NewTimer(10 * time.Second)
			select {
			case <-deadTimer.C: //timeout
				log.Error("PConnMap lock by ", nm, ",", name, "waited 10 seconds to be unlocked.")
				<-end
				end <- true
			case <-end: //end
				end <- false
				deadTimer.Stop()
			}

		}(nm, name, end)
		n.l.Lock()
		end <- true
		f := <-end
		close(end)
		if f == true {
			log.Error("PConnMap unlock by ", nm, " enter ", name)
		}
	} else {
		n.l.Lock()
	}
	n.name = name
}

func (n *PConnMap) unlock() {
	n.name = ""
	n.l.Unlock()
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (n *PConnMap) load(key string) (*physicalConnection, bool) {
	v, has := n.m[key]
	return v, has
}

// Store sets the value for a key.
func (n *PConnMap) store(key string, value *physicalConnection) {
	if 0 == len(n.m) {
		n.m = map[string]*physicalConnection{
			key: value,
		}
	} else {
		n.m[key] = value
	}

}

// Delete deletes the value for a key.
func (n *PConnMap) delete(key string) {
	delete(n.m, key)
}

//LConnMap is a structure that manages logical connections.
type LConnMap struct {
	l    sync.Mutex
	name string
	m    map[common.Coordinate]*logicalConnection
}

func (n *LConnMap) lock(name string) {
	if nm := n.name; nm != "" {
		// log.Debug("LConnMap lock by ", nm, " and wait ", name)
		n.l.Lock()
		// log.Debug("LConnMap unlock by ", nm, " enter ", name)
	} else {
		n.l.Lock()
	}
	n.name = name
}

func (n *LConnMap) unlock() {
	n.name = ""
	n.l.Unlock()
}

// Store sets the value for a key.
func (n *LConnMap) len() int {
	return len(n.m)
}

// Store sets the value for a key.
func (n *LConnMap) store(key common.Coordinate, value *logicalConnection) {
	if 0 == len(n.m) {
		n.m = map[common.Coordinate]*logicalConnection{
			key: value,
		}
	} else {
		n.m[key] = value
	}

}

// has returns true if there is a value in the store.
func (n *LConnMap) has(key common.Coordinate) bool {
	_, has := n.m[key]
	return has
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (n *LConnMap) load(key common.Coordinate) (*logicalConnection, bool) {
	v, has := n.m[key]
	return v, has
}

// Delete deletes the value for a key.
func (n *LConnMap) delete(key common.Coordinate) {
	delete(n.m, key)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
func (n *LConnMap) Range(f func(common.Coordinate, *logicalConnection) bool) {
	for key, value := range n.m {
		if !f(key, value) {
			break
		}
	}
}

//ReceiverChanMap is a structure that manages received channel.
type ReceiverChanMap struct {
	l sync.Mutex
	m map[string]chan *logicalConnection
}

// Load returns the value stored in the map for a key
// if it does not exist then make it
func (n *ReceiverChanMap) load(port int, ChainCoord common.Coordinate) chan *logicalConnection {
	n.l.Lock()
	defer n.l.Unlock()

	key := strconv.Itoa(port) + ":" + string(ChainCoord.Bytes())

	v, has := n.m[key]
	if !has {
		ch := make(chan *logicalConnection, 128)
		if 0 == len(n.m) {
			n.m = map[string]chan *logicalConnection{
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

// TimerMap stores time limited data
type TimerMap struct {
	interval int64
	size     int64
	data     map[int64]map[interface{}]interface{}
}

// NewTimerMap is creator of timerMap
func NewTimerMap(interval time.Duration, size int64) *TimerMap {
	return &TimerMap{
		interval: int64(interval),
		size:     size,
		data:     map[int64]map[interface{}]interface{}{},
	}
}

// Store sets the value for a key.
func (tm *TimerMap) Store(key interface{}, value interface{}) {
	pos := time.Now().UnixNano() / tm.interval

	m, has := tm.data[pos]
	if !has {
		m = map[interface{}]interface{}{}
		tm.data[pos] = m
	}
	m[key] = value
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (tm *TimerMap) Load(key interface{}) (interface{}, bool) {
	deleteKeyList := []int64{}
	endLine := (time.Now().UnixNano() / tm.interval) - tm.size
	for k, v := range tm.data {
		if endLine > k {
			deleteKeyList = append(deleteKeyList, k)
			continue
		}

		if value, has := v[key]; has {
			return value, true
		}
	}

	for _, key := range deleteKeyList {
		delete(tm.data, key)
	}

	return nil, false
}
