package message

import (
	"encoding/binary"
	"io"
	"sync"

	"git.fleta.io/fleta/common/hash"
	"git.fleta.io/fleta/framework/log"
)

//SendInterface is Default function of messenger
type SendInterface interface {
	Send(m Message)
}

//Type is message type
type Type uint64

//DefineType is return string type
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

//ByteToType returns a Type of the byte array
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

//Message TODO
type Message interface {
	io.WriterTo
	io.ReaderFrom
	GetType() Type
}

type creator func(r io.Reader) Message
type handler func(m Message) error

type messageTaker struct {
	creator creator
	handler handler
}

//Handler is a structure that stores data for message processing.
type Handler struct {
	messages     map[Type]messageTaker
	messagesLock sync.Mutex
}

//NewHandler is messageHandler creator
func NewHandler() *Handler {
	return &Handler{
		messages: make(map[Type]messageTaker),
	}
}

//MessageGenerator receives messages as Reader and processes them through the creator and handler.
func (mh *Handler) MessageGenerator(r io.Reader, mt Type) (Message, error) {
	mh.messagesLock.Lock()
	c, has := mh.messages[mt]
	mh.messagesLock.Unlock()
	if !has {
		return nil, ErrNotExistMessageTaker
	}

	m := c.creator(r)
	if err := c.handler(m); err != nil {
		return nil, err
	}
	return m, nil

}

//ApplyMessage is a function to register a message.
//Register author and handler by type to use when receiving messages.
func (mh *Handler) ApplyMessage(mt Type, c creator, h handler) error {
	mh.messagesLock.Lock()
	defer mh.messagesLock.Unlock()
	_, has := mh.messages[mt]
	if has {
		return ErrAlreadyAppliedMessage
	}

	mh.messages[mt] = messageTaker{
		creator: c,
		handler: h,
	}
	return nil
}

//DeleteMessage deletes registered messages.
func (mh *Handler) DeleteMessage(m Type) error {
	mh.messagesLock.Lock()
	defer mh.messagesLock.Unlock()
	_, has := mh.messages[m]
	if !has {
		return ErrNotAppliedMessage
	}
	delete(mh.messages, m)
	return nil
}
