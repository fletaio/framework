package chain

import "errors"

// errors
var (
	ErrAlreadyConnected   = errors.New("already connected")
	ErrAlreadyGenesised   = errors.New("already genesised")
	ErrForkDetected       = errors.New("fork detected")
	ErrInvalidVersion     = errors.New("invalid version")
	ErrInvalidPrevHash    = errors.New("invalid prev hash")
	ErrInvalidHeight      = errors.New("invalid height")
	ErrInvalidBodyHash    = errors.New("invalid body hash")
	ErrInvalidGenesisHash = errors.New("invalid genesis hash")
	ErrChainClosed        = errors.New("chain closed")
)
