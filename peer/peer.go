package peer

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"

	"git.fleta.io/fleta/framework/log"
	"git.fleta.io/fleta/framework/message"
)

//Peer is manages connections between nodes that cause logical connections.
type Peer interface {
	net.Conn
	Send(m message.Message)
	PingTime() time.Duration
	SetPingTime(t time.Duration)
	ConnectedTime() int64
	IsClose() bool
	ID() string
	NetAddr() string
}

type onRecv func(p *peer, t message.Type)
type peer struct {
	net.Conn
	pingTime time.Duration
	score    int64
	mm       *message.Manager
	closed   bool

	registeredTime int64
	connectedTime  int64
	deletePeer     func(addr string)

	writeLock    sync.Mutex
	eventHandler onRecv
}

//NewPeer is the peer creator.
func newPeer(conn net.Conn, pingTime time.Duration, mm *message.Manager, deletePeer func(addr string), OnRecvEventHandler onRecv) Peer {
	p := &peer{
		Conn:          conn,
		pingTime:      pingTime,
		mm:            mm,
		closed:        false,
		deletePeer:    deletePeer,
		connectedTime: time.Now().UnixNano(),
		eventHandler:  OnRecvEventHandler,
	}

	log.Info("add peer ", pingTime)

	go p.readPacket()

	return p
}

func (p *peer) ID() string {
	return p.NetAddr()
}

func (p *peer) NetAddr() string {
	// if addr, ok := p.Conn.RemoteAddr().(*net.TCPAddr); ok { //TCP 로 ip를 바로 획득 가능한경우
	// 	return addr.IP.String()
	// }

	// //:으로 나눈 마지막을 port로 취급
	// addrs := strings.Split(p.Conn.RemoteAddr().String(), ":")
	// addr := strings.Join(addrs[:len(addrs)-1], ":")

	// return addr
	return p.Conn.RemoteAddr().String()
}

func (p *peer) ConnectedTime() int64 {
	return p.connectedTime
}

func (p *peer) readPacket() {
	for !p.closed {
		BNum := make([]byte, 8)
		n, err := p.Read(BNum)
		if err != nil {
			if err != io.EOF {
				log.Error("recv read type error : ", err)
			}
			return
		}
		if n != 8 {
			log.Error("recv read type error : invalied packet length")
			return
		}

		mt := message.ByteToType(BNum)
		m, h, err := p.mm.ParseMessage(p, mt)
		if err != nil {
			if err != message.ErrUnknownMessage {
				// pass EventHandler
				p.eventHandler(p, mt)
				return
			}
			log.Error("recv parse message : ", err)
			return
		}
		if err := h(m); err != nil {
			log.Error("recv handle message : ", err)
			return
		}
	}
}

//Send conveys a message to the connected node.
func (p *peer) Send(m message.Message) {
	p.writeLock.Lock()
	defer p.writeLock.Unlock()

	mt := m.Type()

	bf := bytes.Buffer{}
	bf.Write(message.TypeToByte(mt))
	m.WriteTo(&bf)

	bs := bf.Bytes()
	p.Write(bs)
}

//PingTime return pingTime
func (p *peer) PingTime() time.Duration {
	return p.pingTime
}

//SetPingTime set pingTime
func (p *peer) SetPingTime(t time.Duration) {
	p.pingTime = t
}

//SetRegisteredTime set registeredTime
func (p *peer) SetRegisteredTime(t int64) {
	p.registeredTime = t
}

//IsClose returns closed
func (p *peer) IsClose() bool {
	return p.closed
}

//Close is used to break logical connections and delete stored peer data.
func (p *peer) Close() error {
	p.closed = true
	p.deletePeer(p.NetAddr())
	p.Conn.Close()

	return nil
}
