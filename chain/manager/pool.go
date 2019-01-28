package manager

import (
	"git.fleta.io/fleta/common/queue"
	"git.fleta.io/fleta/framework/chain"
)

// Pool holds and provides the required data by the chain
type Pool struct {
	q *queue.SortedQueue
}

// NewPool returns a Pool
func NewPool() *Pool {
	p := &Pool{
		q: queue.NewSortedQueue(),
	}
	return p
}

// Push adds a data to the pool
func (p *Pool) Push(cd *chain.Data) error {
	if item := p.q.Find(uint64(cd.Header.Height)); item != nil {
		old := item.(*chain.Data)
		if !old.Header.Hash().Equal(cd.Header.Hash()) {
			return chain.ErrForkDetected
		}
	}
	p.q.Insert(cd, uint64(cd.Header.Height))
	return nil
}

// Exist returns the data of the target height exists or not
func (p *Pool) Exist(TargetHeight uint32) bool {
	return p.q.Find(uint64(TargetHeight)) != nil
}

// Pop returns the data of the target height
func (p *Pool) Pop(TargetHeight uint32) *chain.Data {
	targetHeight := uint64(TargetHeight)
	for {
		item, height := p.q.Peek()
		if item == nil {
			return nil
		}
		if height < targetHeight {
			p.q.Pop()
		} else if height == targetHeight {
			p.q.Pop()
			return item.(*chain.Data)
		} else {
			return nil
		}
	}
}
