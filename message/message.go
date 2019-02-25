package message

import (
	"io"
	"sync"

	"git.fleta.io/fleta/common/hash"
	"git.fleta.io/fleta/common/util"
	"git.fleta.io/fleta/framework/log"
)

// Sender is Default function of messenger
type Sender interface {
	Send(m Message) error
}

// Type is message type
type Type uint64

var gDefineMap = map[Type]string{}

// DefineType is return string type
func DefineType(Name string) Type {
	h := hash.DoubleHash([]byte(Name))
	t := Type(util.BytesToUint64(h[:8]))
	old, has := gDefineMap[t]
	if has {
		panic("Type is collapsed (" + old + ", " + Name + ")")
	}
	gDefineMap[t] = Name
	return t
}

// NameOfType returns the name of the type
func NameOfType(t Type) string {
	return gDefineMap[t]
}

// Message is a message for communicating between services
type Message interface {
	io.WriterTo
	io.ReaderFrom
	Type() Type
}

// Creator is a message creator function type
type Creator func(r io.Reader, mt Type) (Message, error)

// Manager is a structure that stores data for message processing.
type Manager struct {
	messageMap     map[Type]Creator
	messageMapLock sync.Mutex
}

// NewManager returns a Manager
func NewManager() *Manager {
	return &Manager{
		messageMap: make(map[Type]Creator),
	}
}

// ParseMessage receives the data stream as a Reader and processes them through the creator and returns the message.
func (mm *Manager) ParseMessage(r io.Reader, mt Type) (Message, error) {
	mm.messageMapLock.Lock()
	log.Info("ParseMessage", NameOfType(mt), mt)
	c, has := mm.messageMap[mt]
	mm.messageMapLock.Unlock()
	if !has {
		return nil, ErrUnknownMessage
	}
	msg, err := c(r, mt)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// SetCreator is a function to register a message.
// Register author and handler by type to use when receiving messageMap.
func (mm *Manager) SetCreator(mt Type, c Creator) error {
	mm.messageMapLock.Lock()
	defer mm.messageMapLock.Unlock()
	log.Info("SetCreator", NameOfType(mt), mt)
	_, has := mm.messageMap[mt]
	if has {
		return ErrAlreadyAppliedMessage
	}

	mm.messageMap[mt] = c
	return nil
}

// DeleteMessage deletes registered messageMap.
func (mm *Manager) DeleteMessage(m Type) error {
	mm.messageMapLock.Lock()
	defer mm.messageMapLock.Unlock()
	_, has := mm.messageMap[m]
	if !has {
		return ErrNotAppliedMessage
	}
	delete(mm.messageMap, m)
	return nil
}
