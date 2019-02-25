package router

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.fleta.io/fleta/framework/router/evilnode"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/network"

	"git.fleta.io/fleta/framework/log"
)

// Config is router config
type Config struct {
	Network        string
	Address        string
	Port           int
	EvilNodeConfig evilnode.Config
}

//Router that converts external connections to logical connections.
type Router interface {
	AddListen(ChainCoord *common.Coordinate) error
	Request(addrStr string, ChainCoord *common.Coordinate) error
	Accept(ChainCoord *common.Coordinate) (Conn, time.Duration, error)
	Localhost() string
	EvilNodeManager() *evilnode.Manager
	Conf() *Config
}

type router struct {
	Config          *Config
	localhost       string
	ListenersLock   sync.Mutex
	Listeners       ListenerMap
	PConn           PConnMap
	evilNodeManager *evilnode.Manager
	ReceiverChan    ReceiverChanMap
}

// NewRouter is creator of router
func NewRouter(Config *Config) (Router, error) {
	return &router{
		Config:          Config,
		Listeners:       ListenerMap{},
		PConn:           PConnMap{},
		evilNodeManager: evilnode.NewManager(&Config.EvilNodeConfig),
		ReceiverChan:    ReceiverChanMap{},
	}, nil
}

func (r *router) Conf() *Config {
	return r.Config
}

//AddListen registers a logical connection as a waiting-for-connect condition.
func (r *router) AddListen(ChainCoord *common.Coordinate) error {
	r.ListenersLock.Lock()
	defer r.ListenersLock.Unlock()

	listenAddr := ":" + strconv.Itoa(r.Config.Port)
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
	return nil
}

//Request requests the connection by entering the address when a logical connection is required.
//The chain coordinates support the connection between subchains.
func (r *router) Request(addr string, ChainCoord *common.Coordinate) error {
	if r.localhost != "" && strings.HasPrefix(addr, r.localhost) {
		return ErrCannotRequestToLocal
	}
	if r.evilNodeManager.IsBanNode(addr) {
		return ErrCanNotConnectToEvilNode
	}

	addr, _ = removePort(addr)
	addr = addr + ":" + strconv.Itoa(r.Config.Port)
	r.PConn.lock("Request")
	defer r.PConn.unlock()

	_, has := r.PConn.load(addr)
	if !has {
		conn, err := network.DialTimeout(r.Config.Network, addr, time.Second*2)
		if err != nil {
			return err
		}

		_, err = r.incommingConn(conn, ChainCoord)
		if err != nil {
			if err == ErrCanNotConnectToEvilNode {
				conn.Close()
			}
			return err
		}
	}

	// conn, err := network.Dial(r.Config.Network, addr)
	return nil
}

// Accept returns a logical connection when an external connection request is received.
func (r *router) Accept(ChainCoord *common.Coordinate) (Conn, time.Duration, error) {
	ch := r.ReceiverChan.load(r.Config.Port, *ChainCoord)

	receiver := <-ch
	var c Conn
	c = receiver
	return c, receiver.ping, nil
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
			_, err = r.incommingConn(conn, nil)
			if err != nil {
				if err == ErrCanNotConnectToEvilNode {
					conn.Close()
				} else {
					log.Error("incommingConn err", err)
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

func (r *router) incommingConn(conn net.Conn, ChainCoord *common.Coordinate) (*physicalConnection, error) {
	if r.localhost == "" {
		r.setLocalhost(conn.LocalAddr().String())
	}

	addr := conn.RemoteAddr().String()
	if raddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		addr = raddr.IP.String()
	}
	if r.evilNodeManager.IsBanNode(addr) {
		return nil, ErrCanNotConnectToEvilNode
	}

	oldPConn, has := r.PConn.load(addr)
	var pc *physicalConnection
	if has {
		oldPConn.Close()
		log.Debug("router duplicate conn close ", r.Localhost, " ", conn.RemoteAddr().String())
	}
	pc = newPhysicalConnection(addr, conn, r)
	r.PConn.store(addr, pc)

	errCh := make(chan error)
	var wg sync.WaitGroup
	wg.Add(1)
	go func(pc *physicalConnection) {
		wg.Done()

		err := pc.doHandshake()
		if err != nil {
			pc.Close()
		}
		errCh <- err
	}(pc)
	if ChainCoord != nil {
		pc.handshake(ChainCoord)
	}
	wg.Wait()
	deadTimer := time.NewTimer(5 * time.Second)
	select {
	case <-deadTimer.C:
		pc.Close()
		return nil, ErrPeerTimeout
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	}

	go pc.run()
	return pc, nil
}
func (r *router) localAddress() string {
	return r.Config.Address
}
func (r *router) port() int {
	return r.Config.Port
}
func (r *router) acceptConn(conn *logicalConnection, ChainCoord *common.Coordinate) error {
	ch := r.ReceiverChan.load(r.Config.Port, *ChainCoord)
	ch <- conn

	return nil
}

func (r *router) removePhysicalConnenction(pc *physicalConnection) error {
	r.PConn.lock("removePhysicalConnenction")
	defer r.PConn.unlock()

	addr := pc.RemoteAddr().String()
	if raddr, ok := pc.RemoteAddr().(*net.TCPAddr); ok {
		addr = raddr.IP.String()
	}
	r.PConn.delete(addr)
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
	}
	return addr, ErrNotFoundPort
}
