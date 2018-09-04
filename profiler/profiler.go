package profiler

import (
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/v2pro/plz/gls"
)

var hashLock sync.Mutex
var profilerHash = map[string]*Profiler{}
var stackHash = map[int64][]*Profiler{}

func retainProfiler() *Profiler {
	Caller, FileName, Line := caller(4)
	GOID := gls.GoID()

	hashLock.Lock()
	defer hashLock.Unlock()

	id := Caller + "#" + strconv.FormatInt(GOID, 10)
	p, has := profilerHash[id]
	if !has {
		p = &Profiler{
			Caller:     Caller,
			GOID:       GOID,
			dataHash:   map[string]*dataLockable{},
			ParentHash: map[string]bool{},
			FileName:   FileName,
			Line:       Line,
		}
		profilerHash[id] = p
	}
	stack, has := stackHash[GOID]
	if !has {
		stack = []*Profiler{}
	}
	if len(stack) > 0 {
		p.ParentHash[stack[len(stack)-1].ID()] = true
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

// Profiler TODO
type Profiler struct {
	sync.RWMutex
	Caller     string
	FileName   string
	Line       int
	GOID       int64
	ParentHash map[string]bool
	dataHash   map[string]*dataLockable
}

// Instance TODO
type Instance struct {
	p          *Profiler
	tag        string
	tLastEnter int64
}

// ID TODO
func (p *Profiler) ID() string {
	return p.Caller + "#" + strconv.FormatInt(p.GOID, 10)
}

// Enter TODO
func Enter() *Instance {
	return EnterTag("")
}

// EnterTag TODO
func EnterTag(Tag string) *Instance {
	p := retainProfiler()
	p.Lock()
	data, has := p.dataHash[Tag]
	if !has {
		data = &dataLockable{
			Data: Data{
				TMin: -1,
				TMax: -1,
			},
		}
		p.dataHash[Tag] = data
	}
	p.Unlock()

	data.Lock()
	data.CallCount++
	data.Unlock()

	return &Instance{
		p:          p,
		tag:        Tag,
		tLastEnter: time.Now().UnixNano(),
	}
}

// Exit TODO
func (pi *Instance) Exit() {
	pi.p.Exit(pi.tag, pi.tLastEnter)
}

// Exit TODO
func (p *Profiler) Exit(Tag string, tLastEnter int64) {
	defer releaseProfiler(p)

	sd := &SnapshotData{}
	p.RLock()
	sd.ID = p.ID()
	Caller := p.Caller
	sd.FileName = p.FileName
	sd.Line = p.Line
	for k := range p.ParentHash {
		PkgName, FuncName := ParseCaller(k)
		sd.Parents = append(sd.Parents, &Call{
			PkgName:  PkgName,
			FuncName: FuncName,
		})
	}
	sd.Tag = Tag
	data := p.dataHash[Tag]
	p.RUnlock()
	sd.PkgName, sd.FuncName = ParseCaller(Caller)

	TNow := time.Now().UnixNano()
	data.Lock()
	TDiff := TNow - tLastEnter
	if data.TMin < 0 || data.TMin < TDiff {
		data.TMin = TDiff
	}
	if data.TMax < 0 || data.TMax > TDiff {
		data.TMax = TDiff
	}
	data.TSum += TDiff
	sd.Data = data.Data
	sd.TDiff = TDiff
	data.Unlock()

	watchChan <- sd
}

const watchMaxCount = 10000

var watchLock sync.Mutex
var watchChan = make(chan *SnapshotData, watchMaxCount)
var watches = make([]*Watcher, 0)

// Watcher TODO
type Watcher struct {
	C chan SnapshotData
}

// Watch TODO
func Watch() *Watcher {
	watcher := &Watcher{
		C: make(chan SnapshotData, watchMaxCount),
	}
	watchLock.Lock()
	watches = append(watches, watcher)
	watchLock.Unlock()
	return watcher
}

func init() {
	go func() {
		for {
			for sd := range watchChan {
				if len(watches) > 0 {
					watchLock.Lock()
					for _, watcher := range watches {
						if len(watcher.C) >= watchMaxCount-1 {
							<-watcher.C
						}
						watcher.C <- *sd
					}
					watchLock.Unlock()
				}
			}
		}
	}()
}

// SnapshotData TODO
type SnapshotData struct {
	ID       string
	PkgName  string
	FuncName string
	FileName string
	Line     int
	Parents  []*Call
	Tag      string
	Data     Data
	TDiff    int64
}

// Call TODO
type Call struct {
	PkgName  string
	FuncName string
}

// Snapshot TODO
func Snapshot() []SnapshotData {
	list := make([]SnapshotData, 0, 1024)
	hashLock.Lock()
	for _, p := range profilerHash {
		p.RLock()
		for tag, data := range p.dataHash {
			sd := SnapshotData{
				ID:       p.ID(),
				FileName: p.FileName,
				Line:     p.Line,
				Parents:  []*Call{},
				Tag:      tag,
			}
			for k := range p.ParentHash {
				PkgName, FuncName := ParseCaller(k)
				sd.Parents = append(sd.Parents, &Call{
					PkgName:  PkgName,
					FuncName: FuncName,
				})
			}
			data.RLock()
			sd.Data = data.Data
			data.RUnlock()
			sd.PkgName, sd.FuncName = ParseCaller(p.Caller)
			list = append(list, sd)
		}
		p.RUnlock()
	}
	hashLock.Unlock()

	return list
}

// dataLockable TODO
type dataLockable struct {
	sync.RWMutex
	Data
}

// Data TODO
type Data struct {
	CallCount int64
	TSum      int64
	TMin      int64
	TMax      int64
}

// ParseCaller TODO
func ParseCaller(funcName string) (string, string) {
	lastSlash := 0
	for i := 0; i < len(funcName); i++ {
		if funcName[i] == '/' {
			lastSlash = i
		}
	}
	firstDot := lastSlash
	for i := lastSlash; i < len(funcName); i++ {
		if funcName[i] == '.' {
			firstDot = i
			break
		}
	}
	return funcName[:firstDot], funcName[firstDot+1:]
}

func caller(skip int) (string, string, int) {
	pc, fileName, line, ok := runtime.Caller(skip)
	if !ok {
		return "nil", "nil", 0
	}
	fileName = filepath.ToSlash(fileName)
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "nil", "nil", 0
	}
	return fn.Name(), fileName, line
}
