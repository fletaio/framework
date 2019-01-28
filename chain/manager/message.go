package manager

import (
	"io"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/common/hash"
	"git.fleta.io/fleta/common/util"
	"git.fleta.io/fleta/framework/chain"
	"git.fleta.io/fleta/framework/message"
)

// types
var (
	HeaderMessageType  = message.DefineType("chain.HeaderMessage")
	DataMessageType    = message.DefineType("chain.DataMessage")
	RequestMessageType = message.DefineType("chain.RequestMessage")
	StatusMessageType  = message.DefineType("chain.StatusMessage")
)

// HeaderMessage used to send a chain header to a peer
type HeaderMessage struct {
	Header     *chain.Header
	Signatures []common.Signature
}

// Type returns message type
func (msg *HeaderMessage) Type() message.Type {
	return HeaderMessageType
}

// WriteTo is a serialization function
func (msg *HeaderMessage) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	if n, err := msg.Header.WriteTo(w); err != nil {
		return wrote, err
	} else {
		wrote += n
	}

	if n, err := util.WriteUint8(w, uint8(len(msg.Signatures))); err != nil {
		return wrote, err
	} else {
		wrote += n
		for _, sig := range msg.Signatures {
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
func (msg *HeaderMessage) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	if n, err := msg.Header.ReadFrom(r); err != nil {
		return read, err
	} else {
		read += n
	}

	if Len, n, err := util.ReadUint8(r); err != nil {
		return read, err
	} else {
		read += n
		msg.Signatures = make([]common.Signature, 0, Len)
		for i := 0; i < int(Len); i++ {
			var sig common.Signature
			if n, err := sig.ReadFrom(r); err != nil {
				return read, err
			} else {
				read += n
				msg.Signatures = append(msg.Signatures, sig)
			}
		}
	}
	return read, nil
}

// DataMessage used to send a chain data to a peer
type DataMessage struct {
	Data *chain.Data
}

// Type returns message type
func (msg *DataMessage) Type() message.Type {
	return DataMessageType
}

// WriteTo is a serialization function
func (msg *DataMessage) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	if n, err := msg.Data.WriteTo(w); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	return wrote, nil
}

// ReadFrom is a deserialization function
func (msg *DataMessage) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	if n, err := msg.Data.ReadFrom(r); err != nil {
		return read, err
	} else {
		read += n
	}
	return read, nil
}

// RequestMessage used to request a chain data to a peer
type RequestMessage struct {
	Height uint32
}

// Type returns message type
func (msg *RequestMessage) Type() message.Type {
	return RequestMessageType
}

// WriteTo is a serialization function
func (msg *RequestMessage) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	if n, err := util.WriteUint32(w, msg.Height); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	return wrote, nil
}

// ReadFrom is a deserialization function
func (msg *RequestMessage) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	if v, n, err := util.ReadUint32(r); err != nil {
		return read, err
	} else {
		read += n
		msg.Height = v
	}
	return read, nil
}

// StatusMessage used to provide the chain information to a peer
type StatusMessage struct {
	Version  uint16
	Height   uint32
	PrevHash hash.Hash256
}

// Type returns message type
func (msg *StatusMessage) Type() message.Type {
	return StatusMessageType
}

// WriteTo is a serialization function
func (msg *StatusMessage) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	if n, err := util.WriteUint16(w, msg.Version); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := util.WriteUint32(w, msg.Height); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := msg.PrevHash.WriteTo(w); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	return wrote, nil
}

// ReadFrom is a deserialization function
func (msg *StatusMessage) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	if v, n, err := util.ReadUint16(r); err != nil {
		return read, err
	} else {
		read += n
		msg.Version = v
	}
	if v, n, err := util.ReadUint32(r); err != nil {
		return read, err
	} else {
		read += n
		msg.Height = v
	}
	if n, err := msg.PrevHash.ReadFrom(r); err != nil {
		return read, err
	} else {
		read += n
	}
	return read, nil
}
