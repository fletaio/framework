package router

import "errors"

//router error list
var (
	ErrNotConnected              = errors.New("not connected")
	ErrMismatchHashSize          = errors.New("mismatch hash size")
	ErrNotFoundLogicalConnection = errors.New("not found logical connection")
	ErrListenFirst               = errors.New("not found listen address")
	ErrDuplicateAccept           = errors.New("cannot accept ")
	ErrNotMatchCompressionType   = errors.New("not matched compression type")
	ErrPacketNotStartedMagicword = errors.New("packet not started magicword")
	ErrInvalidIntegrity          = errors.New("invalid integrity")
	ErrNotFoundListener          = errors.New("not found listener")
	ErrCannotRequestToLocal      = errors.New("cannot request to local")
	ErrNotFoundPort              = errors.New("not found port")
	ErrCanNotConnectToEvilNode   = errors.New("can not connect to evil node")
	ErrWriteTimeout              = errors.New("write timeout")
	ErrNotHandshakeFormate       = errors.New("not handshake formate")
	ErrPeerTimeout               = errors.New("peer timeout")
)
