package chain

import (
	"git.fleta.io/fleta/common"
)

// EventHandler is a event handler of the chain
type EventHandler interface {
	OnInit() error
	OnScreening(cd *Data) error
	OnCheckFork(ch *Header, sigs []common.Signature) error
	OnProcess(cd *Data, UserData interface{}) error
	Provider() Provider
	OnClose()
}
