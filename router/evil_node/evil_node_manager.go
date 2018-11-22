package evilnode

import (
	"fmt"
	"time"

	"git.fleta.io/fleta/framework/log"
	"github.com/dgraph-io/badger"
)

// Config is router config
type Config struct {
	StorePath    string
	BanEvilScore uint16
}

type NoticeEvil interface {
	NewEvilNode(string)
}

type EvilNodeManager struct {
	Config     *Config
	List       *ConnList
	NoticeList map[string]NoticeEvil
}

// KindOfEvil is define evil table type
type KindOfEvil uint16

// evilnode evil score table
const (
	DialFail KindOfEvil = 40
)

const reduceEvilScorePerMinute uint16 = 1

func NewEvilNodeManager(c *Config) *EvilNodeManager {
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
		return nil
	}

	return &EvilNodeManager{
		Config:     c,
		List:       pl,
		NoticeList: map[string]NoticeEvil{},
	}
}

// RegisterNoticeEvilNode is Registered notice EvilNode
func (r *EvilNodeManager) RegisterNoticeEvilNode(ne NoticeEvil) {
	addr := fmt.Sprintf("%p", ne)
	r.NoticeList[addr] = ne
}

// IsBanNode is return true value when the target node over the base evil score
func (r *EvilNodeManager) IsBanNode(addr string) bool {
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
			log.Error("EvilNodeManager IsBanNode ", err)
			return false
		}
	}

	passedSecond := (time.Now().Sub(pi.Time) / time.Second) * time.Duration(reduceEvilScorePerMinute)

	var evilScore uint16
	if pi.EvilScore < uint16(passedSecond) {
		evilScore = 0
	} else {
		evilScore = pi.EvilScore - uint16(passedSecond)
	}

	log.Info("IsBanNode  evilScore : ", addr, " : ", evilScore, passedSecond)

	if evilScore > r.Config.BanEvilScore {
		return true
	}
	return false
}

// TellOn is update nodes evil score
func (r *EvilNodeManager) TellOn(addr string, es KindOfEvil) error {
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

	passedSecond := (time.Now().Sub(pi.Time) / time.Second) * time.Duration(reduceEvilScorePerMinute)
	if pi.EvilScore <= uint16(passedSecond) {
		pi.EvilScore = 0
	} else {
		pi.EvilScore -= uint16(passedSecond)
	}
	pi.EvilScore += uint16(es)
	pi.Time = time.Now()
	log.Info("TellOn ", addr, ":", pi.EvilScore)

	return r.List.Store(pi)
}
