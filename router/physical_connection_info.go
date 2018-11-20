package router

import (
	"io"

	"git.fleta.io/fleta/common/util"
)

// PhysicalConnectionInfo is struct for manage evel scroe
type PhysicalConnectionInfo struct {
	Addr      string
	EvilScore uint16
}

// WriteTo is a serialization function
func (pi *PhysicalConnectionInfo) WriteTo(w io.Writer) (int64, error) {
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
	return wrote, nil
}

// ReadFrom is a deserialization function
func (pi *PhysicalConnectionInfo) ReadFrom(r io.Reader) (int64, error) {
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

	return read, nil
}
