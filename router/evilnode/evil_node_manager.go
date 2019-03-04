package evilnode

import (
	"fmt"
	"time"

	"github.com/fletaio/framework/log"
	"github.com/dgraph-io/badger"
)

// Config is router config
type Config struct {
	StorePath    string
	BanEvilScore uint16
}

// NoticeEvil is interface for notifies the registered object when the evil node appears.
type NoticeEvil interface {
	NewEvilNode(string)
}

// Manager is node evil score manager
type Manager struct {
	Config     *Config
	List       *ConnList
	NoticeList map[string]NoticeEvil
}

// KindOfEvil is define evil table type
type KindOfEvil uint16

// evilnode evil score table
const (
	BadBehaviour KindOfEvil = 40
)

const reduceEvilScorePerMinute uint16 = 1

// NewManager is creator of evilnode Manager
func NewManager(c *Config) *Manager {
	if c == nil {
		c = &Config{
			BanEvilScore: 100,
		}
	}
	if c.StorePath == "" {
		c.StorePath = "./_data/router/"
	}

	pl, err := NewConnList(c.StorePath)
	if err != nil {
		panic(err)
	}

	return &Manager{
		Config:     c,
		List:       pl,
		NoticeList: map[string]NoticeEvil{},
	}
}

// RegisterNoticeEvilNode is Registered notice EvilNode
func (r *Manager) RegisterNoticeEvilNode(ne NoticeEvil) {
	addr := fmt.Sprintf("%p", ne)
	r.NoticeList[addr] = ne
}

// IsBanNode is return true value when the target node over the base evil score
func (r *Manager) IsBanNode(addr string) bool {
	pi, err := r.List.Get(addr)
	if err != nil {
		if err == badger.ErrKeyNotFound {
			pi = ConnectionInfo{
				Addr:      addr,
				EvilScore: 0,
				Time:      time.Now(),
			}
			r.List.Store(pi)
		} else {
			log.Error("Manager IsBanNode ", err)
			return false
		}
	}

	passedSecond := (time.Now().Sub(pi.Time) / time.Minute) * time.Duration(reduceEvilScorePerMinute)

	var evilScore uint16
	if pi.EvilScore < uint16(passedSecond) {
		evilScore = 0
	} else {
		evilScore = pi.EvilScore - uint16(passedSecond)
	}

	if evilScore > r.Config.BanEvilScore {
		log.Info("IsBanNode func evilScore : ", addr, " : ", evilScore, passedSecond)
		return true
	}
	return false
}

// TellOn is update nodes evil score
func (r *Manager) TellOn(addr string, es KindOfEvil) error {
	pi, err := r.List.Get(addr)
	if err != nil {
		if err == badger.ErrKeyNotFound {
			pi = ConnectionInfo{
				Addr:      addr,
				EvilScore: 0,
			}
		} else {
			return err
		}
	}

	passedSecond := (time.Now().Sub(pi.Time) / time.Minute) * time.Duration(reduceEvilScorePerMinute)
	if pi.EvilScore <= uint16(passedSecond) {
		pi.EvilScore = 0
	} else {
		pi.EvilScore -= uint16(passedSecond)
	}
	pi.EvilScore += uint16(es)
	pi.Time = time.Now()
	log.Info("TellOn ", r.Config.StorePath, ":", addr, ":", pi.EvilScore)

	return r.List.Store(pi)
}
