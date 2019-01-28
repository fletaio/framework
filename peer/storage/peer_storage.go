package storage

import (
	"errors"
	"net"
	"sort"
	"strconv"
	"time"

	"git.fleta.io/fleta/framework/log"
)

// storage errors
var (
	ErrInvalidGroupIndex = errors.New("invalid group index")
)

//peer const list
const (
	groupLength int = 2

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
	kickOut   kickOut
}

// IPeerStorage is a list of functions to be exposed to external sources.
type IPeerStorage interface {
	Add(peer IPeer, scoreFunc Score) bool
	List() []string
	Have(addr string) bool
	NotEnoughPeer() bool
}

// IPeer is a functional list of Peer structures to be used internally.
type IPeer interface {
	RemoteAddr() net.Addr
	LocalAddr() net.Addr
	PingTime() time.Duration
	IsClose() bool
}

// Score is the type of function that scores.
type Score func(string) (time.Duration, bool)

// KickOut is a function called when a peer is not needed.
type kickOut func(p IPeer)

//NewPeerStorage is the PeerStorage creator.
func NewPeerStorage(kickOut kickOut) IPeerStorage {
	ps := &peerStorage{
		peerGroup: map[peerGroupType]nodeList{},
		peerMap:   map[string]*peerInfomation{},
	}
	ps.kickOut = kickOut

	ps.peerGroup[group1] = make([]*peerInfomation, groupLength)
	ps.peerGroup[group2] = make([]*peerInfomation, groupLength)
	ps.peerGroup[group3] = make([]*peerInfomation, groupLength)

	return ps
}

type peerInfomation struct {
	p              IPeer
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
func (ps *peerStorage) Add(p IPeer, score Score) (inserted bool) {
	addr := p.RemoteAddr().String()
	t := p.PingTime()

	if _, has := ps.peerMap[addr]; has {
		ps.updatePingtime(addr)
		return
	}

	if t > distance3 {
		ps.kickOut(p)
		return
	}

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
		if pi != nil {
			index++
			list = append(list, strconv.Itoa(index)+":"+pi.p.RemoteAddr().String()+":"+pi.score().String()+":"+pi.p.PingTime().String())
		}
	}
	for _, pi := range ps.peerGroup[group2] {
		if pi != nil {
			index++
			list = append(list, strconv.Itoa(index)+":"+pi.p.RemoteAddr().String()+":"+pi.score().String()+":"+pi.p.PingTime().String())
		}
	}
	for _, pi := range ps.peerGroup[group3] {
		if pi != nil {
			index++
			list = append(list, strconv.Itoa(index)+":"+pi.p.RemoteAddr().String()+":"+pi.score().String()+":"+pi.p.PingTime().String())
		}
	}

	return list
}

//Have indicates whether the group peer is included or not.
func (ps *peerStorage) Have(addr string) bool {
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
		log.Debug("close peer ", p.p.LocalAddr(), " ", p.p.RemoteAddr())
		ps.kickOut(p.p)
		// p.p.Close()
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
			ps.peerMap[p.p.RemoteAddr().String()] = p
			if deleteEl != nil {
				delete(ps.peerMap, deleteEl.p.RemoteAddr().String())
				ps.insertSort(deleteEl)
			}
			log.Debug("peer insert ", p.p.LocalAddr().String(), ":", p.p.RemoteAddr().String())
			return true
		}
	}
	p.affiliation = 0

	return false
}
