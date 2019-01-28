package message

import (
	"errors"
)

// message error list
var (
	ErrInvalidLength         = errors.New("invalid length")
	ErrAlreadyAppliedMessage = errors.New("already applied message")
	ErrNotAppliedMessage     = errors.New("not applied message")
	ErrUnhandledMessage      = errors.New("unhandled message")
	ErrInvalidMessage        = errors.New("invalid message")
	ErrUnknownMessage        = errors.New("unknown message")
)
