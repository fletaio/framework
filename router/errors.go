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
	ErrDoNotRequestToEvelNode    = errors.New("do not request to evel node")
)
