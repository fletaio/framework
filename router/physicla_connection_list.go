package router

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger"
)

// PConnList stores information for peers even connected at least once.
type PConnList struct {
	db *badger.DB
}

// NewPConnList is creator of physical Connection list
func NewPConnList(dbpath string) (*PConnList, error) {
	db, err := openNodesDB(dbpath)
	if err != nil {
		return nil, err
	}
	n := &PConnList{
		db: db,
	}
	return n, nil
}

// Store is strore the physicalConnectionInfo
func (pl *PConnList) Store(v PhysicalConnectionInfo) error {
	err := pl.db.Update(func(txn *badger.Txn) error {
		bf := bytes.Buffer{}
		v.WriteTo(&bf)
		if err := txn.Set([]byte(v.Addr), bf.Bytes()); err != nil {
			return err
		}
		return nil
	})

	return err
}

// Get is returned strored physicalConnectionInfo
func (pl *PConnList) Get(addr string) (p PhysicalConnectionInfo, err error) {
	var v []byte
	if err2 := pl.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(addr))
		if err != nil {
			return err
		}
		value, err := item.ValueCopy(nil)
		v = value
		if err != nil {
			return err
		}
		return nil
	}); err2 != nil {
		err = err2
		return
	}
	bf := bytes.NewBuffer(v)
	p.ReadFrom(bf)
	return
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
