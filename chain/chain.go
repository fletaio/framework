package chain

import (
	"git.fleta.io/fleta/common"
)

// Chain validates and stores the chain data
type Chain interface {
	Provider() Provider
	Screening(cd *Data) error
	CheckFork(ch Header, sigs []common.Signature) error
	Process(cd *Data, UserData interface{}) error
}
