package chain

import (
	"io"
	"sync"
	"time"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/common/hash"
	"git.fleta.io/fleta/framework/chain/mesh"
	"git.fleta.io/fleta/framework/message"
)

// consts
const (
	DataFetchHeightDiffMax = 10
)

// Status is a status of a peer
type Status struct {
	Version  uint16
	Height   uint32
	PrevHash hash.Hash256
}

// Manager synchronizes the chain via the mesh
type Manager struct {
	sync.Mutex
	Mesh mesh.Mesh

	chain     Chain
	pool      *Pool
	manager   *message.Manager
	statusMap map[string]*Status

	isRunning    bool
	isClose      bool
	closeLock    sync.RWMutex
	processLock  sync.Mutex
	requestLock  sync.Mutex
	requestTimer *RequestTimer
}

// NewManager returns a Manager
func NewManager(cn Chain) *Manager {
	cm := &Manager{
		chain:     cn,
		pool:      NewPool(),
		manager:   message.NewManager(),
		statusMap: map[string]*Status{},
	}
	cm.manager.SetCreator(HeaderMessageType, cm.messageCreator)
	cm.manager.SetCreator(DataMessageType, cm.messageCreator)
	cm.manager.SetCreator(RequestMessageType, cm.messageCreator)
	cm.manager.SetCreator(StatusMessageType, cm.messageCreator)
	cm.requestTimer = NewRequestTimer(cm)
	return cm
}

// Init initializes the Manager
func (cm *Manager) Init() error {
	if err := cm.chain.Init(); err != nil {
		return err
	}
	return nil
}

// Close terminates and cleans the chain
func (cm *Manager) Close() {
	cm.closeLock.Lock()
	defer cm.closeLock.Unlock()

	cm.isClose = true
	cm.chain.Close()
}

// IsClose returns the close status of the chain
func (cm *Manager) IsClose() bool {
	cm.closeLock.RLock()
	defer cm.closeLock.RUnlock()

	return cm.isClose
}

// Provider returns the provider of the chain
func (cm *Manager) Provider() Provider {
	return cm.chain.Provider()
}

// BeforeConnect is called before accepting a peer to the peer list
func (cm *Manager) BeforeConnect(p mesh.Peer) error {
	return nil
}

// AfterConnect is called after accepting a peer to the peer list
func (cm *Manager) AfterConnect(p mesh.Peer) {
	cm.Lock()
	defer cm.Unlock()

	cm.statusMap[p.ID()] = &Status{}

	cp := cm.Provider()
	msg := &StatusMessage{
		Version:  cp.Version(),
		Height:   cp.Height(),
		PrevHash: cp.PrevHash(),
	}
	p.Send(msg)
}

// OnClosed is called when the peer is closed
func (cm *Manager) OnClosed(p mesh.Peer) {
	cm.Lock()
	defer cm.Unlock()

	delete(cm.statusMap, p.ID())
}

// OnTimerExpired is called when request is expired
func (cm *Manager) OnTimerExpired(height uint32, ID string) {
	cm.Mesh.RemoveByID(ID)
	cm.tryRequestData(height, 1)
}

// OnRecv is called when a message is received from the peer
func (cm *Manager) OnRecv(p mesh.Peer, t message.Type, r io.Reader) error {
	cm.Lock()
	defer cm.Unlock()

	cp := cm.Provider()

	m, err := cm.manager.ParseMessage(r, t)
	if err != nil {
		if err != message.ErrUnknownMessage {
			return err
		} else {
			return cm.chain.OnRecv(p, t, r)
		}
	}

	switch msg := m.(type) {
	case *HeaderMessage:
		status := cm.statusMap[p.ID()]
		if status.Height < msg.Header.Height() {
			status.Height = msg.Header.Height()
		}

		height := cp.Height()
		if msg.Header.Height() <= height {
			if err := cm.checkFork(msg.Header, msg.Signatures); err != nil {
				if err == ErrForkDetected {
					// TODO : fork detected
					panic(err) // TEMP
				} else {
					return err
				}
			}
		} else if msg.Header.Height()-height < 10 {
			cm.tryRequestData(msg.Header.Height(), 1)
		}
		return nil
	case *DataMessage:
		status := cm.statusMap[p.ID()]
		if status.Height < msg.Data.Header.Height() {
			status.Height = msg.Data.Header.Height()
		}
		if err := cm.AddData(msg.Data); err != nil {
			return err
		}
		return nil
	case *RequestMessage:
		cd, err := cp.Data(msg.Height)
		if err != nil {
			return err
		}
		sm := &DataMessage{
			Data: cd,
		}
		if err := p.Send(sm); err != nil {
			return err
		}
		return nil
	case *StatusMessage:
		status := cm.statusMap[p.ID()]
		if status.Height < msg.Height {
			status.Version = msg.Version
			status.Height = msg.Height
			status.PrevHash = msg.PrevHash
		}

		height := cp.Height()
		if msg.Height > height {
			diff := msg.Height - height
			if diff > DataFetchHeightDiffMax {
				diff = DataFetchHeightDiffMax
			}
			cm.tryRequestData(height, diff)
		}
		return nil
	default:
		return message.ErrUnhandledMessage
	}
}

// AddData TODO
func (cm *Manager) AddData(cd *Data) error {
	cp := cm.Provider()
	if cd.Header.Height() <= cp.Height() {
		if err := cm.checkFork(cd.Header, cd.Signatures); err != nil {
			if err == ErrForkDetected {
				// TODO : fork detected
				panic(err) // TEMP
			} else {
				return err
			}
		}
	} else {
		if err := cm.chain.Screening(cd); err != nil {
			return err
		}
		if err := cm.pool.Push(cd); err != nil {
			if err == ErrForkDetected {
				// TODO : fork detected
				panic(err) // TEMP
			} else {
				return err
			}
		}
		go cm.tryProcess()
	}
	return nil
}

// Run is the main loop of Manager
func (cm *Manager) Run() {
	cm.Lock()
	if cm.isRunning {
		cm.Unlock()
		return
	}
	cm.isRunning = true
	cm.Unlock()

	go cm.requestTimer.Run()

	cp := cm.chain.Provider()
	timer := time.NewTimer(10 * time.Second)
	for {
		select {
		case <-timer.C:
			height := cp.Height()
			cm.tryRequestData(height, DataFetchHeightDiffMax)
			timer.Reset(10 * time.Second)
		}
	}
}

// WaitForSync waits until required peers are connected and the chain is synced with them
func (cm *Manager) WaitForSync(RequiredPeerCount int) {
	cp := cm.chain.Provider()
	timer := time.NewTimer(10 * time.Second)
	for {
		select {
		case <-timer.C:
			height := cp.Height()
			if len(cm.statusMap) >= RequiredPeerCount {
				Count := 0
				isSynced := true
				for _, v := range cm.statusMap {
					if v.Version > 0 {
						Count++
						if height < v.Height {
							isSynced = false
							break
						}
					}
				}
				if Count >= RequiredPeerCount {
					if isSynced {
						return
					}
				}
			}
			timer.Reset(10 * time.Second)
		}
	}
}

func (cm *Manager) tryProcess() {
	cm.processLock.Lock()
	defer cm.processLock.Unlock()

	cp := cm.Provider()
	item := cm.pool.Pop(cp.Height() + 1)
	for item != nil {
		if err := cm.ProcessAndBroadcast(item, nil); err != nil {
			return
		}
		cm.tryRequestData(item.Header.Height(), DataFetchHeightDiffMax)
		item = cm.pool.Pop(cp.Height() + 1)
	}
}

func (cm *Manager) tryRequestData(From uint32, Count uint32) {
	cm.requestLock.Lock()
	defer cm.requestLock.Unlock()

	cp := cm.Provider()
	height := cp.Height()
	for TargetHeight := From; TargetHeight < From+Count; TargetHeight++ {
		if TargetHeight <= height {
			continue
		}
		if TargetHeight > height+DataFetchHeightDiffMax {
			continue
		}
		if !cm.pool.Exist(TargetHeight) {
			if !cm.requestTimer.Exist(TargetHeight) {
				list := cm.Mesh.Peers()
				for _, p := range list {
					if ph, has := cm.statusMap[p.ID()]; has {
						if ph.Height >= TargetHeight {
							sm := &RequestMessage{
								Height: TargetHeight,
							}
							if err := p.Send(sm); err != nil {
								cm.Mesh.Remove(p.NetAddr())
							} else {
								cm.requestTimer.Add(TargetHeight, 10*time.Second, p.ID())
								break
							}
						}
					}
				}
			}
		}
	}
}

func (cm *Manager) checkFork(fh Header, sigs []common.Signature) error {
	cm.closeLock.RLock()
	defer cm.closeLock.RUnlock()
	if cm.isClose {
		return ErrChainClosed
	}

	cp := cm.chain.Provider()
	if fh.Height() <= cp.Height() {
		ch, err := cp.Header(fh.Height())
		if err != nil {
			return err
		}
		if !ch.Hash().Equal(fh.Hash()) {
			if ch.PrevHash().Equal(fh.PrevHash()) {
				if err := cm.chain.CheckFork(ch, sigs); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ProcessAndBroadcast processes the chain data and broadcast it if it is processed
func (cm *Manager) ProcessAndBroadcast(cd *Data, UserData interface{}) error {
	if err := cm.process(cd, UserData); err != nil {
		return err
	}
	cm.broadcastHeader(cd.Header)
	return nil
}

func (cm *Manager) process(cd *Data, UserData interface{}) error {
	cm.closeLock.RLock()
	defer cm.closeLock.RUnlock()
	if cm.isClose {
		return ErrChainClosed
	}

	cm.Lock()
	defer cm.Unlock()

	cp := cm.chain.Provider()
	height := cp.Height()
	if cd.Header.Height() != height+1 {
		return ErrInvalidHeight
	}

	if height == 0 {
		if cd.Header.Version() <= 0 {
			return ErrInvalidVersion
		}
		if !cd.Header.PrevHash().Equal(cp.PrevHash()) {
			return ErrInvalidPrevHash
		}
	} else {
		PrevHeader, err := cp.Header(height)
		if err != nil {
			return err
		}
		if cd.Header.Version() < PrevHeader.Version() {
			return ErrInvalidVersion
		}
		if !cd.Header.PrevHash().Equal(PrevHeader.Hash()) {
			return ErrInvalidPrevHash
		}
	}
	if err := cm.chain.Process(cd, UserData); err != nil {
		return err
	}
	return nil
}

func (cm *Manager) broadcastHeader(ch Header) {
	list := cm.Mesh.Peers()
	for _, p := range list {
		sm := &HeaderMessage{
			Header: ch,
		}
		if err := p.Send(sm); err != nil {
			cm.Mesh.Remove(p.NetAddr())
		}
	}
}

func (cm *Manager) broadcastStatus() {
	cp := cm.Provider()
	msg := &StatusMessage{
		Version:  cp.Version(),
		Height:   cp.Height(),
		PrevHash: cp.PrevHash(),
	}

	list := cm.Mesh.Peers()
	for _, p := range list {
		if err := p.Send(msg); err != nil {
			cm.Mesh.Remove(p.NetAddr())
		}
	}
}

func (cm *Manager) messageCreator(r io.Reader, t message.Type) (message.Message, error) {
	switch t {
	case HeaderMessageType:
		msg := &HeaderMessage{
			Header: cm.chain.Provider().CreateHeader(),
		}
		if _, err := msg.ReadFrom(r); err != nil {
			return nil, err
		}
		return msg, nil
	case DataMessageType:
		msg := &DataMessage{
			Data: &Data{
				Header: cm.chain.Provider().CreateHeader(),
				Body:   cm.chain.Provider().CreateBody(),
			},
		}
		if _, err := msg.ReadFrom(r); err != nil {
			return nil, err
		}
		return msg, nil
	case RequestMessageType:
		msg := &RequestMessage{}
		if _, err := msg.ReadFrom(r); err != nil {
			return nil, err
		}
		return msg, nil
	case StatusMessageType:
		msg := &StatusMessage{}
		if _, err := msg.ReadFrom(r); err != nil {
			return nil, err
		}
		return msg, nil
	default:
		return nil, message.ErrUnknownMessage
	}
}
