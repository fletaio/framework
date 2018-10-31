package message

import (
	"io"

	"git.fleta.io/fleta/common/util"
	"git.fleta.io/fleta/framework/message"
)

//Ping is struct of ping message
type Ping struct {
	Time uint32
	From string
	To   string
}

//PingCreator is ping creator
func PingCreator(r io.Reader) message.Message {
	p := &Ping{}
	p.ReadFrom(r)
	return p
}

//PingMessageType is unique message type.
var PingMessageType message.Type

func init() {
	PingMessageType = message.DefineType("ping")
}

//SendPing creates a "ping" and sends a message.
func SendPing(p message.SendInterfase, time uint32, from, to string) {
	ping := &Ping{
		Time: time,
		From: from,
		To:   to,
	}
	p.Send(ping)
}

//GetType is the basic function of "message".
//Returns the type of message.
func (ping *Ping) GetType() message.Type {
	return PingMessageType
}

//WriteTo is a serialization function
func (ping *Ping) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	n, err := util.WriteUint32(w, ping.Time)
	if err != nil {
		return wrote, err
	}
	wrote += n

	{
		bsFrom := []byte(ping.From)

		fromLen := uint8(len(bsFrom))
		n, err = util.WriteUint8(w, fromLen)
		if err != nil {
			return wrote, err
		}
		wrote += n

		nint, err := w.Write(bsFrom)
		if err != nil {
			return wrote, err
		}
		wrote += int64(nint)
	}

	{
		bsTo := []byte(ping.To)

		toLen := uint8(len(bsTo))
		n, err = util.WriteUint8(w, toLen)
		if err != nil {
			return wrote, err
		}
		wrote += n

		nint, err := w.Write(bsTo)
		if err != nil {
			return wrote, err
		}
		wrote += int64(nint)
	}

	return wrote, nil
}

// ReadFrom is a deserialization function
func (ping *Ping) ReadFrom(r io.Reader) (int64, error) {
	var read int64

	{
		v, n, err := util.ReadUint32(r)
		if err != nil {
			return read, err
		}
		read += n
		ping.Time = v
	}

	{
		v, n, err := util.ReadUint8(r)
		if err != nil {
			return read, err
		}
		read += n
		fromLen := v
		fromBs := make([]byte, fromLen)

		nInt, err := r.Read(fromBs)
		if err != nil {
			return read, err
		}
		read += int64(nInt)
		ping.From = string(fromBs)
	}

	{
		v, n, err := util.ReadUint8(r)
		if err != nil {
			return read, err
		}
		read += n
		toLen := v
		toBs := make([]byte, toLen)

		nInt, err := r.Read(toBs)
		if err != nil {
			return read, err
		}
		read += int64(nInt)
		ping.To = string(toBs)
	}

	return read, nil
}
