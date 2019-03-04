package chain

import (
	"github.com/fletaio/common/hash"
)

// Provider is a interface to give a chain data
type Provider interface {
	CreateHeader() Header
	CreateBody() Body
	Version() uint16
	Height() uint32
	LastHash() hash.Hash256
	Hash(height uint32) (hash.Hash256, error)
	Header(height uint32) (Header, error)
	Data(height uint32) (*Data, error)
}
