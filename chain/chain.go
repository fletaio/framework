package chain

import (
	"github.com/fletaio/common"
)

// Chain validates and stores the chain data
type Chain interface {
	Provider() Provider
	Screening(cd *Data) error
	CheckFork(ch Header, sigs []common.Signature) error
	Process(cd *Data, UserData interface{}) error
	DebugLog(args ...interface{})
}
