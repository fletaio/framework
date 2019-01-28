package chain

import (
	"bytes"
	"io"

	"git.fleta.io/fleta/common/hash"
	"git.fleta.io/fleta/common/util"
)

// Header includes validation informations
type Header struct {
	Version   uint16
	Height    uint32
	PrevHash  hash.Hash256
	BodyHash  hash.Hash256
	Timestamp uint64
}

// Hash returns the hash value of it
func (ch *Header) Hash() hash.Hash256 {
	var buffer bytes.Buffer
	if _, err := ch.WriteTo(&buffer); err != nil {
		panic(err)
	}
	return hash.DoubleHash(buffer.Bytes())
}

// WriteTo is a serialization function
func (ch *Header) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	if n, err := util.WriteUint32(w, ch.Height); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := util.WriteUint16(w, ch.Version); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := ch.PrevHash.WriteTo(w); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := ch.BodyHash.WriteTo(w); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := util.WriteUint64(w, ch.Timestamp); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	return wrote, nil
}

// ReadFrom is a deserialization function
func (ch *Header) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	if v, n, err := util.ReadUint32(r); err != nil {
		return read, err
	} else {
		ch.Height = v
		read += n
	}
	if v, n, err := util.ReadUint16(r); err != nil {
		return read, err
	} else {
		ch.Version = v
		read += n
	}
	if n, err := ch.PrevHash.ReadFrom(r); err != nil {
		return read, err
	} else {
		read += n
	}
	if n, err := ch.BodyHash.ReadFrom(r); err != nil {
		return read, err
	} else {
		read += n
	}
	if v, n, err := util.ReadUint64(r); err != nil {
		return read, err
	} else {
		ch.Timestamp = v
		read += n
	}
	return read, nil
}
