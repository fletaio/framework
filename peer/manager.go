package peer

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/framework/log"
	"git.fleta.io/fleta/framework/message"
	peerMessage "git.fleta.io/fleta/framework/peer/message"
	"git.fleta.io/fleta/framework/peer/storage"
	"git.fleta.io/fleta/framework/router"
)

//Config is structure storing settings information
type Config struct {
	Port uint16
}

//EventHandler is callback when peer connected or closed
type EventHandler interface {
	PeerConnected(p *Peer)
	PeerClosed(p *Peer)
}

//BaseEventHandler is empty EventHandler struct
type BaseEventHandler struct{}

//PeerConnected is empty BaseEventHandler functions
func (b *BaseEventHandler) PeerConnected(p *Peer) {}

//PeerClosed is empty BaseEventHandler functions
func (b *BaseEventHandler) PeerClosed(p *Peer) {}

//Manager TODO
type Manager struct {
	ChainCoord *common.Coordinate
	router     router.Router
	Handler    *message.Handler
	onReady    func(p *Peer)

	// nodes           NodeStore
	nodes           *NodeStore
	nodeRotateIndex int
	candidates      CandidateMap

	peerGroupLock sync.Mutex
	connections   ConnectMap

	peerStorage storage.IPeerStorage

	eventHandlerLock sync.Mutex
	eventHandler     []EventHandler
}

type candidateState int

const (
	csDelete       candidateState = 0
	csDialWait     candidateState = 1
	csDialWait2    candidateState = 2
	csDialWait3    candidateState = 3
	csPongWait     candidateState = 4
	csPeerListWait candidateState = 5
)

//NewManager is the peerManager creator.
//Apply messages necessary for peer management.
func NewManager(ChainCoord *common.Coordinate, mh *message.Handler, cfg Config, TempMockID string) (*Manager, error) {
	ns, err := NewNodeStore(TempMockID)
	if err != nil {
		return nil, err
	}
	pm := &Manager{
		ChainCoord:   ChainCoord,
		router:       router.NewRouter(cfg.Port, TempMockID),
		Handler:      mh,
		nodes:        ns,             //make(map[string]peerMessage.ConnectInfo),
		candidates:   CandidateMap{}, //make(map[string]candidateState),
		connections:  ConnectMap{},   //make(map[string]*Peer),
		eventHandler: []EventHandler{},
	}
	pm.peerStorage = storage.NewPeerStorage(pm.kickOutPeerStorage)

	//add ping message
	mh.ApplyMessage(peerMessage.PingMessageType, peerMessage.PingCreator, pm.pingHandler)
	//add pong message
	mh.ApplyMessage(peerMessage.PongMessageType, peerMessage.PongCreator, pm.pongHandler)
	//add requestPeerList message
	mh.ApplyMessage(peerMessage.PeerListMessageType, peerMessage.PeerListCreator, pm.peerListHandler)

	return pm, nil
}

//RegisterEventHandler is Registered event handler
func (pm *Manager) RegisterEventHandler(eh EventHandler) {
	pm.eventHandlerLock.Lock()
	pm.eventHandler = append(pm.eventHandler, eh)
	pm.eventHandlerLock.Unlock()
}

//StartManage is start peer management
func (pm *Manager) StartManage() {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer func() {
			log.Debug("StartManage go func() end")
		}()
		wg.Done()
		for {
			conn, err := pm.router.Accept(pm.ChainCoord)
			if err != nil {
				continue
			}
			log.Debug("Accept ", conn.LocalAddr().String(), " ", conn.RemoteAddr().String())
			go func(conn router.Receiver) {
				peer := NewPeer(conn, pm.Handler, pm.deletePeer)
				pm.addPeer(peer)
			}(conn)
		}
	}()
	wg.Wait()

	// peer list load from db
	pm.router.AddListen(pm.ChainCoord)

	go pm.manageCandidate()
	go pm.rotatePeer()
}

// AddNode is used to register additional peers from outside.
func (pm *Manager) AddNode(addr string) error {
	if pm.router.Localhost() != "" && strings.HasPrefix(addr, pm.router.Localhost()) {
		return nil
	}
	pm.candidates.store(addr, csDialWait)
	if err := pm.router.Dial(addr, pm.ChainCoord); err != nil {
		log.Error("StartManage connect seednode ", err)
		return err
	}
	return nil
}

//BroadCast is used to propagate messages to all nodes.
func (pm *Manager) BroadCast(m message.Message) {
	pm.connections.Range(func(addr string, p *Peer) bool {
		p.Send(m)
		return true
	})
}

//ConnectedPeerList is returns the addresses of the connected peers
func (pm *Manager) ConnectedPeerList() []string {
	list := make([]string, 0)
	pm.connections.Range(func(key string, value *Peer) bool {
		list = append(list, key)
		return true
	})
	return list
}

//NodeList is returns the addresses of the collected peers
func (pm *Manager) NodeList() []string {
	list := make([]string, 0)
	pm.nodes.Range(func(addr string, ci peerMessage.ConnectInfo) bool {
		list = append(list, addr+":"+strconv.Itoa(ci.ScoreBoard.Len()))
		return true
	})
	return list
}

//CandidateList is returns the address of the node waiting for the operation.
func (pm *Manager) CandidateList() []string {
	list := make([]string, 0)
	pm.candidates.rangeMap(func(addr string, cs candidateState) bool {
		list = append(list, addr+","+strconv.Itoa(int(cs)))
		return true
	})
	return list
}

//GroupList returns a list of peer groups.
func (pm *Manager) GroupList() []string {
	return pm.peerStorage.List()
}

func (pm *Manager) pingHandler(m message.Message) error {
	if ping, ok := m.(*peerMessage.Ping); ok {
		log.Debug("PingHandler ", pm.router.Port(), " ", ping.From, " ", ping.To)

		if p, has := pm.connections.Load(ping.From); has {
			peerMessage.SendPong(p, ping)
		}
	}
	return nil
}

func (pm *Manager) pongHandler(m message.Message) error {
	if pong, ok := m.(*peerMessage.Pong); ok {
		log.Debug("PongHandler start ", pm.router.ID(), " ", pong.From, " ", pong.To)
		pingTime := time.Duration(int64(uint32(time.Now().UnixNano()) - pong.Time))

		if p, has := pm.connections.Load(pong.To); has {
			p.SetPingTime(pingTime)
			addr := p.RemoteAddr().String()
			pm.nodes.LoadOrStore(addr, peerMessage.NewConnectInfo(addr, p.PingTime()))
			pm.candidates.store(addr, csPeerListWait)

			peerMessage.SendRequestPeerList(p, pong.From)
		}
		log.Debug("PongHandler end ", pm.router.ID(), " ", pong.From, " ", pong.To)
	}
	return nil
}

func (pm *Manager) peerListHandler(m message.Message) error {
	if peerList, ok := m.(*peerMessage.PeerList); ok {
		if peerList.Request == true {
			log.Debug("peerListHandler ", pm.router.ID(), " ", peerList.From)
			peerList.Request = false
			nodeMap := make(map[string]peerMessage.ConnectInfo)
			pm.nodes.Range(func(addr string, ci peerMessage.ConnectInfo) bool {
				nodeMap[addr] = ci
				return true
			})
			peerList.List = nodeMap

			if p, has := pm.connections.Load(peerList.From); has {
				peerList.From = p.LocalAddr().String()
				p.Send(peerList)
			}

		} else {
			log.Debug("peerListHandler ", pm.router.ID(), " ", peerList.From, " ", len(peerList.List)) //, " ", peerList.List)
			pm.peerGroupLock.Lock()
			pm.candidates.delete(peerList.From)

			for _, ci := range peerList.List {
				if ci.Address == pm.router.Localhost() {
					continue
				}

				if _, has := pm.candidates.load(ci.Address); has {
					continue
				}
				if _, has := pm.nodes.Load(ci.Address); has {
					continue
				}
				if _, has := pm.connections.Load(ci.Address); has {
					continue
				}

				pm.AddNode(ci.Address)
				time.Sleep(time.Millisecond * 50)
			}

			if p, connectionHas := pm.connections.Load(peerList.From); connectionHas {
				for _, ci := range peerList.List {
					if connectionHas {
						pm.updateScoreBoard(p, ci)
					}
				}
				pm.addReadyConn(p)
			}

			pm.peerGroupLock.Unlock()
		}

		log.Debug("peerListHandler end ", pm.router.ID(), " ", peerList.From)
	}
	return nil
}

func (pm *Manager) updateScoreBoard(p *Peer, ci peerMessage.ConnectInfo) {
	addr := p.RemoteAddr().String()

	node, has := pm.nodes.Load(addr)
	if !has {
		node = peerMessage.NewConnectInfo(addr, p.PingTime())
		pm.nodes.Store(addr, node)
	}
	node.ScoreBoard.Store(ci.Address, ci.PingTime, p.LocalAddr().String()+" "+p.RemoteAddr().String()+" ")
}

func (pm *Manager) manageCandidate() {
	for {
		time.Sleep(time.Second * 30)
		// pm.candidates.lock()
		addList := []string{}
		modifylist := map[string]candidateState{}
		pm.candidates.rangeMap(func(addr string, cs candidateState) bool {
			if strings.HasPrefix(addr, pm.router.Localhost()) {
				modifylist[addr] = csDelete
				// pm.candidates.delete(addr)
				return true
			}
			switch cs {
			case csDialWait:
				modifylist[addr] = csDialWait2
				// pm.candidates.store(addr, dialWait2)
				err := pm.router.Dial(addr, pm.ChainCoord)
				if err != nil {
					log.Error("PeerListHandler err ", err)
				}
			case csDialWait2:
				modifylist[addr] = csDialWait3
				// pm.candidates.store(addr, dialWait3)
				err := pm.router.Dial(addr, pm.ChainCoord)
				if err != nil {
					log.Error("PeerListHandler err ", err)
				}
			case csDialWait3:
				modifylist[addr] = csDelete
				// pm.candidates.delete(addr)
				pm.nodes.Delete(addr)
			case csPongWait:
				if p, has := pm.connections.Load(addr); has {
					peerMessage.SendPing(p, uint32(time.Now().UnixNano()), p.LocalAddr().String(), p.RemoteAddr().String())
				} else {
					addList = append(addList, addr)
				}
			case csPeerListWait:
				if p, has := pm.connections.Load(addr); has {
					peerMessage.SendRequestPeerList(p, pm.router.Localhost()+":"+strconv.Itoa(int(pm.router.Port())))
				} else {
					addList = append(addList, addr)
				}
			}
			time.Sleep(time.Millisecond * 50)
			return true
		})
		// pm.candidates.unlock()
		for k, s := range modifylist {
			switch s {
			case csDelete:
				pm.candidates.delete(k)
			default:
				pm.candidates.store(k, s)
			}
		}
		for _, addr := range addList {
			pm.AddNode(addr)
		}

	}
}

func (pm *Manager) rotatePeer() {
	for {
		if pm.peerStorage.NotEnoughPeer() {
			time.Sleep(time.Second * 2)
		} else {
			time.Sleep(time.Minute * 20)
		}

		pm.appendPeerStorage()
	}
}

func (pm *Manager) appendPeerStorage() {
	for i := pm.nodeRotateIndex; i < pm.nodes.Len(); i++ {
		p := pm.nodes.Get(i)
		pm.nodeRotateIndex = i + 1
		if pm.peerStorage.Have(p.Address) {
			continue
		}
		log.Debug("rotate peer ", pm.router.ID(), " to ", p.Address)
		if connectedPeer, has := pm.connections.Load(p.Address); has {
			pm.addReadyConn(connectedPeer)
		} else {
			err := pm.router.Dial(p.Address, pm.ChainCoord)
			if err != nil {
				log.Error("PeerListHandler err ", err)
			}
		}

		break
	}
	if pm.nodeRotateIndex >= pm.nodes.Len()-1 {
		pm.nodeRotateIndex = 0
	}
}

func (pm *Manager) kickOutPeerStorage(ip storage.IPeer) {
	if p, ok := ip.(*Peer); ok {
		if pm.connections.Len() > storage.MaxPeerStorageLen()*2 {
			closePeer := p
			pm.connections.Range(func(addr string, p *Peer) bool {
				if closePeer.connectedTime > p.connectedTime {
					if !pm.peerStorage.Have(addr) {
						closePeer = p
					}
				}
				return true
			})
			closePeer.Close()
		}
	}
}

func (pm *Manager) deletePeer(addr string) {
	pm.eventHandlerLock.Lock()
	if p, has := pm.connections.Load(addr); has {
		for _, eh := range pm.eventHandler {
			eh.PeerClosed(p)
		}
	}
	pm.eventHandlerLock.Unlock()
	pm.connections.Delete(addr)
	// if pm.peerStorage.NotEnoughPeer() {
	// 	pm.appendPeerStorage()
	// }
}

func (pm *Manager) addPeer(p *Peer) {
	pm.peerGroupLock.Lock()
	defer pm.peerGroupLock.Unlock()

	if _, has := pm.connections.Load(p.RemoteAddr().String()); has {
		log.Debug("close peer2 ", p.LocalAddr().String(), " ", p.RemoteAddr().String())
		p.Close()
	} else {
		addr := p.RemoteAddr().String()
		pm.connections.Store(addr, p)
		pm.candidates.store(addr, csPongWait)

		pm.eventHandlerLock.Lock()
		for _, eh := range pm.eventHandler {
			eh.PeerConnected(p)
		}
		pm.eventHandlerLock.Unlock()

		go func(p *Peer) {
			for p.PingTime() == -1 && p.closed == false {
				peerMessage.SendPing(p, uint32(time.Now().UnixNano()), p.LocalAddr().String(), p.RemoteAddr().String())
				time.Sleep(time.Second * 3)
			}
		}(p)
	}
}

func (pm *Manager) addReadyConn(p *Peer) {
	pm.peerStorage.Add(p, func(addr string) (time.Duration, bool) {
		if node, has := pm.nodes.Load(addr); has {
			return node.ScoreBoard.Load(addr)
		}
		return 0, false
	})

}
