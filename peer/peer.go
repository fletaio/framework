package peer

import (
	"bytes"
	"sync"
	"time"

	"github.com/fletaio/common/util"
	"github.com/fletaio/framework/log"
	"github.com/fletaio/framework/message"
	"github.com/fletaio/framework/router"
)

//Peer is manages connections between nodes that cause logical connections.
type Peer interface {
	router.Conn
	Send(m message.Message) error
	PingTime() time.Duration
	SetPingTime(t time.Duration)
	ConnectedTime() int64
	IsClose() bool
	Remove()
	NetAddr() string
}

type onRecv func(p *peer, t message.Type) error
type peer struct {
	router.Conn
	pingTime time.Duration
	score    int64
	closed   bool

	registeredTime int64
	connectedTime  int64
	deletePeer     func(addr string)

	writeLock          sync.Mutex
	onRecvEventHandler onRecv
}

//NewPeer is the peer creator.
func newPeer(conn router.Conn, pingTime time.Duration, deletePeer func(addr string), OnRecvEventHandler onRecv) *peer {
	p := &peer{
		Conn:               conn,
		pingTime:           pingTime,
		closed:             false,
		deletePeer:         deletePeer,
		connectedTime:      time.Now().UnixNano(),
		onRecvEventHandler: OnRecvEventHandler,
	}

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
	// addrs := strings.Split(p.Conn.ID(), ":")
	// addr := strings.Join(addrs[:len(addrs)-1], ":")

	// return addr
	return p.Conn.ID()
}

func (p *peer) ConnectedTime() int64 {
	return p.connectedTime
}

func (p *peer) Start() {
	go p.readPacket()
}

func (p *peer) readPacket() {
	defer func() {
		p.Close()
	}()
	for !p.closed {
		t, n, err := util.ReadUint64(p)
		if n != 0 && message.NameOfType(message.Type(t)) == "" {
			log.Error("not defind message type recived", t)
			p.Close()
			return
		}
		if err != nil {
			// if err != io.EOF {
			// 	log.Error("recv read type error : ", err)
			// }
			return
		}
		if n != 8 {
			log.Error("recv read type error : invalied packet length")
			return
		}
		err = p.onRecvEventHandler(p, message.Type(t))
		if err != nil {
			log.Error("onRecv error : ", err)
			return
		}
	}
}

//Send conveys a message to the connected node.
func (p *peer) Send(m message.Message) error {
	p.writeLock.Lock()
	defer p.writeLock.Unlock()

	bf := bytes.Buffer{}
	_, err := util.WriteUint64(&bf, uint64(m.Type()))
	if err != nil {
		return err
	}
	_, err = m.WriteTo(&bf)
	if err != nil {
		return err
	}

	_, err = p.Write(bf.Bytes())
	if err != nil {
		return err
	}
	return nil
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

//Remove is Close connection when removed peer
func (p *peer) Remove() {
	p.Conn.Close()
}

//Close is used to break logical connections and delete stored peer data.
func (p *peer) Close() error {
	p.closed = true
	p.deletePeer(p.NetAddr())
	p.Conn.Close()

	return nil
}
