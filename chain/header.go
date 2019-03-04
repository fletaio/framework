package chain

import (
	"io"

	"github.com/fletaio/common/hash"
	"github.com/fletaio/common/util"
)

// Header includes validation informations
type Header interface {
	io.WriterTo
	io.ReaderFrom
	Version() uint16
	Height() uint32
	PrevHash() hash.Hash256
	Timestamp() uint64
	Hash() hash.Hash256
}

// Base implements common functions of a header
type Base struct {
	Version_   uint16
	Height_    uint32
	PrevHash_  hash.Hash256
	Timestamp_ uint64
}

// Version returns the version of the header
func (ch *Base) Version() uint16 {
	return ch.Version_
}

// Height returns the height of the header
func (ch *Base) Height() uint32 {
	return ch.Height_
}

// PrevHash returns the prev hash of the header
func (ch *Base) PrevHash() hash.Hash256 {
	return ch.PrevHash_
}

// Timestamp returns the timestamp of the header
func (ch *Base) Timestamp() uint64 {
	return ch.Timestamp_
}

// WriteTo is a serialization function
func (ch *Base) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	if n, err := util.WriteUint32(w, ch.Height_); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := util.WriteUint16(w, ch.Version_); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := ch.PrevHash_.WriteTo(w); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := util.WriteUint64(w, ch.Timestamp_); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	return wrote, nil
}

// ReadFrom is a deserialization function
func (ch *Base) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	if v, n, err := util.ReadUint32(r); err != nil {
		return read, err
	} else {
		ch.Height_ = v
		read += n
	}
	if v, n, err := util.ReadUint16(r); err != nil {
		return read, err
	} else {
		ch.Version_ = v
		read += n
	}
	if n, err := ch.PrevHash_.ReadFrom(r); err != nil {
		return read, err
	} else {
		read += n
	}
	if v, n, err := util.ReadUint64(r); err != nil {
		return read, err
	} else {
		ch.Timestamp_ = v
		read += n
	}
	return read, nil
}
