package chain

import (
	"sync"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/common/hash"
)

// Chain validates and stores the chain data
type Chain struct {
	sync.Mutex
	handler EventHandler

	isClose   bool
	closeLock sync.RWMutex
}

// NewChain returns a Chain
func NewChain(handler EventHandler) *Chain {
	cn := &Chain{
		handler: handler,
	}
	return cn
}

// Init initializes the Chain
func (cn *Chain) Init() error {
	if err := cn.handler.OnInit(); err != nil {
		return err
	}
	return nil
}

// Close terminates and cleans the chain
func (cn *Chain) Close() {
	cn.closeLock.Lock()
	defer cn.closeLock.Unlock()

	cn.isClose = true
	cn.handler.OnClose()
}

// IsClose returns the close status of the chain
func (cn *Chain) IsClose() bool {
	cn.closeLock.RLock()
	defer cn.closeLock.RUnlock()

	return cn.isClose
}

// Provider returns the provider of the chain
func (cn *Chain) Provider() Provider {
	return cn.handler.Provider()
}

// CheckFork returns the header is forked or not
func (cn *Chain) CheckFork(fh *Header, sigs []common.Signature) error {
	cn.closeLock.RLock()
	defer cn.closeLock.RUnlock()
	if cn.isClose {
		return ErrChainClosed
	}

	cp := cn.handler.Provider()
	if fh.Height <= cp.Height() {
		ch, err := cp.Header(fh.Height)
		if err != nil {
			return err
		}
		if !ch.Hash().Equal(fh.Hash()) {
			if ch.PrevHash.Equal(fh.PrevHash) {
				if err := cn.handler.OnCheckFork(ch, sigs); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Screening returns a error when the provided data is not appendable to the chain
func (cn *Chain) Screening(cd *Data) error {
	cn.closeLock.RLock()
	defer cn.closeLock.RUnlock()
	if cn.isClose {
		return ErrChainClosed
	}

	cn.Lock()
	defer cn.Unlock()

	cp := cn.handler.Provider()
	if cd.Header.Height <= cp.Height() {
		return ErrInvalidHeight
	}
	if err := cn.handler.OnScreening(cd); err != nil {
		return err
	}
	return nil
}

// Process procceses the data and appends the data to the chain
func (cn *Chain) Process(cd *Data) error {
	cn.closeLock.RLock()
	defer cn.closeLock.RUnlock()
	if cn.isClose {
		return ErrChainClosed
	}

	cn.Lock()
	defer cn.Unlock()

	cp := cn.handler.Provider()
	height := cp.Height()
	if cd.Header.Height != height+1 {
		return ErrInvalidHeight
	}

	if height == 0 {
		if cd.Header.Version <= 0 {
			return ErrInvalidVersion
		}
		if !cd.Header.PrevHash.Equal(cp.PrevHash()) {
			return ErrInvalidPrevHash
		}
	} else {
		PrevHeader, err := cp.Header(height)
		if err != nil {
			return err
		}
		if cd.Header.Version < PrevHeader.Version {
			return ErrInvalidVersion
		}
		if !cd.Header.PrevHash.Equal(PrevHeader.Hash()) {
			return ErrInvalidPrevHash
		}
	}
	BodyHash := hash.DoubleHash(cd.Body)
	if !BodyHash.Equal(cd.Header.BodyHash) {
		return ErrInvalidBodyHash
	}
	if err := cn.handler.OnProcess(cd); err != nil {
		return err
	}
	return nil
}
