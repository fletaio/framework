package router

import (
	"sync"

	"github.com/fletaio/framework/log"
)

type NamedLock struct {
	lock    sync.RWMutex
	title   string
	name    []string
	arrLock sync.Mutex
}

func NewNamedLock(title string) *NamedLock {
	return &NamedLock{
		title: title,
		name:  []string{},
	}
}

func (n *NamedLock) Lock(name string) {
	if len(n.name) != 0 {
		log.Debug(n.title, "lock by ", n.name, " and wait ", name)
		n.lock.Lock()
		n.name = append(n.name, name)
		log.Debug(n.title, "unlock by ", n.name, " enter ", name)
	} else {
		n.lock.Lock()
		n.name = append(n.name, name)
	}
}

func (n *NamedLock) Unlock() {
	n.name = n.name[1:]
	n.lock.Unlock()
}
func (n *NamedLock) RLock(name string) {
	n.lock.RLock()
}

func (n *NamedLock) RUnlock() {
	n.lock.RUnlock()
}
