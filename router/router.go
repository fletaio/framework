package router

import (
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/fletaio/framework/router/evilnode"

	"github.com/fletaio/common"
	"github.com/fletaio/network"

	"github.com/fletaio/framework/log"
)

// Config is router config
type Config struct {
	Network        string
	Address        string
	Port           int
	EvilNodeConfig evilnode.Config
}

type TypeIs bool

const (
	IsAccept TypeIs = TypeIs(true)
	IsDial   TypeIs = TypeIs(false)
)

//Router that converts external connections to logical connections.
type Router interface {
	Listen() error
	Request(addrStr string) error
	Accept() (Conn, time.Duration, error)
	Localhost() string
	EvilNodeManager() *evilnode.Manager
	Conf() *Config
	ConnList() []string
	WaitHandshackConnList() []string
}

type router struct {
	Config                *Config
	ChainCoord            *common.Coordinate
	localhost             string
	evilNodeManager       *evilnode.Manager
	listener              net.Listener
	AcceptConnChan        chan *RouterConn
	ConnMap               map[string]*RouterConn
	ConnMapLock           *NamedLock
	WaitHandshackConn     map[string]struct{}
	WaitHandshackConnLock *NamedLock
}

// NewRouter is creator of router
func NewRouter(Config *Config, ChainCoord *common.Coordinate) (Router, error) {
	r := &router{
		Config:          Config,
		ChainCoord:      ChainCoord,
		evilNodeManager: evilnode.NewManager(&Config.EvilNodeConfig),
		AcceptConnChan:  make(chan *RouterConn),
		ConnMap:         map[string]*RouterConn{},
		ConnMapLock:     NewNamedLock("ConnMap"),

		WaitHandshackConn:     map[string]struct{}{},
		WaitHandshackConnLock: NewNamedLock("WaitHandshackConn"),
	}
	return r, nil
}

func (r *router) Conf() *Config {
	return r.Config
}

//AddListen registers a logical connection as a waiting-for-connect condition.
func (r *router) Listen() error {
	listenAddr := ":" + strconv.Itoa(r.Config.Port)
	l, err := network.Listen(r.Config.Network, listenAddr)
	if err != nil {
		return err
	}
	r.listener = l
	localhost := l.Addr().String()
	if r.localhost == "" {
		if !strings.HasPrefix(localhost, ":") && !strings.HasPrefix(localhost, "[") {
			r.localhost = localhost
		}
	}

	go r.listening()

	return nil
}

//Request requests the connection by entering the address when a logical connection is required.
//The chain coordinates support the connection between subchains.
func (r *router) Request(addr string) error {
	if r.localhost != "" && strings.HasPrefix(addr, r.localhost) {
		return ErrCannotRequestToLocal
	}
	if r.evilNodeManager.IsBanNode(addr) {
		return ErrCanNotConnectToEvilNode
	}

	conn, err := network.DialTimeout(r.Config.Network, addr, time.Second*2)
	if err != nil {
		if conn != nil {
			conn.Close()
		}
		return err
	}

	_, err = r.incommingConn(conn, IsDial)
	if err != nil {
		conn.Close()
		return err
	}

	return nil
}

// Accept returns a logical connection when an external connection request is received.
func (r *router) Accept() (Conn, time.Duration, error) {
	receiver := <-r.AcceptConnChan
	var c Conn
	c = receiver
	return c, receiver.pingTime, nil
}

func (r *router) EvilNodeManager() *evilnode.Manager {
	return r.evilNodeManager
}

func (r *router) Localhost() string {
	return r.localhost
}

func (r *router) setLocalhost(l string) {
	addr, _ := removePort(l)
	r.localhost = addr
}

func (r *router) listening() {
	for {
		conn, err := r.listener.Accept()
		func(conn net.Conn) {

			if err != nil {
				if conn != nil {
					conn.Close()
				}
				log.Error("router run err : ", err)
				return
			}
			_, err = r.incommingConn(conn, IsAccept)
			if err != nil {
				conn.Close()
				if err != ErrCanNotConnectToEvilNode && err != io.EOF {
					log.Error("incommingConn err", err)
				}
			}
		}(conn)
	}
}

func (r *router) WaitHandshackConnList() []string {
	r.WaitHandshackConnLock.Lock("list")
	defer r.WaitHandshackConnLock.Unlock()

	l := []string{}
	for k, _ := range r.WaitHandshackConn {
		l = append(l, k)
	}

	return l
}

func (r *router) ConnList() []string {
	s := []string{}
	r.ConnMapLock.RLock("ConnList")
	for k, _ := range r.ConnMap {
		k = "r" + strings.Replace(k, "testid", "", 0)
		s = append(s, k)
	}
	r.ConnMapLock.RUnlock()
	return s
}

func (r *router) incommingConn(conn net.Conn, typeis TypeIs) (*RouterConn, error) {
	if r.localhost == "" {
		r.setLocalhost(conn.LocalAddr().String())
	}
	addr := conn.RemoteAddr().String()
	if raddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		addr = raddr.IP.String()
	} else {
		addrs := strings.Split(addr, ":")
		addr = strings.Join(addrs[:len(addrs)-1], ":")
	}

	r.WaitHandshackConnLock.Lock("check")
	_, has := r.WaitHandshackConn[addr]
	if !has {
		r.WaitHandshackConn[addr] = struct{}{}
		defer func(addr string) {
			r.WaitHandshackConnLock.Lock("delete")
			delete(r.WaitHandshackConn, addr)
			r.WaitHandshackConnLock.Unlock()
		}(addr)
	}
	r.WaitHandshackConnLock.Unlock()
	if has {
		return nil, ErrDuplicateAccept
	}

	if r.evilNodeManager.IsBanNode(addr) {
		return nil, ErrCanNotConnectToEvilNode
	}

	pc := newRouterConn(addr, conn, r)

	errCh := make(chan error)
	go func(pc *RouterConn) {
		var err error
		if typeis == IsDial {
			pc.handshakeSend(r.ChainCoord)
			_, err = pc.handshakeRecv()
		} else {
			var cc *common.Coordinate
			cc, err = pc.handshakeRecv()
			if err == nil {
				if cc.Equal(r.ChainCoord) {
					pc.handshakeSend(r.ChainCoord)
				} else {
					err = ErrMismatchCoordinate
				}
			}
		}
		errCh <- err
	}(pc)

	deadTimer := time.NewTimer(5 * time.Second)
	var endErr error
	select {
	case <-deadTimer.C:
		endErr = ErrPeerTimeout
	case err := <-errCh:
		deadTimer.Stop()
		if err != nil {
			endErr = err
		}
	}

	if endErr != nil {
		pc.Close()
		return nil, endErr
	}

	{
		r.ConnMapLock.Lock("incommingConn")
		oldPConn, has := r.ConnMap[addr]
		if has {
			oldPConn.LockFreeClose()
		}
		r.ConnMap[addr] = pc
		r.AcceptConnChan <- pc
		r.ConnMapLock.Unlock()
	}

	return pc, nil
}

func (r *router) chainCoord() *common.Coordinate {
	return r.ChainCoord
}

func (r *router) localAddress() string {
	return r.Config.Address
}

func (r *router) port() int {
	return r.Config.Port
}

func (r *router) acceptConn(pc *RouterConn) error {
	return nil
}

func (r *router) unsafeRemoveRouterConn(conn net.Conn) {
	addr := conn.RemoteAddr().String()
	if raddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		addr = raddr.IP.String()
	} else {
		addrs := strings.Split(addr, ":")
		addr = strings.Join(addrs[:len(addrs)-1], ":")
	}

	delete(r.ConnMap, addr)
	conn.Close()
}

func (r *router) removeRouterConn(conn net.Conn) {
	r.ConnMapLock.Lock("removeRouterConn")
	defer r.ConnMapLock.Unlock()
	r.unsafeRemoveRouterConn(conn)
}

func removePort(addr string) (string, error) {
	splitPort := strings.Split(addr, ":")
	if len(splitPort) > 1 {
		lastOne := splitPort[len(splitPort)-1]
		if _, err := strconv.Atoi(lastOne); err == nil {
			addr = strings.Join(splitPort[:len(splitPort)-1], ":")
			return addr, nil
		}
	}
	return addr, ErrNotFoundPort
}
