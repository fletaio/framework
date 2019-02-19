package chain

import (
	"io"
	"sync"
	"time"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/common/hash"
	"git.fleta.io/fleta/common/queue"
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
	dataQ     *queue.SortedQueue
	manager   *message.Manager
	statusMap map[string]*Status

	isRunning    bool
	processLock  sync.Mutex
	requestLock  sync.Mutex
	requestTimer *RequestTimer
}

// NewManager returns a Manager
func NewManager(cn Chain) *Manager {
	cm := &Manager{
		chain:     cn,
		dataQ:     queue.NewSortedQueue(),
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

// Provider returns the provider of the chain
func (cm *Manager) Provider() Provider {
	return cm.chain.Provider()
}

// OnConnected is called after accepting a peer to the peer list
func (cm *Manager) OnConnected(p mesh.Peer) {
	cm.Lock()
	defer cm.Unlock()

	cm.statusMap[p.ID()] = &Status{}

	cp := cm.Provider()
	p.Send(&StatusMessage{
		Version:  cp.Version(),
		Height:   cp.Height(),
		PrevHash: cp.PrevHash(),
	})
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
	m, err := cm.manager.ParseMessage(r, t)
	if err != nil {
		return err
	}

	switch msg := m.(type) {
	case *HeaderMessage:
		cm.Lock()
		if status, has := cm.statusMap[p.ID()]; has {
			if status.Height < msg.Header.Height() {
				status.Height = msg.Header.Height()
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
		} else if msg.Header.Height()-height < 10 {
			cm.tryRequestData(msg.Header.Height(), 1)
		} else {
			cm.tryRequestData(height+1, 1)
		}
		return nil
	case *DataMessage:
		if err := cm.AddData(msg.Data); err != nil {
			return err
		}

		cm.Lock()
		if status, has := cm.statusMap[p.ID()]; has {
			if status.Height < msg.Data.Header.Height() {
				status.Height = msg.Data.Header.Height()
			}
		}
		cm.Unlock()

		cm.tryRequestData(msg.Data.Header.Height(), 1)
		return nil
	case *RequestMessage:
		cd, err := cm.Provider().Data(msg.Height)
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
		cm.Lock()
		if status, has := cm.statusMap[p.ID()]; has {
			if status.Height < msg.Height {
				status.Version = msg.Version
				status.Height = msg.Height
				status.PrevHash = msg.PrevHash
			}
		}
		cm.Unlock()

		height := cm.Provider().Height()
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
				cm.Unlock()
				// TODO : fork detected
				panic(ErrForkDetected) // TEMP
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

	timer := time.NewTimer(10 * time.Second)
	for {
		select {
		case <-timer.C:
			cm.tryRequestData(cm.chain.Provider().Height(), DataFetchHeightDiffMax)
			timer.Reset(10 * time.Second)
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
		cm.tryRequestData(cd.Header.Height(), DataFetchHeightDiffMax)
		targetHeight++
		item = cm.dataQ.PopUntil(targetHeight)
	}
}

func (cm *Manager) tryRequestData(From uint32, Count uint32) {
	cm.requestLock.Lock()
	defer cm.requestLock.Unlock()

	if Count > DataFetchHeightDiffMax {
		Count = DataFetchHeightDiffMax
	}

	height := cm.Provider().Height()
	if From <= height {
		Diff := (height - From)
		if Diff >= Count {
			return
		}
		Count -= Diff
		From = height + 1
	}
	for TargetHeight := From; TargetHeight < From+Count; TargetHeight++ {
		if cm.dataQ.Find(uint64(TargetHeight)) == nil {
			if !cm.requestTimer.Exist(TargetHeight) {
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

// BroadcastHeader sends the header to all of its peers
func (cm *Manager) BroadcastHeader(ch Header) {
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
