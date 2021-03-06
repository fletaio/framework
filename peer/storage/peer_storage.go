package storage

import (
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"
)

// storage errors
var (
	ErrInvalidGroupIndex = errors.New("invalid group index")
)

//peer const list
const (
	groupLength int = 5

	distance1 time.Duration = time.Millisecond * 50
	distance2 time.Duration = time.Millisecond * 250
	distance3 time.Duration = time.Millisecond * 500
)

//MaxPeerStorageLen is returns the maximum number of connections allowed.
func MaxPeerStorageLen() int {
	return groupLength * 3
}

type peerGroupType int

const (
	group1 peerGroupType = 1
	group2 peerGroupType = 2
	group3 peerGroupType = 3
)

type nodeList []*peerInfomation

func (a nodeList) Len() int      { return len(a) }
func (a nodeList) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a nodeList) Less(i, j int) bool {
	var b time.Duration
	var c time.Duration
	if a[i] != nil {
		b = a[i].p.PingTime()
	}
	if a[j] != nil {
		c = a[j].p.PingTime()
	}
	return b < c
}

type peerStorage struct {
	peerGroup map[peerGroupType]nodeList
	peerMap   map[string]*peerInfomation
	mapLock   sync.RWMutex
}

// PeerStorage is a list of functions to be exposed to external sources.
type PeerStorage interface {
	Add(peer Peer, scoreFunc Score) bool
	List() []string
	Have(addr string) bool
	NotEnoughPeer() bool
}

// Peer is a functional list of Peer structures to be used internally.
type Peer interface {
	ID() string
	PingTime() time.Duration
	IsClose() bool
}

// Score is the type of function that scores.
type Score func(string) (time.Duration, bool)

//NewPeerStorage is the PeerStorage creator.
func NewPeerStorage() PeerStorage {
	ps := &peerStorage{
		peerGroup: map[peerGroupType]nodeList{},
		peerMap:   map[string]*peerInfomation{},
	}

	ps.peerGroup[group1] = make([]*peerInfomation, groupLength)
	ps.peerGroup[group2] = make([]*peerInfomation, groupLength)
	ps.peerGroup[group3] = make([]*peerInfomation, groupLength)

	return ps
}

type peerInfomation struct {
	p              Peer
	advantage      *scoreBoard
	registeredTime time.Time
	group          peerGroupType
	affiliation    peerGroupType
}

func (p *peerInfomation) score() (t time.Duration) {
	t = p.p.PingTime()

	if p.group != p.affiliation {
		t -= distance3
	}

	t -= time.Now().Sub(p.registeredTime) / 1000

	t += p.advantage.score[p.affiliation]

	return
}

//Add a new peer
//Scores the peer and determines whether it is included in the group.
func (ps *peerStorage) Add(p Peer, score Score) (inserted bool) {
	addr := p.ID()

	ps.mapLock.RLock()
	if _, has := ps.peerMap[addr]; has {
		ps.updatePingtime(addr)
		return
	}
	ps.mapLock.RUnlock()

	advantage := ps.getScoreBoard(score)
	pi := &peerInfomation{
		p:              p,
		advantage:      advantage,
		registeredTime: time.Now(),
	}
	if pi.p.PingTime() < distance1 {
		pi.group = 1
	} else if pi.p.PingTime() < distance2 {
		pi.group = 2
	} else {
		pi.group = 3
	}

	ps.mapLock.Lock()
	defer ps.mapLock.Unlock()

	return ps.insertSort(pi)
}

//List returns the peers that are included in the group in order.
func (ps *peerStorage) NotEnoughPeer() bool {
	if ps.peerGroup[group1][groupLength-1] == nil {
		return true
	} else if ps.peerGroup[group2][groupLength-1] == nil {
		return true
	} else if ps.peerGroup[group3][groupLength-1] == nil {
		return true
	} else if ps.peerGroup[group1][groupLength-1].p == nil || ps.peerGroup[group1][groupLength-1].p.IsClose() {
		return true
	} else if ps.peerGroup[group2][groupLength-1].p == nil || ps.peerGroup[group2][groupLength-1].p.IsClose() {
		return true
	} else if ps.peerGroup[group3][groupLength-1].p == nil || ps.peerGroup[group3][groupLength-1].p.IsClose() {
		return true
	}
	return false
}

//List returns the peers that are included in the group in order.
func (ps *peerStorage) List() []string {
	list := make([]string, 0)

	index := 0
	for _, pi := range ps.peerGroup[group1] {
		if pi != nil && pi.p.IsClose() {
			index++
			list = append(list, strconv.Itoa(index)+":"+pi.p.ID()+":"+pi.score().String()+":"+pi.p.PingTime().String())
		}
	}
	for _, pi := range ps.peerGroup[group2] {
		if pi != nil && pi.p.IsClose() {
			index++
			list = append(list, strconv.Itoa(index)+":"+pi.p.ID()+":"+pi.score().String()+":"+pi.p.PingTime().String())
		}
	}
	for _, pi := range ps.peerGroup[group3] {
		if pi != nil && pi.p.IsClose() {
			index++
			list = append(list, strconv.Itoa(index)+":"+pi.p.ID()+":"+pi.score().String()+":"+pi.p.PingTime().String())
		}
	}

	return list
}

//Have indicates whether the group peer is included or not.
func (ps *peerStorage) Have(addr string) bool {
	ps.mapLock.RLock()
	defer ps.mapLock.RUnlock()
	_, has := ps.peerMap[addr]
	return has
}

func (ps *peerStorage) updatePingtime(addr string) {
	if p, has := ps.peerMap[addr]; has {
		if g, has := ps.peerGroup[p.group]; has {
			sort.Sort(g)
		}
	}

}

func (ps *peerStorage) insertSort(p *peerInfomation) bool {
	// 자기 그룹부터 시작 ex) 3인경우 3 -> 2 -> 1, 2인경우 2 -> 3 -> 1, 1인경우 1 -> 2-> 3
	if p.group <= group1 && ps.insert(p, group1) {
	} else if p.group <= group2 && ps.insert(p, group2) {
	} else if p.group <= group3 && ps.insert(p, group3) {
	} else if p.group > group2 && ps.insert(p, group2) {
	} else if p.group > group1 && ps.insert(p, group1) {
	} else {
		return false
	}
	return true
}

func (ps *peerStorage) insert(p *peerInfomation, pt peerGroupType) bool {
	p.affiliation = pt
	if nl, has := ps.peerGroup[p.affiliation]; has {
		index := sort.Search(len(nl), func(i int) bool {
			if nl[i] == nil {
				return true
			}
			return nl[i].score() < p.score()
		})

		if index < groupLength {
			deleteEl := nl[groupLength-1]
			copy(nl[index+1:], nl[index:groupLength])
			nl[index] = p
			ps.peerMap[p.p.ID()] = p
			if deleteEl != nil {
				delete(ps.peerMap, deleteEl.p.ID())
				ps.insertSort(deleteEl)
			}
			// log.Debug("peer insert ", ":", p.p.ID())
			return true
		}
	}
	p.affiliation = 0

	return false
}
