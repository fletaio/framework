package chain

import (
	"sync"
	"time"
)

// RequestTimer triggers a event when a request is expired
type RequestTimer struct {
	sync.Mutex
	timerMap map[uint32]*requestTimerItem
	manager  *Manager
}

// NewRequestTimer returns a RequestTimer
func NewRequestTimer(manager *Manager) *RequestTimer {
	rm := &RequestTimer{
		timerMap: map[uint32]*requestTimerItem{},
		manager:  manager,
	}
	return rm
}

// Exist returns the target height request exists or not
func (rm *RequestTimer) Exist(height uint32) bool {
	rm.Lock()
	defer rm.Unlock()

	_, has := rm.timerMap[height]
	return has
}

// Add adds the timer of the request
func (rm *RequestTimer) Add(height uint32, t time.Duration, ID string) {
	rm.Lock()
	defer rm.Unlock()

	rm.timerMap[height] = &requestTimerItem{
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
			remainMap := map[uint32]*requestTimerItem{}
			for h, v := range rm.timerMap {
				if v.ExpiredAt <= now {
					expired = append(expired, v)
				} else {
					remainMap[h] = v
				}
			}
			rm.timerMap = remainMap
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
