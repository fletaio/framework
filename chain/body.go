package chain

import (
	"io"
)

// Body includes chain informations
type Body interface {
	io.WriterTo
	io.ReaderFrom
}
