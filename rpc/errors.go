package rpc

import (
	"errors"
)

// errors
var (
	ErrInvalidArgument      = errors.New("invalid argument")
	ErrInvalidArgumentIndex = errors.New("invalid argument index")
	ErrInvalidArgumentType  = errors.New("invalid argument type")
)
