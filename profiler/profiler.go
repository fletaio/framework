package profiler

import (
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/v2pro/plz/gls"
)

// Profiler TODO
type Profiler struct {
	sync.RWMutex
	Caller   string
	GOID     int64
	ParentID string
	DataHash map[string]*dataLockable
	lastTag  string
}

// Data TODO
type Data struct {
	Tag        string
	CallCount  int64
	TSum       int64
	tLastEnter int64
}

// dataLockable TODO
type dataLockable struct {
	sync.RWMutex
	Data
}

var hashLock sync.Mutex
var profilerHash = map[string]*Profiler{}
var stackHash = map[int64][]*Profiler{}

func retainProfiler() *Profiler {
	Caller := caller(4)
	GOID := gls.GoID()

	hashLock.Lock()
	defer hashLock.Unlock()

	id := Caller + "#" + strconv.FormatInt(GOID, 10)
	p, has := profilerHash[id]
	if !has {
		p = &Profiler{
			Caller:   Caller,
			GOID:     GOID,
			DataHash: map[string]*dataLockable{},
		}
		profilerHash[id] = p
	}
	stack, has := stackHash[GOID]
	if !has {
		stack = []*Profiler{}
	}
	if len(stack) > 0 {
		p.ParentID = stack[len(stack)-1].ID()
	}
	stackHash[GOID] = append(stack, p)
	return p
}

func releaseProfiler(p *Profiler) {
	hashLock.Lock()
	defer hashLock.Unlock()

	stack := stackHash[p.GOID]
	stackHash[p.GOID] = stack[:len(stack)-1]
}

// ID TODO
func (p *Profiler) ID() string {
	return p.Caller + "#" + strconv.FormatInt(p.GOID, 10)
}

// Enter TODO
func Enter() *Profiler {
	return EnterTag("")
}

// EnterTag TODO
func EnterTag(Tag string) *Profiler {
	p := retainProfiler()
	p.Lock()
	p.lastTag = Tag
	data, has := p.DataHash[Tag]
	if !has {
		data = &dataLockable{}
		p.DataHash[Tag] = data
	}
	p.Unlock()
	data.Lock()
	data.CallCount++
	data.tLastEnter = time.Now().UnixNano()
	data.Unlock()
	//log.Println("Enter", Tag, p.ID())
	return p
}

// Exit TODO
func (p *Profiler) Exit() {
	defer releaseProfiler(p)
	p.RLock()
	data := p.DataHash[p.lastTag]
	p.RUnlock()
	TNow := time.Now().UnixNano()
	data.Lock()
	data.TSum += TNow - data.tLastEnter
	data.Unlock()
	//log.Println("Exit", p.lastTag, p.ID(), time.Duration(TNow-data.tLastEnter), time.Duration(data.TSum))
}

// EachFunc TODO
type EachFunc func(Caller string, GOID int64, tag string, data Data)

// Each TODO
func (p *Profiler) Each(fn EachFunc) {
	hash := map[string]Data{}
	p.RLock()
	for tag, data := range p.DataHash {
		data.Lock()
		hash[tag] = data.Data
		data.Unlock()
	}
	Caller := p.Caller
	GOID := p.GOID
	p.RUnlock()
	for tag, data := range hash {
		fn(Caller, GOID, tag, data)
	}
}

// ReadFunc TODO
type ReadFunc func(Caller string, GOID int64, data Data)

// Read TODO
func (p *Profiler) Read(Tag string, fn ReadFunc) {
	p.RLock()
	data := p.DataHash[Tag]
	Caller := p.Caller
	GOID := p.GOID
	p.RUnlock()
	data.Lock()
	fn(Caller, GOID, data.Data)
	data.Unlock()
}

func caller(skip int) string {
	fpcs := make([]uintptr, 1)
	n := runtime.Callers(skip, fpcs)
	if n == 0 {
		return "nil"
	}

	pc := fpcs[0] - 1
	fun := runtime.FuncForPC(pc)
	if fun == nil {
		return "nil"
	}
	return fun.Name()
}
