package router

import (
	"net"
	"strconv"
	"strings"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/framework/log"
)

//RemoteAddr remote address type
type RemoteAddr string

//Router that converts external connections to logical connections.
type Router interface {
	AddListen() error
	Dial(addrStr string, ChainCoord *common.Coordinate) error
	Accept(ChainCoord *common.Coordinate) (Receiver, error)
	Port() uint16
	ConnList() []string
	ID() string
}

type router struct {
	Listeners    ListenerMap
	pConn        PConnMap
	ReceiverChan ReceiverChanMap
	port         uint16
	TempMockID   string
	localhost    string
}

var i = 0

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
func (r *router) AddListen() error {
	listenAddr := ":" + strconv.Itoa(int(r.port))
	_, has := r.Listeners.Load(RemoteAddr(listenAddr))
	if !has {
		// l, err := mocknet.Listen("tcp", r.TempMockID+listenAddr)
		l, err := net.Listen("tcp", listenAddr)
		log.Debug("Listen ", listenAddr, " ", l.Addr().String())
		if err != nil {
			return err
		}
		r.Listeners.Store(RemoteAddr(listenAddr), l)
		go r.run(l)
	}
	return nil
}

func (r *router) run(l net.Listener) {
	for {
		conn, err := l.Accept()
		// log.Debug("Router Run Accept " + conn.LocalAddr().String() + " : " + conn.RemoteAddr().String())
		if err != nil {
			log.Error("router run err : ", err)
			continue
		}
		if r.localhost == "" {
			l := conn.LocalAddr().String()
			ls := strings.Split(l, ":")
			if len(ls) == 2 {
				l = ls[0]
			}
			r.localhost = l
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
	if len(splitPort) == 2 {
		addrStr = splitPort[0] + ":" + strconv.Itoa(int(r.Port()))
	}

	addr := RemoteAddr(addrStr)
	pConn, has := r.pConn.load(addr)
	if !has {
		// conn, err := mocknet.Dial("tcp", addrStr, r.TempMockID+":"+strconv.Itoa(int(r.port)))
		conn, err := net.Dial("tcp", addrStr)
		if r.localhost == "" {
			l := conn.LocalAddr().String()
			ls := strings.Split(l, ":")
			if len(ls) == 2 {
				l = ls[0]
			}
			r.localhost = l
		}

		log.Debug("Dial1 ", r.ID(), " ", addrStr)
		if err != nil {
			return err
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
