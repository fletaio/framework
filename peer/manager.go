package peer

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fletaio/framework/chain/mesh"

	"github.com/fletaio/common"
	"github.com/fletaio/framework/log"
	"github.com/fletaio/framework/message"
	"github.com/fletaio/framework/peer/peermessage"
	"github.com/fletaio/framework/peer/storage"
	"github.com/fletaio/framework/router"
)

//Config is structure storing settings information
type Config struct {
	StorePath string
}

// peer errors
var (
	ErrNotFoundPeer = errors.New("not found peer")
)

//Manager manages peer-connected networks.
type Manager interface {
	RegisterEventHandler(eh mesh.EventHandler)
	StartManage()
	EnforceConnect()
	AddNode(addr string) error
	BroadCast(m message.Message)
	BroadCastLimit(m message.Message, Limint int)
	NodeList() []string
	ConnectedList() []string
	TargetCast(addr string, m message.Message) error
	ExceptCast(addr string, m message.Message)
	ExceptCastLimit(addr string, m message.Message, Limit int)
}

type manager struct {
	Config         *Config
	ChainCoord     *common.Coordinate
	router         router.Router
	MessageManager *message.Manager
	onReady        func(p *peer)

	nodes           *nodeStore
	nodeRotateIndex int
	candidates      candidateMap

	peerGroupLock sync.Mutex
	connections   connectMap

	peerStorage storage.PeerStorage

	eventHandlerLock sync.RWMutex
	eventHandler     []mesh.EventHandler
	BanPeerInfos     *ByTime

	TestMsg string
}

type candidateState int

const (
	csRequestWait  candidateState = 1
	csPeerListWait candidateState = 2
)

//NewManager is the peerManager creator.
//Apply messages necessary for peer management.
func NewManager(ChainCoord *common.Coordinate, r router.Router, Config *Config) (*manager, error) {
	ns, err := newNodeStore(Config.StorePath)
	if err != nil {
		return nil, err
	}
	pm := &manager{
		Config:         Config,
		ChainCoord:     ChainCoord,
		router:         r,
		MessageManager: message.NewManager(),
		nodes:          ns,
		candidates:     candidateMap{},
		connections:    connectMap{},
		eventHandler:   []mesh.EventHandler{},
		BanPeerInfos:   NewByTime(),
	}
	pm.peerStorage = storage.NewPeerStorage()

	//add requestPeerList message
	pm.MessageManager.SetCreator(peermessage.PeerListMessageType, peermessage.PeerListCreator)

	pm.RegisterEventHandler(pm)

	go func() {
		for {
			time.Sleep(10 * time.Second)
			pm.connections.Range(func(addr string, p Peer) bool {
				p.SendHeartBit()
				return true
			})
		}
	}()

	// mc := make(chan simulations.Msg)
	// go func() {
	// 	for {
	// 		simulations.AddVisualizationData(pm.router.Localhost(), "peer", pm.ConnectedList)
	// 		simulations.AddVisualizationData(pm.router.Localhost(), "ConnList", pm.router.ConnList)
	// 		time.Sleep(time.Second)
	// 	}
	// }()
	// go simulations.VisualizationStart(mc, 58080)

	return pm, nil
}

func (pm *manager) errLog(msg ...interface{}) {
	var file string
	var line int
	{
		pc := make([]uintptr, 10) // at least 1 entry needed
		runtime.Callers(2, pc)
		f := runtime.FuncForPC(pc[0])
		file, line = f.FileLine(pc[0])

		path := strings.Split(file, "/")
		file = strings.Join(path[len(path)-3:], "/")
	}
	log.Error(append([]interface{}{file, " ", line, " ", pm.router.Conf().Network, " "}, msg...)...)
}

//RegisterEventHandler is Registered event handler
func (pm *manager) RegisterEventHandler(eh mesh.EventHandler) {
	pm.eventHandlerLock.Lock()
	pm.eventHandler = append(pm.eventHandler, eh)
	pm.eventHandlerLock.Unlock()
}

//StartManage is start peer management
func (pm *manager) StartManage() {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		wg.Done()
		for {
			conn, pingTime, err := pm.router.Accept()
			if err != nil {
				if conn != nil {
					conn.Close()
				}
				// pm.errLog(err, conn.ID())
				continue
			}

			// ban check
			addr := conn.ID()
			if pm.BanPeerInfos.IsBan(addr) {
				if hp, has := pm.connections.Load(addr); has {
					hp.Close()
				}
				conn.Close()
				// pm.errLog("BanPeerInfos.IsBan(addr) ", addr)
				continue
			}

			go func(conn router.Conn) {
				peer := newPeer(conn, pingTime, pm.deletePeer, pm.onRecvEventHandler)
				defer peer.Close()

				err = pm.addPeer(peer)
				if err != nil {
					return
				}
				pm.eventHandlerLock.RLock()
				for _, eh := range pm.eventHandler {
					eh.OnConnected(peer)
				}
				pm.eventHandlerLock.RUnlock()
				peer.Start()
			}(conn)
		}
	}()
	wg.Wait()

	pm.router.Listen()

	go pm.manageCandidate()
	go pm.rotatePeer()
}

func (pm *manager) onRecvEventHandler(p *peer, t message.Type) error {
	pm.eventHandlerLock.RLock()
	defer pm.eventHandlerLock.RUnlock()
	for _, eh := range pm.eventHandler {
		err := eh.OnRecv(p, p, t)
		if err != nil {
			if err == message.ErrUnknownMessage {
				continue
			}
			// pm.errLog("onRecvEventHandler ", err, " local ", p.LocalAddr().String(), "remote", p.ID())
			return err
		}
		break
	}

	return nil
}

// EnforceConnect handles all of the Request standby nodes in the cardidate.
func (pm *manager) EnforceConnect() {
	dialList := []string{}
	pm.candidates.rangeMap(func(addr string, cs candidateState) bool {
		if cs == csRequestWait {
			dialList = append(dialList, addr)
		}
		return true
	})
	for _, addr := range dialList {
		err := pm.router.Request(addr)
		if err != nil {
			// pm.errLog("EnforceConnect error ", err)
		}
		time.Sleep(time.Millisecond * 50)
	}
}

// AddNode is used to register additional peers from outside.
func (pm *manager) AddNode(addr string) error {
	if pm.router.Localhost() != "" && strings.HasPrefix(addr, pm.router.Localhost()) {
		return nil
	}

	if !pm.router.EvilNodeManager().IsBanNode(addr) {
		pm.candidates.store(addr, csRequestWait)
		pm.doManageCandidate(addr, csRequestWait)
	} else {
		// pm.errLog("AddNode router.ErrCanNotConnectToEvilNode", addr)
		return router.ErrCanNotConnectToEvilNode
	}
	return nil
}

//BroadCast is used to propagate messages to all nodes.
func (pm *manager) BroadCast(m message.Message) {
	pm.connections.Range(func(addr string, p Peer) bool {
		p.Send(m)
		return true
	})
}

//BroadCastLimit is used to propagate messages to limited number of nodes.
func (pm *manager) BroadCastLimit(m message.Message, Limit int) {
	Count := 0
	pm.connections.Range(func(addr string, p Peer) bool {
		p.Send(m)
		Count++
		return Count < Limit
	})
}

//BroadCast is used to propagate messages to all nodes.
func (pm *manager) ExceptCast(exceptAddr string, m message.Message) {
	pm.connections.Range(func(addr string, p Peer) bool {
		if exceptAddr != addr {
			p.Send(m)
		}
		return true
	})
}

//ExceptCastLimit is used to propagate messages to limited number of nodes.
func (pm *manager) ExceptCastLimit(exceptAddr string, m message.Message, Limit int) {
	Count := 0
	pm.connections.Range(func(addr string, p Peer) bool {
		if exceptAddr != addr {
			p.Send(m)
			Count++
		}
		return Count < Limit
	})
}

//TargetCast is used to propagate messages to all nodes.
func (pm *manager) TargetCast(addr string, m message.Message) error {
	if p, has := pm.connections.Load(addr); has {
		p.Send(m)
		return nil
	}
	return ErrNotFoundPeer
}

//NodeList is returns the addresses of the collected peers
func (pm *manager) TestList() []string {
	return []string{pm.TestMsg}
}

//NodeList is returns the addresses of the collected peers
func (pm *manager) NodeList() []string {
	list := make([]string, 0)
	pm.nodes.Range(func(addr string, ci peermessage.ConnectInfo) bool {
		list = append(list, "no"+addr+":"+strconv.Itoa(ci.PingScoreBoard.Len()))
		return true
	})
	return list
}

//ConnectedList is returns the address of the node waiting for the operation.
func (pm *manager) ConnectedList() []string {
	list := make([]string, 0)
	pm.connections.Range(func(addr string, p Peer) bool {
		addr = "c" + strings.Replace(addr, "testid", "", 0)
		list = append(list, addr)
		return true
	})
	return list
}

//candidateList is returns the address of the node waiting for the operation.
func (pm *manager) CandidateList() []string {
	list := make([]string, 0)
	pm.candidates.rangeMap(func(addr string, cs candidateState) bool {
		list = append(list, "ca"+addr+":"+strconv.Itoa(int(cs)))
		return true
	})
	return list
}

//GroupList returns a list of peer groups.
func (pm *manager) GroupList() []string {
	return pm.peerStorage.List()
}

// func (pm *manager) peerListHandler(m message.Message) error {
func (pm *manager) OnRecv(p mesh.Peer, r io.Reader, t message.Type) error {
	m, err := pm.MessageManager.ParseMessage(r, t)
	if err != nil {
		return err
	}

	switch m.(type) {
	case *peermessage.PeerList:
		peerList := m.(*peermessage.PeerList)
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
			defer pm.peerGroupLock.Unlock()

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
			}

			if p, connectionHas := pm.connections.Load(peerList.From); connectionHas {
				for _, ci := range peerList.List {
					if connectionHas {
						pm.updateScoreBoard(p, ci)
					}
				}
				pm.addReadyConn(p)
			}

		}

	}

	return nil
}

func (pm *manager) updateScoreBoard(p Peer, ci peermessage.ConnectInfo) {
	addr := p.NetAddr()

	node := pm.nodes.LoadOrStore(addr, peermessage.NewConnectInfo(addr, p.PingTime()))
	node.PingScoreBoard.Store(ci.Address, ci.PingTime, p.LocalAddr().String()+" "+p.NetAddr()+" ")
}

func (pm *manager) doManageCandidate(addr string, cs candidateState) error {
	if strings.HasPrefix(addr, pm.router.Localhost()) {
		go pm.candidates.delete(addr)
	}
	var err error
	switch cs {
	case csRequestWait:
		// go func(addr string) {
		/*err = */
		pm.router.Request(addr)
		// if err != nil {
		// pm.errLog("RequestWait err ", err)
		// }
		// }(addr)
	case csPeerListWait:
		if p, has := pm.connections.Load(addr); has {
			peermessage.SendRequestPeerList(p, p.LocalAddr().String())
		} else {
			go pm.candidates.store(addr, csRequestWait)
		}
	}
	return err
}

func (pm *manager) manageCandidate() {
	for {
		if pm.peerStorage.NotEnoughPeer() {
			time.Sleep(time.Second * 5)
		} else {
			time.Sleep(time.Second * 30)
		}
		pm.candidates.rangeMap(func(addr string, cs candidateState) bool {
			pm.doManageCandidate(addr, cs)
			time.Sleep(time.Millisecond * 50)
			return true
		})
	}
}

func (pm *manager) rotatePeer() {
	for {
		if pm.peerStorage.NotEnoughPeer() {
			time.Sleep(time.Second * 5)
		} else {
			time.Sleep(time.Minute * 20)
		}

		pm.appendPeerStorage()
	}
}

func (pm *manager) appendPeerStorage() {
	var len int
	pm.connections.Range(func(k string, p Peer) bool {
		len++
		return true
	})
	if len == 0 {
		return
	}

	if len == 1 {
		pm.connections.Range(func(k string, p Peer) bool {
			peermessage.SendRequestPeerList(p, p.LocalAddr().String())
			return false
		})
	}

	for i := pm.nodeRotateIndex; i < pm.nodes.Len(); i++ {
		p := pm.nodes.Get(i)
		pm.nodeRotateIndex = i + 1
		if pm.peerStorage.Have(p.Address) {
			continue
		}
		if connectedPeer, has := pm.connections.Load(p.Address); has {
			pm.addReadyConn(connectedPeer)
		} else {
			err := pm.router.Request(p.Address)
			if err != nil {
				// pm.errLog("PeerListHandler err ", pm.router.Localhost(), err)
			}
		}

		break
	}
	if pm.nodeRotateIndex >= pm.nodes.Len()-1 {
		pm.nodeRotateIndex = 0
	}
}

func (pm *manager) kickOutPeerStorage(ip storage.Peer) {
	if p, ok := ip.(Peer); ok {
		var len int
		pm.connections.Range(func(k string, p Peer) bool {
			len++
			return true
		})

		if len > storage.MaxPeerStorageLen()*2 {
			closePeer := p
			pm.connections.Range(func(addr string, p Peer) bool {
				if closePeer.ConnectedTime() > p.ConnectedTime() {
					closePeer = p
				}
				return true
			})
			closePeer.Close()
		}
	}
}

func (pm *manager) deletePeer(addr string) {
	pm.connections.Delete(addr)
	pm.eventHandlerLock.RLock()
	if p, has := pm.connections.Load(addr); has {
		for _, eh := range pm.eventHandler {
			eh.OnDisconnected(p)
		}
	}
	pm.eventHandlerLock.RUnlock()
}

func (pm *manager) addPeer(p Peer) error {
	pm.peerGroupLock.Lock()
	defer pm.peerGroupLock.Unlock()

	if oldP, has := pm.connections.Load(p.NetAddr()); has {
		oldP.Close() //deletePeer, conn.Close()
	}
	{
		addr := p.NetAddr()
		pm.connections.Store(addr, p)
		pm.nodes.Store(addr, peermessage.NewConnectInfo(addr, p.PingTime()))
		pm.candidates.store(addr, csPeerListWait)

		go func(p Peer) {
			peermessage.SendRequestPeerList(p, p.LocalAddr().String())
		}(p)
	}
	return nil
}

func (pm *manager) addReadyConn(p Peer) {
	pm.peerStorage.Add(p, func(addr string) (time.Duration, bool) {
		if node, has := pm.nodes.Load(addr); has {
			return node.PingScoreBoard.Load(addr)
		}
		return 0, false
	})

}

/***
 * implament of mage interface
**/
// AddNode is used to register additional peers from outside.
func (pm *manager) Add(netAddr string, doForce bool) {
	err := pm.router.Request(netAddr)
	if err != nil {
		// pm.errLog("RequestWait err ", err, netAddr)
	}
}

func (pm *manager) Remove(netAddr string) {
	p, has := pm.connections.Load(netAddr)
	if has {
		p.Close()
	}
}
func (pm *manager) RemoveByID(ID string) {
	pm.Remove(ID)
}

type BanPeerInfo struct {
	NetAddr  string
	Timeout  int64
	OverTime int64
}

func (p BanPeerInfo) String() string {
	return fmt.Sprintf("%s Ban over %d", p.NetAddr, p.OverTime)
}

// ByTime implements sort.Interface for []BanPeerInfo on the Timeout field.
type ByTime struct {
	Arr []*BanPeerInfo
	Map map[string]*BanPeerInfo
}

func NewByTime() *ByTime {
	return &ByTime{
		Arr: []*BanPeerInfo{},
		Map: map[string]*BanPeerInfo{},
	}
}

func (a *ByTime) Len() int           { return len(a.Arr) }
func (a *ByTime) Swap(i, j int)      { a.Arr[i], a.Arr[j] = a.Arr[j], a.Arr[i] }
func (a *ByTime) Less(i, j int) bool { return a.Arr[i].Timeout < a.Arr[j].Timeout }

func (a *ByTime) Add(netAddr string, Seconds int64) {
	b, has := a.Map[netAddr]
	if !has {
		b = &BanPeerInfo{
			NetAddr:  netAddr,
			Timeout:  time.Now().UnixNano() + (int64(time.Second) * Seconds),
			OverTime: Seconds,
		}
		a.Arr = append(a.Arr, b)
		a.Map[netAddr] = b
	} else {
		b.Timeout = Seconds
	}
	sort.Sort(a)
}

func (a *ByTime) Delete(netAddr string) {
	i := a.Search(netAddr)
	if i < 0 {
		return
	}

	b := a.Arr[i]
	a.Arr = append(a.Arr[:i], a.Arr[i+1:]...)
	delete(a.Map, b.NetAddr)
}

func (a *ByTime) Search(netAddr string) int {
	b, has := a.Map[netAddr]
	if !has {
		return -1
	}

	i, j := 0, len(a.Arr)
	for i < j {
		h := int(uint(i+j) >> 1) // avoid overflow when computing h
		// i ≤ h < j
		if !(a.Arr[h].Timeout >= b.Timeout) {
			i = h + 1 // preserves f(i-1) == false
		} else {
			j = h // preserves f(j) == true
		}
	}
	// i == j, f(i-1) == false, and f(j) (= f(i)) == true  =>  answer is i.
	return i
}

func (a *ByTime) IsBan(netAddr string) bool {
	now := time.Now().UnixNano()
	var slicePivot = 0
	for i, b := range a.Arr {
		if now < b.Timeout {
			slicePivot = i
			break
		}
		delete(a.Map, a.Arr[i].NetAddr)
	}

	a.Arr = a.Arr[slicePivot:]
	_, has := a.Map[netAddr]
	return has
}

func (pm *manager) Ban(netAddr string, Seconds uint32) {
	pm.BanPeerInfos.Add(netAddr, int64(Seconds))
	p, has := pm.connections.Load(netAddr)
	if has {
		p.Close()
	}
}

func (pm *manager) BanByID(ID string, Seconds uint32) {
	pm.Ban(ID, Seconds)
}

func (pm *manager) Unban(netAddr string) {
	pm.BanPeerInfos.Delete(netAddr)
}

func (pm *manager) Peers() []mesh.Peer {
	list := make([]mesh.Peer, 0)
	pm.connections.Range(func(addr string, p Peer) bool {
		list = append(list, p)
		return true
	})
	return list
}

//OnConnected is empty BaseEventHandler functions
func (pm *manager) OnConnected(p mesh.Peer) {}

//OnDisconnected is empty BaseEventHandler functions
func (pm *manager) OnDisconnected(p mesh.Peer) {}
