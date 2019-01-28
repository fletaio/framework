package manager

import (
	"sync"
	"time"
)

// RequestTimer triggers a event when a request is expired
type RequestTimer struct {
	sync.Mutex
	timerHash map[uint32]*requestTimerItem
	manager   *Manager
}

// NewRequestTimer returns a RequestTimer
func NewRequestTimer(manager *Manager) *RequestTimer {
	rm := &RequestTimer{
		timerHash: map[uint32]*requestTimerItem{},
		manager:   manager,
	}
	return rm
}

// Exist returns the target height request exists or not
func (rm *RequestTimer) Exist(height uint32) bool {
	rm.Lock()
	defer rm.Unlock()

	_, has := rm.timerHash[height]
	return has
}

// Add adds the timer of the request
func (rm *RequestTimer) Add(height uint32, t time.Duration, ID string) {
	rm.Lock()
	defer rm.Unlock()

	rm.timerHash[height] = &requestTimerItem{
		Height:    height,
		ExpiredAt: uint64(time.Now().UnixNano()) + uint64(t),
		ID:        ID,
	}
}

// Run is the main loop of RequestTimer
func (rm *RequestTimer) Run() {
	timer := time.NewTimer(100 * time.Millisecond)
	for {
		select {
		case <-timer.C:
			rm.Lock()
			expired := []*requestTimerItem{}
			now := uint64(time.Now().UnixNano())
			remainHash := map[uint32]*requestTimerItem{}
			for h, v := range rm.timerHash {
				if v.ExpiredAt <= now {
					expired = append(expired, v)
				} else {
					remainHash[h] = v
				}
			}
			rm.timerHash = remainHash
			rm.Unlock()

			for _, v := range expired {
				rm.manager.OnTimerExpired(v.Height, v.ID)
			}
			timer.Reset(100 * time.Millisecond)
		}
	}
}

type requestTimerItem struct {
	Height    uint32
	ExpiredAt uint64
	ID        string
}
