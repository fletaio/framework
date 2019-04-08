package chain

import (
	"io"
	"sync"
	"time"

	"github.com/fletaio/common"
	"github.com/fletaio/common/hash"
	"github.com/fletaio/common/queue"
	"github.com/fletaio/framework/chain/mesh"
	"github.com/fletaio/framework/message"
)

// consts
const (
	DataFetchHeightDiffMax = 100
)

// Status is a status of a peer
type Status struct {
	Version  uint16
	Height   uint32
	LastHash hash.Hash256
}

// Manager synchronizes the chain via the mesh
type Manager struct {
	sync.Mutex
	Mesh mesh.Mesh

	chain     Chain
	dataQ     *queue.SortedQueue
	mm        *message.Manager
	statusMap map[string]*Status

	isRunning    bool
	requestTimer *RequestTimer
	requestLock  sync.Mutex
	processLock  sync.Mutex
}

// NewManager returns a Manager
func NewManager(cn Chain) *Manager {
	cm := &Manager{
		chain:     cn,
		dataQ:     queue.NewSortedQueue(),
		mm:        message.NewManager(),
		statusMap: map[string]*Status{},
	}
	cm.mm.SetCreator(HeaderMessageType, cm.messageCreator)
	cm.mm.SetCreator(DataMessageType, cm.messageCreator)
	cm.mm.SetCreator(RequestMessageType, cm.messageCreator)
	cm.mm.SetCreator(StatusMessageType, cm.messageCreator)
	cm.requestTimer = NewRequestTimer(cm)
	return cm
}

// Provider returns the provider of the chain
func (cm *Manager) Provider() Provider {
	return cm.chain.Provider()
}

// OnConnected is called after accepting a peer to the peer list
func (cm *Manager) OnConnected(p mesh.Peer) {
	cm.Lock()
	cm.statusMap[p.ID()] = &Status{}
	cm.Unlock()

	cp := cm.Provider()
	//cm.chain.DebugLog("chain.Manager", cm.chain.Provider().Height(), "Send StatusMessage", cp.Height())
	p.Send(&StatusMessage{
		Version:  cp.Version(),
		Height:   cp.Height(),
		LastHash: cp.LastHash(),
	})
}

// OnDisconnected is called when the peer is closed
func (cm *Manager) OnDisconnected(p mesh.Peer) {
	cm.Lock()
	defer cm.Unlock()

	delete(cm.statusMap, p.ID())
}

// OnTimerExpired is called when request is expired
func (cm *Manager) OnTimerExpired(height uint32, ID string) {
	cm.Mesh.RemoveByID(ID)
}

// OnRecv is called when a message is received from the peer
func (cm *Manager) OnRecv(p mesh.Peer, r io.Reader, t message.Type) error {
	m, err := cm.mm.ParseMessage(r, t)
	if err != nil {
		return err
	}

	switch msg := m.(type) {
	case *HeaderMessage:
		//cm.chain.DebugLog("chain.Manager", cm.chain.Provider().Height(), "HeaderMessage", msg.Header.Height())
		cm.Lock()
		if status, has := cm.statusMap[p.ID()]; has {
			if status.Height < msg.Header.Height() {
				status.Version = msg.Header.Version()
				status.Height = msg.Header.Height()
				status.LastHash = msg.Header.Hash()
			}
		}
		cm.Unlock()

		height := cm.Provider().Height()
		if msg.Header.Height() <= height {
			if err := cm.checkFork(msg.Header, msg.Signatures); err != nil {
				if err == ErrForkDetected {
					// TODO : fork detected
					panic(err) // TEMP
				} else {
					return err
				}
			}
		}
		return nil
	case *DataMessage:
		//cm.chain.DebugLog("chain.Manager", cm.chain.Provider().Height(), "DataMessage", msg.Data.Header.Height())
		if err := cm.AddData(msg.Data); err != nil {
			return err
		}

		cm.requestTimer.Remove(msg.Data.Header.Height())

		cm.Lock()
		if status, has := cm.statusMap[p.ID()]; has {
			if status.Height < msg.Data.Header.Height() {
				status.Version = msg.Data.Header.Version()
				status.Height = msg.Data.Header.Height()
				status.LastHash = msg.Data.Header.Hash()
			}
		}
		cm.Unlock()
		return nil
	case *RequestMessage:
		//cm.chain.DebugLog("chain.Manager", cm.chain.Provider().Height(), "RequestMessage", msg.Height)
		cd, err := cm.Provider().Data(msg.Height)
		if err != nil {
			return err
		}
		sm := &DataMessage{
			Data: cd,
		}
		//cm.chain.DebugLog("chain.Manager", cm.chain.Provider().Height(), "Send DataMessage", msg.Height)
		if err := p.Send(sm); err != nil {
			return err
		}
		return nil
	case *StatusMessage:
		//cm.chain.DebugLog("chain.Manager", cm.chain.Provider().Height(), "StatusMessage", msg.Height)
		cm.Lock()
		if status, has := cm.statusMap[p.ID()]; has {
			if status.Height < msg.Height {
				status.Version = msg.Version
				status.Height = msg.Height
				status.LastHash = msg.LastHash
			}
		}
		cm.Unlock()
		return nil
	default:
		return message.ErrUnhandledMessage
	}
}

// IsExistData returns the chain data is exist or not in the data queue or the chain
func (cm *Manager) IsExistData(Height uint32) bool {
	if Height <= cm.Provider().Height() {
		return true
	} else {
		return cm.dataQ.Find(uint64(Height)) != nil
	}
}

// AddData pushes a chain data to the chain queue
func (cm *Manager) AddData(cd *Data) error {
	if cd.Header.Height() <= cm.Provider().Height() {
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
		if item := cm.dataQ.FindOrInsert(cd, uint64(cd.Header.Height())); item != nil {
			old := item.(*Data)
			if !old.Header.Hash().Equal(cd.Header.Hash()) {
				// TODO : fork detected
				panic(ErrForkDetected) // TEMP
			}
		}
		cm.tryProcess()
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

	reqTimer := time.NewTimer(time.Millisecond)
	processTimer := time.NewTimer(time.Millisecond)
	for {
		select {
		case <-reqTimer.C:
			cm.tryRequestData(DataFetchHeightDiffMax)
			reqTimer.Reset(100 * time.Millisecond)
		case <-processTimer.C:
			cm.tryProcess()
			processTimer.Reset(100 * time.Millisecond)
		}
	}
}

func (cm *Manager) tryRequestData(Count uint32) {
	cm.requestLock.Lock()
	defer cm.requestLock.Unlock()

	if Count > DataFetchHeightDiffMax {
		Count = DataFetchHeightDiffMax
	}

	From := cm.Provider().Height() + 1
	for TargetHeight := From; TargetHeight < From+Count; TargetHeight++ {
		if !cm.requestTimer.Exist(TargetHeight) {
			if cm.dataQ.Find(uint64(TargetHeight)) == nil {
				list := cm.Mesh.Peers()
				for _, p := range list {
					cm.Lock()
					ph := cm.statusMap[p.ID()]
					is := ph != nil && ph.Height >= TargetHeight
					cm.Unlock()
					if is {
						sm := &RequestMessage{
							Height: TargetHeight,
						}
						//cm.chain.DebugLog("chain.Manager", cm.chain.Provider().Height(), "Send RequestMessage", sm.Height)
						if err := p.Send(sm); err != nil {
							cm.Mesh.Remove(p.NetAddr())
						} else {
							cm.requestTimer.Add(TargetHeight, 5*time.Second, p.ID())
							break
						}
					}
				}
			}
		}
	}
}

func (cm *Manager) tryProcess() {
	cm.processLock.Lock()
	defer cm.processLock.Unlock()

	targetHeight := uint64(cm.Provider().Height() + 1)
	item := cm.dataQ.PopUntil(targetHeight)
	for item != nil {
		cd := item.(*Data)
		if err := cm.Process(cd, nil); err != nil {
			return
		}
		cm.BroadcastHeader(cd.Header)
		//cm.chain.DebugLog("chain.Manager", cm.chain.Provider().Height(), "Block Connected", cd.Header.Height())
		targetHeight++
		item = cm.dataQ.PopUntil(targetHeight)
	}
	cm.tryRequestData(DataFetchHeightDiffMax)
}

func (cm *Manager) checkFork(fh Header, sigs []common.Signature) error {
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

// Process processes the chain data
func (cm *Manager) Process(cd *Data, UserData interface{}) error {
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
		if !cd.Header.PrevHash().Equal(cp.LastHash()) {
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

// BroadcastHeader sends the header to all of its peers
func (cm *Manager) BroadcastHeader(ch Header) {
	list := cm.Mesh.Peers()
	for _, p := range list {
		sm := &HeaderMessage{
			Header: ch,
		}
		//cm.chain.DebugLog("chain.Manager", cm.chain.Provider().Height(), "Send HeaderMessage", ch.Height)
		if err := p.Send(sm); err != nil {
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
