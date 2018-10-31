package message

import (
	"errors"
)

// message error list
var (
	ErrInvalidLength         = errors.New("invalid length")
	ErrAlReadyAppliedMessage = errors.New("already applied message")
	ErrNotAppliedMessage     = errors.New("not applied message")
	ErrNotExistMessageTaker  = errors.New("not exist message taker")
)
