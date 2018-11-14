package peer

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/framework/log"
	"git.fleta.io/fleta/framework/message"
	"git.fleta.io/fleta/framework/peer/peermessage"
	"git.fleta.io/fleta/framework/peer/storage"
	"git.fleta.io/fleta/framework/router"
)

//Config is structure storing settings information
type Config struct {
	Port      uint16
	Network   string
	StorePath string
}

//Manager manages peer-connected networks.
type Manager interface {
	RegisterEventHandler(eh EventHandler)
	StartManage()
	EnforceConnect()
	AddNode(addr string) error
	BroadCast(m message.Message)
	NodeList() []string
}

type manager struct {
	ChainCoord *common.Coordinate
	router     router.Router
	Handler    *message.Handler
	onReady    func(p *peer)

	// nodes           NodeStore
	nodes           *nodeStore
	nodeRotateIndex int
	candidates      candidateMap

	peerGroupLock sync.Mutex
	connections   connectMap

	peerStorage storage.IPeerStorage

	eventHandlerLock sync.Mutex
	eventHandler     []EventHandler
}

type candidateState int

const (
	csDelete       candidateState = 0
	csRequestWait  candidateState = 1
	csRequestWait2 candidateState = 2
	csPongWait     candidateState = 4
	csPeerListWait candidateState = 5
)

//NewManager is the peerManager creator.
//Apply messages necessary for peer management.
func NewManager(ChainCoord *common.Coordinate, mh *message.Handler, cfg Config) (Manager, error) {
	ns, err := newNodeStore(cfg.StorePath)
	if err != nil {
		return nil, err
	}
	pm := &manager{
		ChainCoord:   ChainCoord,
		router:       router.NewRouter(cfg.Network, cfg.Port),
		Handler:      mh,
		nodes:        ns,             //make(map[string]peermessage.ConnectInfo),
		candidates:   candidateMap{}, //make(map[string]candidateState),
		connections:  connectMap{},   //make(map[string]*Peer),
		eventHandler: []EventHandler{},
	}
	pm.peerStorage = storage.NewPeerStorage(pm.kickOutPeerStorage)

	//add ping message
	mh.ApplyMessage(peermessage.PingMessageType, peermessage.PingCreator, pm.pingHandler)
	//add pong message
	mh.ApplyMessage(peermessage.PongMessageType, peermessage.PongCreator, pm.pongHandler)
	//add requestPeerList message
	mh.ApplyMessage(peermessage.PeerListMessageType, peermessage.PeerListCreator, pm.peerListHandler)

	return pm, nil
}

//RegisterEventHandler is Registered event handler
func (pm *manager) RegisterEventHandler(eh EventHandler) {
	pm.eventHandlerLock.Lock()
	pm.eventHandler = append(pm.eventHandler, eh)
	pm.eventHandlerLock.Unlock()
}

//StartManage is start peer management
func (pm *manager) StartManage() {
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
			go func(conn net.Conn) {
				peer := newPeer(conn, pm.Handler, pm.deletePeer)
				pm.addPeer(peer)
			}(conn)
		}
	}()
	wg.Wait()

	pm.router.AddListen(pm.ChainCoord)

	go pm.manageCandidate()
	go pm.rotatePeer()
}

// EnforceConnect handles all of the Request standby nodes in the cardidate.
func (pm *manager) EnforceConnect() {
	dialList := []string{}
	pm.candidates.rangeMap(func(addr string, cs candidateState) bool {
		if cs == csRequestWait || cs == csRequestWait2 {
			dialList = append(dialList, addr)
		}
		return true
	})
	for _, addr := range dialList {
		err := pm.router.Request(addr, pm.ChainCoord)
		if err != nil {
			log.Error("EnforceConnect error ", err)
		}
		time.Sleep(time.Millisecond * 50)
	}
}

// AddNode is used to register additional peers from outside.
func (pm *manager) AddNode(addr string) error {
	if pm.router.Localhost() != "" && strings.HasPrefix(addr, pm.router.Localhost()) {
		return nil
	}
	if ci, has := pm.nodes.Load(addr); !has || ci.EvilScore < 100 {
		pm.candidates.store(addr, csRequestWait)
		pm.doManageCandidate(addr, csRequestWait)
	}
	// if err := pm.router.Request(addr, pm.ChainCoord); err != nil {
	// 	log.Error("StartManage connect seednode ", err)
	// 	return err
	// }
	return nil
}

//BroadCast is used to propagate messages to all nodes.
func (pm *manager) BroadCast(m message.Message) {
	// for p := range pm.connections.Range() {
	// 	if p != nil {
	// 		p.Send(m)
	// 	}
	// }
	pm.connections.Range(func(addr string, p Peer) bool {
		p.Send(m)
		return true
	})
}

//ConnectedPeerList is returns the addresses of the connected peers
func (pm *manager) ConnectedPeerList() []string {
	list := make([]string, 0)
	// for p := range pm.connections.Range() {
	// 	list = append(list, p.ID())
	// }
	pm.connections.Range(func(key string, value Peer) bool {
		list = append(list, key)
		return true
	})
	return list
}

//NodeList is returns the addresses of the collected peers
func (pm *manager) NodeList() []string {
	list := make([]string, 0)
	pm.nodes.Range(func(addr string, ci peermessage.ConnectInfo) bool {
		list = append(list, addr+":"+strconv.Itoa(ci.PingScoreBoard.Len()))
		return true
	})
	return list
}

//CandidateList is returns the address of the node waiting for the operation.
func (pm *manager) CandidateList() []string {
	list := make([]string, 0)
	pm.candidates.rangeMap(func(addr string, cs candidateState) bool {
		list = append(list, addr+","+strconv.Itoa(int(cs)))
		return true
	})
	return list
}

//GroupList returns a list of peer groups.
func (pm *manager) GroupList() []string {
	return pm.peerStorage.List()
}

func (pm *manager) pingHandler(m message.Message) error {
	if ping, ok := m.(*peermessage.Ping); ok {
		log.Debug("PingHandler ", pm.router.Port(), " ", ping.From, " ", ping.To)

		if p, has := pm.connections.Load(ping.From); has {
			peermessage.SendPong(p, ping)
		}
	}
	return nil
}

func (pm *manager) pongHandler(m message.Message) error {
	if pong, ok := m.(*peermessage.Pong); ok {
		pingTime := time.Duration(int64(uint32(time.Now().UnixNano()) - pong.Time))

		if p, has := pm.connections.Load(pong.To); has {
			p.SetPingTime(pingTime)
			addr := p.RemoteAddr().String()
			pm.nodes.LoadOrStore(addr, peermessage.NewConnectInfo(addr, p.PingTime()))
			pm.candidates.store(addr, csPeerListWait)

			peermessage.SendRequestPeerList(p, pong.From)
		}
	}
	return nil
}

func (pm *manager) peerListHandler(m message.Message) error {
	if peerList, ok := m.(*peermessage.PeerList); ok {
		if peerList.Request == true {
			peerList.Request = false
			nodeMap := make(map[string]peermessage.ConnectInfo)
			pm.nodes.Range(func(addr string, ci peermessage.ConnectInfo) bool {
				nodeMap[addr] = ci
				return true
			})
			peerList.List = nodeMap

			if p, has := pm.connections.Load(peerList.From); has {
				peerList.From = p.LocalAddr().String()
				p.Send(peerList)
			}

		} else {
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

	}
	return nil
}

func (pm *manager) updateScoreBoard(p Peer, ci peermessage.ConnectInfo) {
	addr := p.RemoteAddr().String()

	node := pm.nodes.LoadOrStore(addr, peermessage.NewConnectInfo(addr, p.PingTime()))
	node.PingScoreBoard.Store(ci.Address, ci.PingTime, p.LocalAddr().String()+" "+p.RemoteAddr().String()+" ")
}

func (pm *manager) doManageCandidate(addr string, cs candidateState) candidateState {
	if strings.HasPrefix(addr, pm.router.Localhost()) {
		return csDelete
	}
	switch cs {
	case csRequestWait:
		err := pm.router.Request(addr, pm.ChainCoord)
		if err != nil {
			log.Error("PeerListHandler err ", err)
		}
		return csRequestWait2
	case csRequestWait2:
		pm.nodes.Update(addr, func(ci peermessage.ConnectInfo) peermessage.ConnectInfo {
			ci.EvilScore += 40
			return ci
		})
		err := pm.router.Request(addr, pm.ChainCoord)
		if err != nil {
			log.Error("PeerListHandler err ", err)
		}
	case csPongWait:
		if p, has := pm.connections.Load(addr); has {
			peermessage.SendPing(p, uint32(time.Now().UnixNano()), p.LocalAddr().String(), p.RemoteAddr().String())
		} else {
			return csRequestWait
		}
	case csPeerListWait:
		if p, has := pm.connections.Load(addr); has {
			peermessage.SendRequestPeerList(p, pm.router.Localhost()+":"+strconv.Itoa(int(pm.router.Port())))
		} else {
			return csRequestWait
		}
	}
	return cs
}

func (pm *manager) manageCandidate() {
	for {
		time.Sleep(time.Second * 30)
		modifylist := map[string]candidateState{}
		pm.candidates.rangeMap(func(addr string, cs candidateState) bool {
			changedCs := pm.doManageCandidate(addr, cs)
			if changedCs != cs {
				modifylist[addr] = changedCs
			}
			time.Sleep(time.Millisecond * 50)
			return true
		})
		for k, s := range modifylist {
			switch s {
			case csDelete:
				pm.candidates.delete(k)
			default:
				pm.candidates.store(k, s)
			}
		}
	}
}

func (pm *manager) rotatePeer() {
	for {
		if pm.peerStorage.NotEnoughPeer() {
			time.Sleep(time.Second * 2)
		} else {
			time.Sleep(time.Minute * 20)
		}

		pm.appendPeerStorage()
	}
}

func (pm *manager) appendPeerStorage() {
	for i := pm.nodeRotateIndex; i < pm.nodes.Len(); i++ {
		p := pm.nodes.Get(i)
		pm.nodeRotateIndex = i + 1
		if pm.peerStorage.Have(p.Address) {
			continue
		}
		if connectedPeer, has := pm.connections.Load(p.Address); has {
			pm.addReadyConn(connectedPeer)
		} else {
			err := pm.router.Request(p.Address, pm.ChainCoord)
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

func (pm *manager) kickOutPeerStorage(ip storage.IPeer) {
	if p, ok := ip.(Peer); ok {
		if pm.connections.Len() > storage.MaxPeerStorageLen()*2 {
			closePeer := p
			// for p := range pm.connections.Range() {
			// 	if closePeer.connectedTime > p.connectedTime {
			// 		if !pm.peerStorage.Have(p.ID()) {
			// 			closePeer = p
			// 		}
			// 	}
			// }
			pm.connections.Range(func(addr string, p Peer) bool {
				if closePeer.ConnectedTime() > p.ConnectedTime() {
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

func (pm *manager) deletePeer(addr string) {
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

func (pm *manager) addPeer(p Peer) {
	pm.peerGroupLock.Lock()
	defer pm.peerGroupLock.Unlock()

	if _, has := pm.connections.Load(p.RemoteAddr().String()); has {
		log.Debug("close peer2 ", p.LocalAddr(), " ", p.RemoteAddr())
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

		go func(p Peer) {
			for p.PingTime() == -1 && p.IsClose() == false {
				peermessage.SendPing(p, uint32(time.Now().UnixNano()), p.LocalAddr().String(), p.RemoteAddr().String())
				time.Sleep(time.Second * 3)
			}
		}(p)
	}
}

func (pm *manager) addReadyConn(p Peer) {
	pm.peerStorage.Add(p, func(addr string) (time.Duration, bool) {
		if node, has := pm.nodes.Load(addr); has {
			return node.PingScoreBoard.Load(addr)
		}
		return 0, false
	})

}
