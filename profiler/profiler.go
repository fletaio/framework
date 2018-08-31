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
	Caller   string
	GOID     int64
	ParentID string
	DataHash map[string]*ProfileData
	lastTag  string
}

// ProfileData TODO
type ProfileData struct {
	Tag        string
	CallCount  int64
	TSum       int64
	tLastEnter int64
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
			DataHash: map[string]*ProfileData{},
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
	p.lastTag = Tag
	data, has := p.DataHash[Tag]
	if !has {
		data = &ProfileData{}
		p.DataHash[Tag] = data
	}
	data.CallCount++
	data.tLastEnter = time.Now().UnixNano()
	//log.Println("Enter", Tag, p.ID())
	return p
}

// Exit TODO
func (p *Profiler) Exit() {
	defer releaseProfiler(p)
	data := p.DataHash[p.lastTag]
	TNow := time.Now().UnixNano()
	data.TSum += TNow - data.tLastEnter
	//log.Println("Exit", p.lastTag, p.ID(), time.Duration(TNow-data.tLastEnter), time.Duration(data.TSum))
}

func caller(skip int) string {
	// we get the callers as uintptrs - but we just need 1
	fpcs := make([]uintptr, 1)

	// skip 3 levels to get to the caller of whoever called Caller()
	n := runtime.Callers(skip, fpcs)
	if n == 0 {
		return "n/a" // proper error her would be better
	}

	// get the info of the actual function that's in the pointer
	pc := fpcs[0] - 1
	fun := runtime.FuncForPC(pc)
	if fun == nil {
		return "n/a"
	}

	// return its name
	return fun.Name()
}
