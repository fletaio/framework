package peermessage

import (
	"io"
	"time"

	"git.fleta.io/fleta/common/util"
	"git.fleta.io/fleta/framework/message"
)

// PeerList is struct of peer list
type PeerList struct {
	Request bool
	From    string
	List    map[string]ConnectInfo
}

// ConnectInfo is a structure of connection information that includes ping time and score board.
type ConnectInfo struct {
	Address        string
	PingTime       time.Duration
	PingScoreBoard *ScoreBoardMap
}

// NewConnectInfo is creator of ConnectInfo
func NewConnectInfo(addr string, t time.Duration) ConnectInfo {
	return ConnectInfo{
		Address:        addr,
		PingTime:       t,
		PingScoreBoard: &ScoreBoardMap{},
	}
}

// WriteTo is a serialization function
func (ci *ConnectInfo) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	{
		n, err := util.WriteUint64(w, uint64(ci.PingTime))
		if err != nil {
			return wrote, err
		}
		wrote += n
	}
	{
		bs := []byte(ci.Address)

		bsLen := uint8(len(bs))
		n, err := util.WriteUint8(w, bsLen)
		if err != nil {
			return wrote, err
		}
		wrote += n

		nint, err := w.Write(bs)
		if err != nil {
			return wrote, err
		}
		wrote += int64(nint)
	}
	return wrote, nil
}

// ReadFrom is a deserialization function
func (ci *ConnectInfo) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	{
		v, n, err := util.ReadUint64(r)
		if err != nil {
			return read, err
		}
		read += n

		ci.PingTime = time.Duration(v)
	}

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

		ci.Address = string(bsBs)
	}
	return read, nil
}

// Score is calculated and returned based on the ping time.
func (ci *ConnectInfo) Score() (score int64) {
	ci.PingScoreBoard.Range(func(addr string, t time.Duration) bool {
		score += int64(t)
		return true
	})
	score /= int64(ci.PingScoreBoard.Len())
	return score
}

const (
	requestTrue  = byte('t')
	requestFalse = byte('f')
)

// PeerListCreator reconstructs the PeerList from the Reader.
func PeerListCreator(r io.Reader, mt message.Type) (message.Message, error) {
	p := &PeerList{}
	if _, err := p.ReadFrom(r); err != nil {
		return nil, err
	}
	return p, nil
}

// PeerListMessageType is define message type
var PeerListMessageType message.Type

func init() {
	PeerListMessageType = message.DefineType("PeerList")
}

// SendRequestPeerList transfers the peer list structure to a given peer
func SendRequestPeerList(p message.Sender, from string) {
	peerList := &PeerList{
		Request: true,
		From:    from,
	}
	p.Send(peerList)
}

// Type is the basic function of "message".
// Returns the type of message.
func (p *PeerList) Type() message.Type {
	return PeerListMessageType
}

// WriteTo is a serialization function
func (p *PeerList) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	{
		n, err := util.WriteString(w, p.From)
		if err != nil {
			return wrote, err
		}
		wrote += n
	}

	{
		b := uint8(requestFalse)
		if p.Request == true {
			b = uint8(requestTrue)
		}
		n, err := util.WriteUint8(w, b)
		if err != nil {
			return wrote, err
		}
		wrote += n
	}

	{
		listLen := uint32(len(p.List))
		n, err := util.WriteUint32(w, listLen)
		if err != nil {
			return wrote, err
		}
		wrote += n

		for _, info := range p.List {
			info.WriteTo(w)
		}
	}

	return wrote, nil
}

// ReadFrom is a deserialization function
func (p *PeerList) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	{
		v, n, err := util.ReadString(r)
		if err != nil {
			return read, err
		}
		read += n
		p.From = v
	}

	{
		v, n, err := util.ReadUint8(r)
		if err != nil {
			return read, err
		}
		read += n
		if v == requestTrue {
			p.Request = true
		} else {
			p.Request = false
		}
	}

	{
		v, n, err := util.ReadUint32(r)
		if err != nil {
			return read, err
		}
		read += n
		listLen := v

		list := make(map[string]ConnectInfo)

		for i := uint32(0); i < listLen; i++ {
			var info ConnectInfo
			info.ReadFrom(r)
			list[info.Address] = info
		}

		p.List = list
	}

	return read, nil
}
