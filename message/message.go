package message

import (
	"encoding/binary"
	"io"
	"sync"

	"git.fleta.io/fleta/common/hash"
	"git.fleta.io/fleta/framework/log"
)

// SendInterface is Default function of messenger
type SendInterface interface {
	Send(m Message)
}

// Type is message type
type Type uint64

// DefineType is return string type
func DefineType(t string) Type {
	h := hash.Hash([]byte(t))
	return Type(binary.BigEndian.Uint64(h[:8]))
}

// TypeToByte returns a byte array of the Type
func TypeToByte(t Type) []byte {
	bs := make([]byte, 8)
	binary.BigEndian.PutUint64(bs, uint64(t))
	return bs
}

// ByteToType returns a Type of the byte array
func ByteToType(b []byte) Type {
	if l := len(b); l > 8 {
		log.Panic("Check")
	} else if l < 8 {
		bs := make([]byte, 8)
		copy(bs[:], b)
		return Type(binary.BigEndian.Uint64(bs))
	}
	return Type(binary.BigEndian.Uint64(b))
}

// Message TODO
type Message interface {
	io.WriterTo
	io.ReaderFrom
	Type() Type
}

// Creator TODO
type Creator func(r io.Reader, mt Type) (Message, error)

// Handler TODO
type Handler func(m Message) error

type item struct {
	creator Creator
	handler Handler
}

// Manager is a structure that stores data for message processing.
type Manager struct {
	messageHash     map[Type]*item
	messageHashLock sync.Mutex
}

// NewManager returns a Manager
func NewManager() *Manager {
	return &Manager{
		messageHash: make(map[Type]*item),
	}
}

// ParseMessage receives the data stream as a Reader and processes them through the creator and returns the message and the handler.
func (mm *Manager) ParseMessage(r io.Reader, mt Type) (Message, Handler, error) {
	mm.messageHashLock.Lock()
	c, has := mm.messageHash[mt]
	mm.messageHashLock.Unlock()
	if !has {
		return nil, nil, ErrUnknownMessage
	}
	msg, err := c.creator(r, mt)
	if err != nil {
		return nil, nil, err
	}
	return msg, c.handler, nil
}

// ApplyMessage is a function to register a message.
// Register author and handler by type to use when receiving messageHash.
func (mm *Manager) ApplyMessage(mt Type, c Creator, h Handler) error {
	mm.messageHashLock.Lock()
	defer mm.messageHashLock.Unlock()
	_, has := mm.messageHash[mt]
	if has {
		return ErrAlreadyAppliedMessage
	}

	mm.messageHash[mt] = &item{
		creator: c,
		handler: h,
	}
	return nil
}

// DeleteMessage deletes registered messageHash.
func (mm *Manager) DeleteMessage(m Type) error {
	mm.messageHashLock.Lock()
	defer mm.messageHashLock.Unlock()
	_, has := mm.messageHash[m]
	if !has {
		return ErrNotAppliedMessage
	}
	delete(mm.messageHash, m)
	return nil
}
