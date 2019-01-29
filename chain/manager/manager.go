package manager

import (
	"io"
	"log"
	"sync"
	"time"

	"git.fleta.io/fleta/common/hash"
	"git.fleta.io/fleta/framework/chain"
	"git.fleta.io/fleta/framework/chain/mesh"
	"git.fleta.io/fleta/framework/message"
)

// consts
const (
	DataFetchHeightDiffMax = 10
)

// RecvDeligator is used when the Manager cannot handle the message type
type RecvDeligator interface {
	OnRecv(p mesh.Peer, r io.Reader, t message.Type) error
}

// Status is a status of a peer
type Status struct {
	Version  uint16
	Height   uint32
	PrevHash hash.Hash256
}

// Manager synchronizes the chain via the mesh
type Manager struct {
	sync.Mutex
	Mesh      mesh.Mesh
	Deligator RecvDeligator

	chain        *chain.Chain
	pool         *Pool
	manager      *message.Manager
	statusMap    map[string]*Status
	processLock  sync.Mutex
	generateLock sync.Mutex
	requestLock  sync.Mutex
	requestTimer *RequestTimer
}

// NewManager returns a Manager
func NewManager(cn *chain.Chain) *Manager {
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

// BeforeConnect is called before accepting a peer to the peer list
func (cm *Manager) BeforeConnect(p mesh.Peer) error {
	return nil
}

// AfterConnect is called after accepting a peer to the peer list
func (cm *Manager) AfterConnect(p mesh.Peer) {
	cm.Lock()
	defer cm.Unlock()

	cm.statusMap[p.ID()] = &Status{}
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
func (cm *Manager) OnRecv(p mesh.Peer, r io.Reader, t message.Type) error {
	cm.Lock()
	defer cm.Unlock()

	cp := cm.chain.Provider()

	m, err := cm.manager.ParseMessage(r, t)
	if err != nil {
		if err != message.ErrUnknownMessage || cm.Deligator == nil {
			return err
		} else {
			return cm.Deligator.OnRecv(p, r, t)
		}
	}
	switch msg := m.(type) {
	case *HeaderMessage:
		status := cm.statusMap[p.ID()]
		if status.Height < msg.Header.Height {
			status.Height = msg.Header.Height
		}

		height := cp.Height()
		if msg.Header.Height <= height {
			if err := cm.chain.CheckFork(msg.Header, msg.Signatures); err != nil {
				if err == chain.ErrForkDetected {
					// TODO : fork detected
					panic(err) // TEMP
				} else {
					return err
				}
			} else {
				cm.Mesh.BanByID(p.ID(), 0)
				return err
			}
		} else if msg.Header.Height-height < 10 {
			cm.tryRequestData(msg.Header.Height, 1)
		}
		return nil
	case *DataMessage:
		status := cm.statusMap[p.ID()]
		if status.Height < msg.Data.Header.Height {
			status.Height = msg.Data.Header.Height
		}

		if msg.Data.Header.Height <= cp.Height() {
			if err := cm.chain.CheckFork(&msg.Data.Header, msg.Data.Signatures); err != nil {
				if err == chain.ErrForkDetected {
					// TODO : fork detected
					panic(err) // TEMP
				} else {
					return err
				}
			} else {
				cm.Mesh.BanByID(p.ID(), 0)
				return err
			}
		} else {
			if err := cm.chain.Screening(msg.Data); err != nil {
				return err
			}
			if err := cm.pool.Push(msg.Data); err != nil {
				if err == chain.ErrForkDetected {
					// TODO : fork detected
					panic(err) // TEMP
				} else {
					return err
				}
			}
			cm.tryProcess()
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
		status.Version = msg.Version
		status.Height = msg.Height
		status.PrevHash = msg.PrevHash

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

// Run is the main loop of Manager
func (cm *Manager) Run() {
	go cm.requestTimer.Run()

	cp := cm.chain.Provider()
	timer := time.NewTimer(10 * time.Second)
	for {
		select {
		case <-timer.C:
			height := cp.Height()
			cm.tryRequestData(height, DataFetchHeightDiffMax)
			cm.TryGenerate()
			timer.Reset(10 * time.Second)
		}
	}
}

// TryGenerate try to next block of the chain
func (cm *Manager) TryGenerate() {
	cm.generateLock.Lock()
	defer cm.generateLock.Unlock()

	if cd, err := cm.chain.Generate(); err != nil {
		log.Println(err)
	} else if cd != nil {
		cm.broadcastHeader(&cd.Header)
		go cm.TryGenerate()
	}
}

func (cm *Manager) tryProcess() {
	cm.processLock.Lock()
	defer cm.processLock.Unlock()

	cp := cm.chain.Provider()
	item := cm.pool.Pop(cp.Height() + 1)
	for item != nil {
		if err := cm.chain.Process(item, nil); err != nil {
			log.Println(err)
		} else {
			cm.broadcastHeader(&item.Header)
			cm.tryRequestData(item.Header.Height, DataFetchHeightDiffMax)
		}
		item = cm.pool.Pop(cp.Height() + 1)
	}
}

func (cm *Manager) tryRequestData(From uint32, Count uint32) {
	cm.requestLock.Lock()
	defer cm.requestLock.Unlock()

	cp := cm.chain.Provider()
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
					ph := cm.statusMap[p.ID()].Height

					if ph >= TargetHeight {
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

func (cm *Manager) broadcastHeader(ch *chain.Header) {
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
	cp := cm.chain.Provider()
	cm.Lock()
	msg := &StatusMessage{
		Version:  cp.Version(),
		Height:   cp.Height(),
		PrevHash: cp.PrevHash(),
	}
	cm.Unlock()

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
			Header: &chain.Header{},
		}
		if _, err := msg.ReadFrom(r); err != nil {
			return nil, err
		}
		return msg, nil
	case DataMessageType:
		msg := &DataMessage{
			Data: &chain.Data{},
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
		msg := &RequestMessage{}
		if _, err := msg.ReadFrom(r); err != nil {
			return nil, err
		}
		return msg, nil
	default:
		return nil, message.ErrUnknownMessage
	}
}
