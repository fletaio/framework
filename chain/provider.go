package chain

import (
	"git.fleta.io/fleta/common/hash"
)

// Provider is a interface to give a chain data
type Provider interface {
	Version() uint16
	Height() uint32
	PrevHash() hash.Hash256
	Hash(height uint32) (hash.Hash256, error)
	Header(height uint32) (*Header, error)
	Data(height uint32) (*Data, error)
}