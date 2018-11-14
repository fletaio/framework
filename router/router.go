package router

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/framework/log"

	network "git.fleta.io/fleta/mocknet"
)

//RemoteAddr remote address type
type RemoteAddr string

const mockMode = true

//Router that converts external connections to logical connections.
type Router interface {
	AddListen(ChainCoord *common.Coordinate) error
	Request(addrStr string, ChainCoord *common.Coordinate) error
	Accept(ChainCoord *common.Coordinate) (net.Conn, error)
	Port() uint16
	ConnList() []string
	Localhost() string
}

type router struct {
	ListenersLock sync.Mutex
	Listeners     ListenerMap
	pConn         PConnMap
	ReceiverChan  ReceiverChanMap
	port          uint16
	network       string
	localhost     string
}

// NewRouter is the router creator.
func NewRouter(Network string, port uint16) Router {
	r := &router{
		Listeners:    ListenerMap{},
		pConn:        PConnMap{},
		ReceiverChan: ReceiverChanMap{},
		port:         port,
		network:      Network,
	}
	return r
}

//Localhost returns the local host itself
func (r *router) Localhost() string {
	return r.localhost
}

//ConnList returns the associated node addresses.
func (r *router) ConnList() []string {
	list := []string{}
	r.pConn.Range(func(addr RemoteAddr, pc *physicalConnection) bool {
		list = append(list, string(addr)+","+strconv.Itoa(pc.lConn.len()))
		return true
	})
	return list
}

//Port returns the port to connect to node
func (r *router) Port() uint16 {
	return r.port
}

//AddListen registers a logical connection as a waiting-for-connect condition.
func (r *router) AddListen(ChainCoord *common.Coordinate) error {
	listenAddr := ":" + strconv.Itoa(int(r.port))
	r.ListenersLock.Lock()
	l, has := r.Listeners.Load(RemoteAddr(listenAddr))
	if !has {
		l2, err := network.Listen(r.network, listenAddr)
		if err != nil {
			return err
		}
		log.Debug("Listen ", listenAddr, " ", l2.Addr().String())
		r.Listeners.Store(RemoteAddr(listenAddr), ChainCoord, l2)
		go r.run(l2)

		l, _ = r.Listeners.Load(RemoteAddr(listenAddr))
		localhost := l.l.Addr().String()
		if r.localhost == "" {
			if !strings.HasPrefix(localhost, ":") {
				r.localhost = localhost
			}
		}
	}
	r.ListenersLock.Unlock()
	return nil
}

func (r *router) PauseListener(ChainCoord *common.Coordinate) {
	listenAddr := ":" + strconv.Itoa(int(r.port))
	r.Listeners.UpdateState(RemoteAddr(listenAddr), ChainCoord, pause)
}

func (r *router) PlayListener(ChainCoord *common.Coordinate) {
	listenAddr := ":" + strconv.Itoa(int(r.port))
	r.Listeners.UpdateState(RemoteAddr(listenAddr), ChainCoord, listening)
}

func (r *router) run(l net.Listener) {
	for {
		listenAddr := ":" + strconv.Itoa(int(r.port))
		r.ListenersLock.Lock()
		ls, has := r.Listeners.Load(RemoteAddr(listenAddr))
		r.ListenersLock.Unlock()
		if has && ls.State() == listening {
			conn, err := l.Accept()
			// log.Debug("Router Run Accept " + conn.LocalAddr().String() + " : " + conn.RemoteAddr().String())
			if err != nil {
				log.Error("router run err : ", err)
				continue
			}
			if r.localhost == "" {
				r.SetLocalhost(conn.LocalAddr().String())
			}

			addr := RemoteAddr(conn.RemoteAddr().String())
			_, has := r.pConn.load(addr)
			if has {
				log.Debug("router conn close ", r.localhost, " ", conn.RemoteAddr().String())
				conn.Close()
			} else {
				log.Debug("router run ", r.localhost, " ", conn.RemoteAddr().String())
				pc := newPhysicalConnection(addr, r.Localhost(), conn, r)
				r.pConn.store(addr, pc)
				go pc.run()
			}
		} else if has {
			time.Sleep(time.Second * 1)
		} else {
			break
		}
	}
}

//Accept returns a logical connection when an external connection request is received.
func (r *router) Accept(ChainCoord *common.Coordinate) (net.Conn, error) {
	ch := r.ReceiverChan.load(int(r.Port()), *ChainCoord)

	receiver := <-ch
	return receiver, nil
}

//Request requests the connection by entering the address when a logical connection is required.
//The chain coordinates support the connection between subchains.
func (r *router) Request(addrStr string, ChainCoord *common.Coordinate) error {
	if r.localhost != "" && strings.HasPrefix(addrStr, r.localhost) {
		return nil
	}

	splitPort := strings.Split(addrStr, ":")
	if len(splitPort) > 1 {
		lastOne := splitPort[len(splitPort)-1]
		if _, err := strconv.Atoi(lastOne); err == nil {
			addrStr = strings.Join(splitPort[:len(splitPort)-1], ":") + ":" + strconv.Itoa(int(r.Port()))
		}
	}

	addr := RemoteAddr(addrStr)
	pConn, has := r.pConn.load(addr)
	if !has {
		conn, err := network.Dial(r.network, addrStr)
		if err != nil {
			return err
		}
		log.Debug("Dial1 ", r.localhost, " ", addrStr)

		if r.localhost == "" {
			r.SetLocalhost(conn.LocalAddr().String())
		}

		pConn = newPhysicalConnection(addr, r.localhost, conn, r)
		r.pConn.store(addr, pConn)
		go pConn.run()
	} else {
		log.Debug("Dial2 ", r.localhost, " ", addrStr)
	}

	pConn.handshake(ChainCoord)

	return nil
}

func (r *router) SetLocalhost(l string) {
	ls := strings.Split(l, ":")
	if len(ls) > 1 {
		l = strings.Join(ls[0:len(ls)-1], ":")
	}
	r.localhost = l

}

func (r *router) acceptConn(conn net.Conn, ChainCoord *common.Coordinate) error {
	ch := r.ReceiverChan.load(int(r.Port()), *ChainCoord)
	ch <- conn

	return nil
}

func (r *router) removePhysicalConnenction(pc *physicalConnection) error {
	r.pConn.delete(pc.addr)
	return pc.Close()
}
