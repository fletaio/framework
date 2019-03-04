package evilnode

import (
	"io"
	"time"

	"github.com/fletaio/common/util"
)

// ConnectionInfo is struct for manage evel scroe
type ConnectionInfo struct {
	Addr      string
	EvilScore uint16
	Time      time.Time
}

// WriteTo is a serialization function
func (pi *ConnectionInfo) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	{
		bsAddr := []byte(pi.Addr)

		toLen := uint8(len(bsAddr))
		n, err := util.WriteUint8(w, toLen)
		if err != nil {
			return wrote, err
		}
		wrote += n

		nint, err := w.Write(bsAddr)
		if err != nil {
			return wrote, err
		}
		wrote += int64(nint)
	}

	{
		bs := util.Uint16ToBytes(pi.EvilScore)
		n, err := w.Write(bs)
		if err != nil {
			return wrote, err
		}
		wrote += int64(n)
	}

	{
		bs, err := pi.Time.MarshalText()
		if err != nil {
			return wrote, err
		}

		toLen := uint8(len(bs))
		n64, err := util.WriteUint8(w, toLen)
		if err != nil {
			return wrote, err
		}
		wrote += n64

		n, err := w.Write(bs)
		if err != nil {
			return wrote, err
		}
		wrote += int64(n)
	}
	return wrote, nil
}

// ReadFrom is a deserialization function
func (pi *ConnectionInfo) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	{
		v, n, err := util.ReadUint8(r)
		if err != nil {
			return read, err
		}
		read += n
		bsLen := v
		bsBs := make([]byte, bsLen)

		nInt, err := r.Read(bsBs)
		if err != nil {
			return read, err
		}
		read += int64(nInt)

		pi.Addr = string(bsBs)
	}
	{
		v, n, err := util.ReadUint16(r)
		if err != nil {
			return read, err
		}
		read += n

		pi.EvilScore = v
	}

	{
		v, n, err := util.ReadUint8(r)
		if err != nil {
			return read, err
		}
		read += n
		bsLen := v
		bs := make([]byte, bsLen)

		nInt, err := r.Read(bs)
		if err != nil {
			return read, err
		}
		read += int64(nInt)

		pi.Time.UnmarshalText(bs)
	}
	return read, nil
}
