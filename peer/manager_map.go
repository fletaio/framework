package peer

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"time"

	"git.fleta.io/fleta/framework/peer/peermessage"
	"github.com/dgraph-io/badger"
)

//NodeStore is the structure of the connection information.
type nodeStore struct {
	l  sync.Mutex
	db *badger.DB
	a  []peermessage.ConnectInfo
	m  map[string]*peermessage.ConnectInfo
}

//NewNodeStore is creator of NodeStore
func newNodeStore(dbpath string) (*nodeStore, error) {
	db, err := openNodesDB(dbpath)
	if err != nil {
		return nil, err
	}
	n := &nodeStore{
		db: db,
	}

	if err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			value, err := it.Item().ValueCopy(nil)
			if err != nil {
				return err
			}
			bf := bytes.NewBuffer(value)

			var ci peermessage.ConnectInfo
			ci.ReadFrom(bf)
			ci.PingScoreBoard = &peermessage.ScoreBoardMap{}
			n.LoadOrStore(ci.Address, ci)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return n, nil
}
func openNodesDB(dbPath string) (*badger.DB, error) {
	opts := badger.DefaultOptions
	opts.Dir = dbPath
	opts.ValueDir = dbPath
	opts.Truncate = true
	opts.SyncWrites = true
	opts.ValueLogFileSize = 1 << 24
	lockfilePath := filepath.Join(opts.Dir, "LOCK")
	os.MkdirAll(dbPath, os.ModeDir)

	os.Remove(lockfilePath)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	{
	again:
		if err := db.RunValueLogGC(0.7); err != nil {
		} else {
			goto again
		}
	}

	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for range ticker.C {
		again:
			if err := db.RunValueLogGC(0.7); err != nil {
			} else {
				goto again
			}
		}
	}()

	return db, nil
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (n *nodeStore) LoadOrStore(key string, value peermessage.ConnectInfo) peermessage.ConnectInfo {
	n.l.Lock()
	defer n.l.Unlock()

	if 0 == len(n.m) {
		n.unsafeStore(key, value)
	} else {
		v, has := n.m[key]
		if has {
			return *v
		}
		n.unsafeStore(key, value)
	}
	return value
}

// Update updates the value for a key.
func (n *nodeStore) Update(key string, update func(peermessage.ConnectInfo) peermessage.ConnectInfo) bool {
	v, has := n.m[key]
	if has {
		re := update(*v)
		v.Address = re.Address
		v.PingTime = re.PingTime

		n.db.Update(func(txn *badger.Txn) error {
			bf := bytes.Buffer{}
			v.WriteTo(&bf)
			if err := txn.Set([]byte(key), bf.Bytes()); err != nil {
				return err
			}
			return nil
		})

	}
	return has
}

// Store sets the value for a key.
func (n *nodeStore) Store(key string, value peermessage.ConnectInfo) {
	n.l.Lock()
	defer n.l.Unlock()
	n.unsafeStore(key, value)
}

func (n *nodeStore) unsafeStore(key string, value peermessage.ConnectInfo) {
	if 0 == len(n.m) {
		n.a = []peermessage.ConnectInfo{value}
		n.m = map[string]*peermessage.ConnectInfo{
			key: &n.a[0],
		}
	} else {
		v, has := n.m[key]
		if has {
			v.Address = value.Address
			v.PingTime = value.PingTime
			v.PingScoreBoard = value.PingScoreBoard
		} else {
			n.a = append(n.a, value)
			n.m[key] = &value
		}
	}
	n.db.Update(func(txn *badger.Txn) error {
		bf := bytes.Buffer{}
		value.WriteTo(&bf)
		if err := txn.Set([]byte(key), bf.Bytes()); err != nil {
			return err
		}
		return nil
	})
}

// Get returns the value stored in the array for a index
func (n *nodeStore) Get(i int) peermessage.ConnectInfo {
	if i < 0 || i > len(n.a) {
		return peermessage.ConnectInfo{}
	}
	return n.a[i]
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (n *nodeStore) Load(key string) (p peermessage.ConnectInfo, has bool) {
	n.l.Lock()
	defer n.l.Unlock()

	v, has := n.m[key]
	if has {
		p = *v
	}
	return
}

// // Delete deletes the value for a key.
// func (n *nodeStore) Delete(key string) {
// 	n.l.Lock()
// 	defer n.l.Unlock()

// 	delete(n.m, key)
// }

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
func (n *nodeStore) Range(f func(string, peermessage.ConnectInfo) bool) {
	n.l.Lock()
	defer n.l.Unlock()

	for key, value := range n.m {
		if !f(key, *value) {
			break
		}
	}
}

// Len returns the length of this map.
func (n *nodeStore) Len() int {
	n.l.Lock()
	defer n.l.Unlock()

	return len(n.m)
}

//ConnectMap is the structure of peer list
type connectMap struct {
	l sync.RWMutex
	m map[string]Peer
}

// Len returns to ConnectMap length
func (n *connectMap) Len() int {
	n.l.RLock()
	defer n.l.RUnlock()

	return len(n.m)
}

// Store sets the value for a key.
func (n *connectMap) Store(key string, value Peer) {
	n.l.Lock()
	defer n.l.Unlock()

	if 0 == len(n.m) {
		n.m = map[string]Peer{
			key: value,
		}
	} else {
		n.m[key] = value
	}
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (n *connectMap) Load(key string) (Peer, bool) {
	n.l.RLock()
	defer n.l.RUnlock()

	v, has := n.m[key]
	return v, has
}

// Delete deletes the value for a key.
func (n *connectMap) Delete(key string) {
	n.l.Lock()
	defer n.l.Unlock()

	delete(n.m, key)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
// func (n *connectMap) Range() <-chan *peer {
// 	arr := make([]*peer, len(n.m))

// 	for _, value := range n.m {
// 		arr = append(arr, value)
// 	}
// 	pch := make(chan *peer)
// 	go func() {
// 		for _, v := range arr {
// 			pch <- v
// 		}
// 		close(pch)
// 	}()
// 	return pch
// }

func (n *connectMap) Range(f func(string, Peer) bool) {
	n.l.Lock()
	defer n.l.Unlock()

	for key, value := range n.m {
		if !f(key, value) {
			break
		}
	}
}

//CandidateMap is the structure of candidate list
type candidateMap struct {
	l sync.Mutex
	m map[string]candidateState
}

// Store sets the value for a key.
func (n *candidateMap) store(key string, value candidateState) {
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
func (n *candidateMap) load(key string) (candidateState, bool) {
	n.l.Lock()
	defer n.l.Unlock()

	v, has := n.m[key]
	return v, has
}

// Delete deletes the value for a key.
func (n *candidateMap) delete(key string) {
	n.l.Lock()
	defer n.l.Unlock()

	delete(n.m, key)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
func (n *candidateMap) rangeMap(f func(string, candidateState) bool) {
	n.l.Lock()
	defer n.l.Unlock()

	for key, value := range n.m {
		if !f(key, value) {
			break
		}
	}
}
