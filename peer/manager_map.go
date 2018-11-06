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
type NodeStore struct {
	l  sync.Mutex
	db *badger.DB
	a  []peermessage.ConnectInfo
	m  map[string]peermessage.ConnectInfo
}

//NewNodeStore is creator of NodeStore
func NewNodeStore(TempMockID string) (*NodeStore, error) {
	db, err := openNodesDB("./_data/" + TempMockID + "/nodes")
	if err != nil {
		return nil, err
	}
	n := &NodeStore{
		db: db,
	}

	if err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			value, err := it.Item().Value()
			if err != nil {
				return err
			}
			bf := bytes.NewBuffer(value)

			var ci peermessage.ConnectInfo
			ci.ReadFrom(bf)
			ci.ScoreBoard = &peermessage.ScoreBoardMap{}
			n.Store(ci.Address, ci)
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
	opts.ValueLogFileSize = 1 << 26
	lockfilePath := filepath.Join(opts.Dir, "LOCK")
	os.MkdirAll(dbPath, os.ModeDir)

	os.Remove(lockfilePath)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	lockfile, err := os.OpenFile(lockfilePath, os.O_EXCL, 0)
	if err != nil {
		lockfile.Close()
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
func (n *NodeStore) LoadOrStore(key string, value peermessage.ConnectInfo) peermessage.ConnectInfo {
	n.l.Lock()
	defer n.l.Unlock()

	if 0 == len(n.m) {
		n.m = map[string]peermessage.ConnectInfo{
			key: value,
		}
		n.a = []peermessage.ConnectInfo{value}
		n.db.Update(func(txn *badger.Txn) error {
			bf := bytes.Buffer{}
			value.WriteTo(&bf)
			if err := txn.Set([]byte(key), bf.Bytes()); err != nil {
				return err
			}
			return nil
		})
	} else {
		v, has := n.m[key]
		if has {
			return v
		}
		n.m[key] = value
		n.a = append(n.a, value)
		n.db.Update(func(txn *badger.Txn) error {
			bf := bytes.Buffer{}
			value.WriteTo(&bf)
			if err := txn.Set([]byte(key), bf.Bytes()); err != nil {
				return err
			}
			return nil
		})
	}
	return value
}

// Store sets the value for a key.
func (n *NodeStore) Store(key string, value peermessage.ConnectInfo) {
	n.l.Lock()
	defer n.l.Unlock()

	if 0 == len(n.m) {
		n.m = map[string]peermessage.ConnectInfo{
			key: value,
		}
		n.a = []peermessage.ConnectInfo{value}
	} else {
		v, has := n.m[key]
		if has {
			v.Address = value.Address
			v.PingTime = value.PingTime
			v.ScoreBoard = value.ScoreBoard
		} else {
			n.m[key] = value
			n.a = append(n.a, value)
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
func (n *NodeStore) Get(i int) peermessage.ConnectInfo {
	if i < 0 || i > len(n.a) {
		return peermessage.ConnectInfo{}
	}
	return n.a[i]
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (n *NodeStore) Load(key string) (peermessage.ConnectInfo, bool) {
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
func (n *NodeStore) Range(f func(string, peermessage.ConnectInfo) bool) {
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
	defer n.l.Unlock()

	if 0 == len(n.m) {
		n.m = map[string]*Peer{
			key: value,
		}
	} else {
		n.m[key] = value
	}
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
