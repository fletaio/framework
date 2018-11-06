package peer

import (
	"bytes"
	"io"
	"sync"
	"time"

	"git.fleta.io/fleta/framework/log"
	"git.fleta.io/fleta/framework/message"
	"git.fleta.io/fleta/framework/router"
)

//Peer is a structure that manages connections between nodes that cause logical connections.
type Peer struct {
	router.Receiver
	pingTime time.Duration
	score    int64
	mh       *message.Handler
	closed   bool

	registeredTime int64
	connectedTime  int64
	deletePeer     func(addr string)

	writeLock sync.Mutex
}

//NewPeer is the peer creator.
func NewPeer(conn router.Receiver, mh *message.Handler, deletePeer func(addr string)) *Peer {
	p := &Peer{
		Receiver:      conn,
		pingTime:      -1,
		mh:            mh,
		closed:        false,
		deletePeer:    deletePeer,
		connectedTime: time.Now().UnixNano(),
	}

	go p.readPacket()

	return p
}

//ID returned peer ID
func (p *Peer) ID() string {
	return p.RemoteAddr().String()
}

func (p *Peer) readPacket() {
	for !p.closed {
		bs, err := p.Recv()
		if err != nil {
			if err != io.EOF {
				log.Error("recv err : ", err)
			}
			p.Close()
			break
		}

		go func(bs []byte) {
			buf := bytes.NewBuffer(bs)

			BNum := make([]byte, 8)
			n, err := buf.Read(BNum)
			if err != nil {
				log.Error("recv read type error : ", err)
				return
			}
			if n != 8 {
				log.Error("recv read type error : invalied packet length")
				return
			}

			mt := message.ByteToType(BNum)
			p.mh.MessageGenerator(buf, mt)
		}(bs)
	}
}

//Send conveys a message to the connected node.
func (p *Peer) Send(m message.Message) {
	p.writeLock.Lock()
	defer p.writeLock.Unlock()

	mt := m.GetType()
	p.Write(message.TypeToByte(mt))
	m.WriteTo(p)
	p.Flush()
}

//PingTime return pingTime
func (p *Peer) PingTime() time.Duration {
	return p.pingTime
}

//SetPingTime set pingTime
func (p *Peer) SetPingTime(t time.Duration) {
	p.pingTime = t
}

//SetRegisteredTime set registeredTime
func (p *Peer) SetRegisteredTime(t int64) {
	p.registeredTime = t
}

//IsClose returns closed
func (p *Peer) IsClose() bool {
	return p.closed
}

//Close is used to break logical connections and delete stored peer data.
func (p *Peer) Close() {
	p.closed = true
	p.deletePeer(p.RemoteAddr().String())
	p.Receiver.Close()

}
