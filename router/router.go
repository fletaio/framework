package router

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/framework/log"
	"git.fleta.io/fleta/mocknet/mocknet"
)

//RemoteAddr remote address type
type RemoteAddr string

const mockMode = true

//Router that converts external connections to logical connections.
type Router interface {
	AddListen(ChainCoord *common.Coordinate) error
	Dial(addrStr string, ChainCoord *common.Coordinate) error
	Accept(ChainCoord *common.Coordinate) (Receiver, error)
	Port() uint16
	ConnList() []string
	ID() string
	Localhost() string
}

type router struct {
	ListenersLock sync.Mutex
	Listeners     ListenerMap
	pConn         PConnMap
	ReceiverChan  ReceiverChanMap
	port          uint16
	TempMockID    string
	localhost     string
}

// NewRouter is the router creator.
func NewRouter(port uint16, TempMockID string) Router {
	r := &router{
		Listeners:    ListenerMap{},
		pConn:        PConnMap{},
		ReceiverChan: ReceiverChanMap{},
		port:         port,
		TempMockID:   TempMockID,
	}
	return r
}

func (r *router) ID() string {
	return r.TempMockID
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
	defer r.ListenersLock.Unlock()

	_, has := r.Listeners.Load(RemoteAddr(listenAddr))
	if !has {
		var l net.Listener
		if mockMode {
			l2, err := mocknet.Listen("tcp", r.TempMockID+listenAddr)
			if err != nil {
				return err
			}
			l = l2
		} else {
			l2, err := net.Listen("tcp", listenAddr)
			if err != nil {
				return err
			}
			l = l2
		}
		log.Debug("Listen ", listenAddr, " ", l.Addr().String())
		r.Listeners.Store(RemoteAddr(listenAddr), ChainCoord, l)
		go r.run(l)
	}
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
				log.Debug("router conn close ", r.ID(), " ", conn.RemoteAddr().String())
				conn.Close()
			} else {
				log.Debug("router run ", r.ID(), " ", conn.RemoteAddr().String())
				pc := newPhysicalConnection(addr, conn, r)
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
func (r *router) Accept(ChainCoord *common.Coordinate) (Receiver, error) {
	// listenAddr := ":" + strconv.Itoa(int(r.Port()))
	// l, has := r.Listeners.Load(RemoteAddr(listenAddr))
	// if !has {
	//  return nil, ErrListenFirst
	// }
	// lAddr := strings.Split(l.Addr().String(), ":")
	// portStr := lAddr[len(lAddr)-1]
	// port, err := strconv.Atoi(portStr)
	// if err != nil {
	//  return nil, err
	// }

	ch := r.ReceiverChan.load(int(r.Port()), *ChainCoord)

	receiver := <-ch
	return receiver, nil
}

//Dial requests the connection by entering the address when a logical connection is required.
//The chain coordinates support the connection between subchains.
func (r *router) Dial(addrStr string, ChainCoord *common.Coordinate) error {
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
		var conn net.Conn
		if mockMode {
			c, err := mocknet.Dial("tcp", addrStr, r.TempMockID+":"+strconv.Itoa(int(r.port)))
			if err != nil {
				return err
			}
			conn = c
		} else {
			c, err := net.Dial("tcp", addrStr)
			if err != nil {
				return err
			}
			conn = c
		}
		log.Debug("Dial1 ", r.ID(), " ", addrStr)

		if r.localhost == "" {
			r.SetLocalhost(conn.LocalAddr().String())
		}

		pConn = newPhysicalConnection(addr, conn, r)
		r.pConn.store(addr, pConn)
		go pConn.run()
	} else {
		log.Debug("Dial2 ", r.ID(), " ", addrStr)
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

func (r *router) acceptConn(conn Receiver, ChainCoord *common.Coordinate) error {
	// listenAddrs := strings.Split(conn.LocalAddr().String(), ":")
	// listenPort := listenAddrs[len(listenAddrs)-1]
	// port, err := strconv.Atoi(listenPort)
	// if err != nil {
	//  log.Debug("acceptConn start err ", conn.LocalAddr().String(), " ", ChainCoord, " ", err)
	//  return err
	// }

	ch := r.ReceiverChan.load(int(r.Port()), *ChainCoord)
	ch <- conn

	return nil
}

func (r *router) removePhysicalConnenction(pc *physicalConnection) error {
	r.pConn.delete(pc.addr)
	return pc.Close()
}
