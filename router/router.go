package router

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.fleta.io/fleta/common"
	network "git.fleta.io/fleta/mocknet"
	"github.com/dgraph-io/badger"

	"git.fleta.io/fleta/framework/log"
)

// Config is router config
type Config struct {
	Network      string
	Port         int
	StorePath    string
	BanEvilScore int
}

//Router that converts external connections to logical connections.
type Router interface {
	AddListen(ChainCoord *common.Coordinate) error
	Request(addrStr string, ChainCoord *common.Coordinate) error
	Accept(ChainCoord *common.Coordinate) (net.Conn, time.Duration, error)
	Localhost() string
	GetEvilScore(addr string) (addEvilScore uint16, err error)
	UpdateEvilScore(addr string, addEvilScore uint16) error
}

type router struct {
	Config        *Config
	localhost     string
	ListenersLock sync.Mutex
	Listeners     ListenerMap
	PConn         PConnMap
	PList         *PConnList
	ReceiverChan  ReceiverChanMap
}

// NewRouter is creator of router
func NewRouter(Config *Config) (Router, error) {
	pl, err := NewPConnList(Config.StorePath)
	if err != nil {
		return nil, err
	}
	return &router{
		Config:       Config,
		Listeners:    ListenerMap{},
		PConn:        PConnMap{},
		PList:        pl,
		ReceiverChan: ReceiverChanMap{},
	}, nil
}

//AddListen registers a logical connection as a waiting-for-connect condition.
func (r *router) AddListen(ChainCoord *common.Coordinate) error {
	listenAddr := ":" + strconv.Itoa(r.Config.Port)
	r.ListenersLock.Lock()
	_, has := r.Listeners.Load(listenAddr)
	var listen net.Listener
	if !has {
		l, err := network.Listen(r.Config.Network, listenAddr)
		if err != nil {
			return err
		}
		listen = l
		localhost := l.Addr().String()
		if r.localhost == "" {
			if !strings.HasPrefix(localhost, ":") && !strings.HasPrefix(localhost, "[") {
				r.localhost = localhost
			}
		}

		log.Debug("Listen ", listenAddr, " ", l.Addr().String())

		go r.listening(l)
	}
	r.Listeners.Store(listenAddr, ChainCoord, listen)
	r.ListenersLock.Unlock()
	return nil
}

//Request requests the connection by entering the address when a logical connection is required.
//The chain coordinates support the connection between subchains.
func (r *router) Request(addr string, ChainCoord *common.Coordinate) error {
	if r.localhost != "" && strings.HasPrefix(addr, r.localhost) {
		return ErrCannotRequestToLocal
	}

	es, err := r.GetEvilScore(addr)
	if err != nil {
		return err
	}
	if es > uint16(r.Config.BanEvilScore) {
		return ErrCanNotConnectToEvilNode
	}

	addr, _ = removePort(addr)
	addr = addr + ":" + strconv.Itoa(r.Config.Port)
	r.PConn.lock("Request")
	defer r.PConn.unlock()

	pConn, has := r.PConn.load(addr)
	if !has {
		conn, err := network.Dial(r.Config.Network, addr)
		if err != nil {
			return err
		}
		pConn, err = r.incommingConn(conn)
		if err != nil {
			if err == ErrCanNotConnectToEvilNode {
				conn.Close()
			}
			return err
		}
	}
	pConn.handshake(ChainCoord)

	// conn, err := mocknet.Dial(r.Config.Network, addr)
	return nil
}

// Accept returns a logical connection when an external connection request is received.
func (r *router) Accept(ChainCoord *common.Coordinate) (net.Conn, time.Duration, error) {
	ch := r.ReceiverChan.load(r.Config.Port, *ChainCoord)

	receiver := <-ch
	var c net.Conn
	c = receiver
	log.Info("Accept ", c.LocalAddr().String(), c.RemoteAddr().String())
	return c, receiver.ping, nil
}

func (r *router) Localhost() string {
	return r.localhost
}

func (r *router) SetLocalhost(l string) {
	addr, _ := removePort(l)
	r.localhost = addr
}

func (r *router) listening(l net.Listener) {
	for {
		listenAddr := ":" + strconv.Itoa(r.Config.Port)
		r.ListenersLock.Lock()
		ls, has := r.Listeners.Load(listenAddr)
		r.ListenersLock.Unlock()
		if has && ls.State() == listening {
			conn, err := l.Accept()
			if err != nil {
				log.Error("router run err : ", err)
				continue
			}
			r.PConn.lock("listening")
			_, err = r.incommingConn(conn)
			if err != nil {
				if err == ErrCanNotConnectToEvilNode {
					conn.Close()
				}
			}
			r.PConn.unlock()
		} else if has {
			time.Sleep(time.Second * 1)
		} else {
			break
		}
	}
}

// GetEvilScore is get evel score
func (r *router) GetEvilScore(addr string) (EvilScore uint16, err error) {
	pi, err := r.PList.Get(addr)
	if err != nil {
		if err == badger.ErrKeyNotFound {
			pi = PhysicalConnectionInfo{
				Addr:      addr,
				EvilScore: 0,
			}
			r.PList.Store(pi)
		} else {
			return 0, err
		}
	}

	return pi.EvilScore, nil
}

// UpdateEvilScore is add evel score
func (r *router) UpdateEvilScore(addr string, addEvilScore uint16) error {
	pi, err := r.PList.Get(addr)
	if err != nil {
		return err
	}

	pi.EvilScore += addEvilScore
	return r.PList.Store(pi)
}

func (r *router) incommingConn(conn net.Conn) (*physicalConnection, error) {
	if r.localhost == "" {
		r.SetLocalhost(conn.LocalAddr().String())
	}

	addr := conn.RemoteAddr().String()
	es, err := r.GetEvilScore(addr)
	if err != nil {
		return nil, err
	}
	if es > uint16(r.Config.BanEvilScore) {
		return nil, ErrCanNotConnectToEvilNode
	}

	_, has := r.PConn.load(addr)
	var pc *physicalConnection
	if has {
		log.Debug("router duplicate conn close ", r.Localhost, " ", conn.RemoteAddr().String())
		conn.Close()
	} else {
		log.Debug("router run ", r.Localhost, " ", conn.RemoteAddr().String())
		pc = newPhysicalConnection(addr, conn, r)
		r.PConn.store(addr, pc)
		go pc.run()
	}
	return pc, nil
}

func (r *router) acceptConn(conn *logicalConnection, ChainCoord *common.Coordinate) error {
	ch := r.ReceiverChan.load(r.Config.Port, *ChainCoord)
	ch <- conn

	return nil
}

func (r *router) removePhysicalConnenction(pc *physicalConnection) error {
	r.PConn.lock("removePhysicalConnenction")
	defer r.PConn.unlock()

	r.PConn.delete(pc.RemoteAddr().String())
	return pc.Close()
}

func removePort(addr string) (string, error) {
	splitPort := strings.Split(addr, ":")
	if len(splitPort) > 1 {
		lastOne := splitPort[len(splitPort)-1]
		if _, err := strconv.Atoi(lastOne); err == nil {
			addr = strings.Join(splitPort[:len(splitPort)-1], ":")
			return addr, nil
		}
		return addr, ErrNotFoundPort
	}
	return addr, ErrNotFoundPort
}
