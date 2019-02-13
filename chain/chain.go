package chain

import (
	"io"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/framework/chain/mesh"
	"git.fleta.io/fleta/framework/message"
)

// Chain validates and stores the chain data
type Chain interface {
	Init() error
	Close()
	Provider() Provider
	Screening(cd *Data) error
	CheckFork(ch Header, sigs []common.Signature) error
	Process(cd *Data, UserData interface{}) error
	OnRecv(p mesh.Peer, t message.Type, r io.Reader) error
}
