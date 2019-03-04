package chain

import (
	"io"

	"github.com/fletaio/common"
	"github.com/fletaio/common/util"
)

// Data is a unit of the chain
type Data struct {
	Header     Header
	Body       Body
	Signatures []common.Signature
}

// WriteTo is a serialization function
func (cd *Data) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	if n, err := cd.Header.WriteTo(w); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := cd.Body.WriteTo(w); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := util.WriteUint8(w, uint8(len(cd.Signatures))); err != nil {
		return wrote, err
	} else {
		wrote += n
		for _, sig := range cd.Signatures {
			wrote += n
			if n, err := sig.WriteTo(w); err != nil {
				return wrote, err
			} else {
				wrote += n
			}
		}
	}
	return wrote, nil
}

// ReadFrom is a deserialization function
func (cd *Data) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	if n, err := cd.Header.ReadFrom(r); err != nil {
		return read, err
	} else {
		read += n
	}
	if n, err := cd.Body.ReadFrom(r); err != nil {
		return read, err
	} else {
		read += n
	}
	if Len, n, err := util.ReadUint8(r); err != nil {
		return read, err
	} else {
		read += n
		cd.Signatures = make([]common.Signature, 0, Len)
		for i := 0; i < int(Len); i++ {
			var sig common.Signature
			if n, err := sig.ReadFrom(r); err != nil {
				return read, err
			} else {
				read += n
				cd.Signatures = append(cd.Signatures, sig)
			}
		}
	}
	return read, nil
}
