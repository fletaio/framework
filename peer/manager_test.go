package peer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.fleta.io/fleta/network/simulations"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/framework/chain/mesh"
	"git.fleta.io/fleta/framework/log"
	"git.fleta.io/fleta/framework/message"
	"git.fleta.io/fleta/framework/peer/peermessage"
	"git.fleta.io/fleta/framework/router"
	"git.fleta.io/fleta/framework/router/evilnode"
)

var (
	testID   int32
	testPort = 3000
	testLock sync.Mutex
)

type testMessage struct {
	BaseEventHandler
	ID string
	peermessage.PeerList
	pm       *manager
	onRecv   func(p mesh.Peer, msg message.Type, r io.Reader) error
	onClosed func(p mesh.Peer)
}

var testMessageType message.Type

func init() {
	testMessageType = message.DefineType("testMessage")
	os.RemoveAll("./test/")
}

func (tm *testMessage) Type() message.Type {
	return testMessageType
}
func (tm *testMessage) OnRecv(p mesh.Peer, t message.Type, r io.Reader) error {
	return tm.onRecv(p, t, r)
}
func (tm *testMessage) OnClosed(p mesh.Peer) {
	if tm.onClosed != nil {
		tm.onClosed(p)
	}
}

func upVisulaization(tms []*testMessage) {
	mc := make(chan simulations.Msg)
	go func() {
		for {
			for _, tm := range tms {
				simulations.AddVisualizationData(tm.ID, "id", func(tm *testMessage) func() []string {
					return func() []string { return []string{tm.ID} }
				}(tm))
				simulations.AddVisualizationData(tm.ID, "peer", tm.pm.NodeList)
				simulations.AddVisualizationData(tm.ID, "group", tm.pm.GroupList)
			}
			time.Sleep(time.Second)
		}
	}()
	go simulations.VisualizationStart(mc, 8080)

}

func Test_manager_BroadCast(t *testing.T) {
	testLock.Lock()
	defer testLock.Unlock()
	ID := int(atomic.AddInt32(&testID, 1))
	size := 20
	path := "./test/Test_manager_BroadCast" + strconv.Itoa(ID)
	port := testPort + ID

	type args struct {
		ChainCoord          *common.Coordinate
		DefaultRouterConfig *router.Config
		DefaultConfig       *Config
		IDs                 []int
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr error
	}{
		{
			name: "string",
			args: args{
				ChainCoord: &common.Coordinate{},
				DefaultRouterConfig: &router.Config{
					Network: "mock:",
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router",
						BanEvilScore: 100,
					},
				},
				DefaultConfig: &Config{
					StorePath: path + "/peer",
				},
				IDs: func() []int {
					IDs := make([]int, 0, size)
					for i := 0; i < size; i++ {
						IDs = append(IDs, int(atomic.AddInt32(&testID, 1)))
					}
					return IDs
				}(),
			},
			want:    true,
			wantErr: router.ErrCanNotConnectToEvilNode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wg := sync.WaitGroup{}
			wg.Add(size)

			creator := func(id int) *testMessage {
				rc := &router.Config{
					Network: tt.args.DefaultRouterConfig.Network + "testid" + strconv.Itoa(id),
					Port:    tt.args.DefaultRouterConfig.Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    tt.args.DefaultRouterConfig.EvilNodeConfig.StorePath + strconv.Itoa(id) + "/",
						BanEvilScore: tt.args.DefaultRouterConfig.EvilNodeConfig.BanEvilScore,
					},
				}
				pc := &Config{
					StorePath: tt.args.DefaultConfig.StorePath + strconv.Itoa(id) + "/",
				}
				r, _ := router.NewRouter(rc)
				pm, _ := NewManager(tt.args.ChainCoord, r, pc)
				mm := message.NewManager()

				tm := &testMessage{
					pm: pm,
					ID: fmt.Sprintf("%v", id),
				}
				func(tm *testMessage) {
					tm.onClosed = func(p mesh.Peer) {
						// log.Notice("tm.onClosed ", tm.ID, p.ID())
					}
					tm.onRecv = func(p mesh.Peer, msg message.Type, r io.Reader) error {
						m, err := mm.ParseMessage(r, msg)
						if err != nil {
							return err
						}
						if t, ok := m.(*testMessage); ok {
							if len(tm.List) == 0 {
								wg.Done()
								log.Info(tm.ID, "Done")

								tm.From = t.From
								tm.List[strconv.Itoa(len(tm.List))] = peermessage.ConnectInfo{
									Address: t.From,
								}
								tm.pm.BroadCast(tm)
							}

							return nil
						}
						return errors.New("is not test message")
					}
					pm.RegisterEventHandler(tm)

					tm.List = map[string]peermessage.ConnectInfo{}
					mm.SetCreator(testMessageType, func(r io.Reader, mt message.Type) (message.Message, error) {
						tm := &testMessage{}
						tm.ReadFrom(r)
						return tm, nil
					})
				}(tm)

				return tm
			}

			tms := make([]*testMessage, 0, size)
			for _, id := range tt.args.IDs {
				tm := creator(id)
				tm.pm.StartManage()
				tm.pm.AddNode("testid" + strconv.Itoa(tt.args.IDs[0]))
				tms = append(tms, tm)
			}

			upVisulaization(tms)

			for len(tms[len(tms)-1].pm.GroupList()) < 6 {
				log.Info(len(tms[len(tms)-1].pm.GroupList()))

				var l string
				for i, t := range tms {
					if len(t.pm.Peers()) > 0 {
						l += fmt.Sprintf("%v:%v(%v),", i, len(t.pm.Peers()), tt.args.IDs[i])
					} else {
						l += fmt.Sprintf("%v:%v,", i, tt.args.IDs[i])
					}
				}
				log.Notice(l)
				time.Sleep(time.Second)
			}

			log.Info("BroadCast init done")

			tms[len(tms)-1].From = "send broadCast"
			tms[len(tms)-1].pm.BroadCast(tms[len(tms)-1])

			wg.Wait()

			count := 0
			for _, tm := range tms {
				for key, ci := range tm.List {
					log.Info("for key ", key, ", ", ci.Address)
					str := ci.Address
					if str == "send broadCast" {
						count++
					}
				}
			}

			if count < size {
				t.Errorf("received count %v", count)
				return
			}

		})
	}
}

func Test_manager_ExceptCast(t *testing.T) {
	testLock.Lock()
	defer testLock.Unlock()
	ID := int(atomic.AddInt32(&testID, 1))
	size := 20
	path := "./test/Test_manager_ExceptCast" + strconv.Itoa(ID)
	port := testPort + ID

	type args struct {
		ChainCoord          *common.Coordinate
		DefaultRouterConfig *router.Config
		DefaultConfig       *Config
		IDs                 []int
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr error
	}{
		{
			name: "string",
			args: args{
				ChainCoord: &common.Coordinate{},
				DefaultRouterConfig: &router.Config{
					Network: "mock:",
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router",
						BanEvilScore: 100,
					},
				},
				DefaultConfig: &Config{
					StorePath: path + "/peer",
				},
				IDs: func() []int {
					IDs := make([]int, 0, size)
					for i := 0; i < size; i++ {
						IDs = append(IDs, int(atomic.AddInt32(&testID, 1)))
					}
					return IDs
				}(),
			},
			want:    true,
			wantErr: router.ErrCanNotConnectToEvilNode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			wg := sync.WaitGroup{}
			wg.Add(size - 1)

			creator := func(id int) *testMessage {
				rc := &router.Config{
					Network: tt.args.DefaultRouterConfig.Network + "testid" + strconv.Itoa(id),
					Port:    tt.args.DefaultRouterConfig.Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    tt.args.DefaultRouterConfig.EvilNodeConfig.StorePath + strconv.Itoa(id) + "/",
						BanEvilScore: tt.args.DefaultRouterConfig.EvilNodeConfig.BanEvilScore,
					},
				}
				pc := &Config{
					StorePath: tt.args.DefaultConfig.StorePath + strconv.Itoa(id) + "/",
				}
				r, _ := router.NewRouter(rc)
				pm, _ := NewManager(tt.args.ChainCoord, r, pc)
				mm := message.NewManager()

				tm := &testMessage{
					pm: pm,
				}
				func(tm *testMessage) {
					tm.onRecv = func(p mesh.Peer, msg message.Type, r io.Reader) error {
						m, err := mm.ParseMessage(r, msg)
						if err != nil {
							return err
						}
						if t, ok := m.(*testMessage); ok {
							if len(tm.List) == 0 {
								wg.Done()
								tm.List[strconv.Itoa(len(tm.List))] = peermessage.ConnectInfo{
									Address: t.From,
								}
								tm.From = t.From
								tm.pm.ExceptCast(tm.From, tm)
							}
							return nil
						}
						return errors.New("is not test message")
					}
					pm.RegisterEventHandler(tm)

					tm.List = map[string]peermessage.ConnectInfo{}
					mm.SetCreator(testMessageType, func(r io.Reader, mt message.Type) (message.Message, error) {
						tm := &testMessage{}
						tm.ReadFrom(r)
						return tm, nil
					})

				}(tm)

				return tm
			}

			tms := make([]*testMessage, 0, size)
			for _, id := range tt.args.IDs {
				tm := creator(id)
				tm.pm.StartManage()
				tm.pm.AddNode("testid" + strconv.Itoa(tt.args.IDs[0]))
				tms = append(tms, tm)
			}

			for len(tms[len(tms)-1].pm.GroupList()) < 6 {
				log.Info(len(tms[len(tms)-1].pm.GroupList()))
				time.Sleep(time.Second)
			}

			log.Info("ExceptCast init done")
			exceptNode := tms[1].pm.router.Localhost()
			log.Info("start except cast except node : ", exceptNode)

			tms[len(tms)-1].From = exceptNode
			tms[len(tms)-1].pm.ExceptCast(exceptNode, tms[len(tms)-1])

			count := 0
			wg.Wait()

			for _, tm := range tms {
				for key, ci := range tm.List {
					log.Info("for key ", key, ", ", ci.Address)
					str := ci.Address
					if str == exceptNode {
						count++
					}
				}
			}

			if count != size-1 {
				t.Errorf("received count not match expect %v real %v", size-1, count)
				return
			}

			if tms[1].From != "" && exceptNode != tms[len(tms)-1].From {
				t.Errorf("except target %v but received data %v", exceptNode, tms[1].From)
				return
			}

		})
	}
}

func Test_target_cast(t *testing.T) {
	testLock.Lock()
	defer testLock.Unlock()
	ID := int(atomic.AddInt32(&testID, 1))
	size := 5
	path := "./test/Test_target_cast" + strconv.Itoa(ID)
	port := testPort + ID

	type args struct {
		ChainCoord          *common.Coordinate
		DefaultRouterConfig *router.Config
		DefaultConfig       *Config
		IDs                 []int
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr error
	}{
		{
			name: "string",
			args: args{
				ChainCoord: &common.Coordinate{},
				DefaultRouterConfig: &router.Config{
					Network: "mock:",
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router",
						BanEvilScore: 1000,
					},
				},
				DefaultConfig: &Config{
					StorePath: path + "/peer",
				},
				IDs: func() []int {
					IDs := make([]int, 0, size)
					for i := 0; i < size; i++ {
						IDs = append(IDs, int(atomic.AddInt32(&testID, 1)))
					}
					return IDs
				}(),
			},
			want:    true,
			wantErr: router.ErrCanNotConnectToEvilNode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			wg := sync.WaitGroup{}
			wg.Add(size - 1)

			creator := func(id int) *testMessage {
				rc := &router.Config{
					Network: tt.args.DefaultRouterConfig.Network + "testid" + strconv.Itoa(id),
					Port:    tt.args.DefaultRouterConfig.Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    tt.args.DefaultRouterConfig.EvilNodeConfig.StorePath + strconv.Itoa(id) + "/",
						BanEvilScore: tt.args.DefaultRouterConfig.EvilNodeConfig.BanEvilScore,
					},
				}
				pc := &Config{
					StorePath: tt.args.DefaultConfig.StorePath + strconv.Itoa(id) + "/",
				}
				r, _ := router.NewRouter(rc)
				pm, _ := NewManager(tt.args.ChainCoord, r, pc)
				mm := message.NewManager()

				tm := &testMessage{
					pm: pm,
				}
				func(tm *testMessage) {
					tm.onRecv = func(p mesh.Peer, msg message.Type, r io.Reader) error {
						m, err := mm.ParseMessage(r, msg)
						if err != nil {
							return err
						}
						if t, ok := m.(*testMessage); ok {
							tm.From = t.From
							wg.Done()
							return nil
						}
						return errors.New("is not test message")
					}
					pm.RegisterEventHandler(tm)

					tm.List = map[string]peermessage.ConnectInfo{}
					mm.SetCreator(testMessageType, func(r io.Reader, mt message.Type) (message.Message, error) {
						tm := &testMessage{}
						tm.ReadFrom(r)
						return tm, nil
					})
				}(tm)

				return tm
			}

			tms := make([]*testMessage, 0, size)
			for _, id := range tt.args.IDs {
				tm := creator(id)
				tm.pm.StartManage()
				tm.pm.AddNode("testid" + strconv.Itoa(tt.args.IDs[0]))
				tms = append(tms, tm)
			}

			for len(tms[0].pm.ConnectedList()) < 4 {
				time.Sleep(time.Second)
			}

			log.Info("targetCast init done")
			for i, tm := range tms {
				if i == 0 {
					continue
				}
				targetNode := tm.pm.router.Localhost()
				tms[0].From = "to " + targetNode
				tms[0].pm.TargetCast(targetNode, tms[0])
				log.Info("send target node : ", targetNode)
			}

			wg.Wait()

			for i, tm := range tms {
				if i == 0 {
					continue
				}

				expectMsg := "to " + tm.pm.router.Localhost()
				if expectMsg != tm.From {
					t.Errorf("expectMsg %v but data %v", expectMsg, tm.From)
					return
				}
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	testLock.Lock()
	defer testLock.Unlock()
	ID := int(atomic.AddInt32(&testID, 1))
	addr := "test" + strconv.Itoa(ID)
	path := "./test/TestNewManager" + strconv.Itoa(ID)
	port := testPort + ID

	type args struct {
		ChainCoord   *common.Coordinate
		routerConfig *router.Config
		Config       *Config
		mm           *message.Manager
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "string",
			args: args{
				ChainCoord: &common.Coordinate{},
				routerConfig: &router.Config{
					Network: "mock:" + addr,
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router/",
						BanEvilScore: 100,
					},
				},
				Config: &Config{
					StorePath: path + "/peer/",
				},
				mm: message.NewManager(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := router.NewRouter(tt.args.routerConfig)
			_, err := NewManager(tt.args.ChainCoord, r, tt.args.Config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestAddNode(t *testing.T) {
	testLock.Lock()
	defer testLock.Unlock()
	ID1 := int(atomic.AddInt32(&testID, 1))
	ID2 := int(atomic.AddInt32(&testID, 1))
	addr1 := "test" + strconv.Itoa(ID1)
	addr2 := "test" + strconv.Itoa(ID2)
	path := "./test/TestAddNode" + strconv.Itoa(ID1)
	port := testPort + ID1

	type args struct {
		ChainCoord    *common.Coordinate
		routerConfig1 *router.Config
		routerConfig2 *router.Config
		Config1       *Config
		Config2       *Config
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "string",
			args: args{
				ChainCoord: &common.Coordinate{},
				routerConfig1: &router.Config{
					Network: "mock:" + addr1,
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router1/",
						BanEvilScore: 100,
					},
				},
				Config1: &Config{
					StorePath: path + "/peer1/",
				},
				routerConfig2: &router.Config{
					Network: "mock:" + addr2,
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router2/",
						BanEvilScore: 100,
					},
				},
				Config2: &Config{
					StorePath: path + "/peer2/",
				},
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			addr1 = addr1 + ":" + strconv.Itoa(port)
			addr2 = addr2 + ":" + strconv.Itoa(port)
			r1, _ := router.NewRouter(tt.args.routerConfig1)
			pm1, _ := NewManager(tt.args.ChainCoord, r1, tt.args.Config1)

			r2, _ := router.NewRouter(tt.args.routerConfig2)
			pm2, _ := NewManager(tt.args.ChainCoord, r2, tt.args.Config2)

			err := pm2.AddNode(addr1)

			pm1.StartManage()
			pm2.StartManage()

			pm1.EnforceConnect()
			pm2.EnforceConnect()

			{
				for {
					for _, addr := range pm1.ConnectedList() {
						if addr == addr2 {
							goto EndFor
						}
					}
					time.Sleep(time.Second)
				}
			EndFor:
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestBanEvil(t *testing.T) {
	testLock.Lock()
	defer testLock.Unlock()
	ID := int(atomic.AddInt32(&testID, 1))
	addr := "test" + strconv.Itoa(ID)
	path := "./test/TestBanEvil" + strconv.Itoa(ID)
	port := testPort + ID

	type args struct {
		ChainCoord    *common.Coordinate
		routerConfig1 *router.Config
		Config1       *Config
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr error
	}{
		{
			name: "string",
			args: args{
				ChainCoord: &common.Coordinate{},
				routerConfig1: &router.Config{
					Network: "mock:" + addr,
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router1/",
						BanEvilScore: 30,
					},
				},
				Config1: &Config{
					StorePath: path + "/peer1/",
				},
			},
			want:    true,
			wantErr: router.ErrCanNotConnectToEvilNode,
		},
	}
	for _, tt := range tests {
		tempAddr := "temp:" + strconv.Itoa(port)
		t.Run(tt.name, func(t *testing.T) {
			r1, _ := router.NewRouter(tt.args.routerConfig1)
			pm, _ := NewManager(tt.args.ChainCoord, r1, tt.args.Config1)
			pm.AddNode(tempAddr)
			pm.StartManage()
			pm.doManageCandidate(tempAddr, csPunishableRequestWait)

			err := pm.AddNode(tempAddr)
			if err != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestPeerListSpread(t *testing.T) {
	testLock.Lock()
	defer testLock.Unlock()
	ID := int(atomic.AddInt32(&testID, 1))
	size := 20
	path := "./test/TestPeerListSpread" + strconv.Itoa(ID)
	port := testPort + ID

	type args struct {
		ChainCoord          *common.Coordinate
		DefaultRouterConfig *router.Config
		DefaultConfig       *Config
		IDs                 []int
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr error
	}{
		{
			name: "string",
			args: args{
				ChainCoord: &common.Coordinate{},
				DefaultRouterConfig: &router.Config{
					Network: "mock:",
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router",
						BanEvilScore: 100,
					},
				},
				DefaultConfig: &Config{
					StorePath: path + "/peer",
				},
				IDs: func() []int {
					IDs := make([]int, 0, size)
					for i := 0; i < size; i++ {
						IDs = append(IDs, int(atomic.AddInt32(&testID, 1)))
					}
					return IDs
				}(),
			},
			want:    true,
			wantErr: router.ErrCanNotConnectToEvilNode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			creator := func(id int) Manager {
				rc := &router.Config{
					Network: tt.args.DefaultRouterConfig.Network + "testid" + strconv.Itoa(id),
					Port:    tt.args.DefaultRouterConfig.Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    tt.args.DefaultRouterConfig.EvilNodeConfig.StorePath + strconv.Itoa(id) + "/",
						BanEvilScore: tt.args.DefaultRouterConfig.EvilNodeConfig.BanEvilScore,
					},
				}
				pc := &Config{
					StorePath: tt.args.DefaultConfig.StorePath + strconv.Itoa(id) + "/",
				}
				r, _ := router.NewRouter(rc)

				pm, _ := NewManager(tt.args.ChainCoord, r, pc)

				pm.RegisterEventHandler(&BaseEventHandler{})

				return pm
			}

			pms := make([]*manager, 0, size)
			for _, id := range tt.args.IDs {
				pm := creator(id)
				pm.StartManage()
				pm.AddNode("testid" + strconv.Itoa(tt.args.IDs[0]))
				Pm := pm.(*manager)
				pms = append(pms, Pm)
			}

			log.Info("wait NodeList fill")

			for len(pms[len(pms)-1].NodeList()) < size-1 {
				time.Sleep(time.Second)
				for _, pm := range pms {
					pm.candidates.rangeMap(func(addr string, cs candidateState) bool {
						pm.doManageCandidate(addr, cs)
						time.Sleep(time.Millisecond * 50)
						return true
					})
				}

			}

			log.Info("NodeList fill done")

			for len(pms[len(pms)-1].GroupList()) < 2 {
				time.Sleep(time.Second)
			}

			log.Info("GroupList fill done")

		})
	}
}

func Test_manager_EnforceConnect(t *testing.T) {
	testLock.Lock()
	defer testLock.Unlock()
	ID1 := int(atomic.AddInt32(&testID, 1))
	ID2 := int(atomic.AddInt32(&testID, 1))
	addr1 := "test" + strconv.Itoa(ID1)
	addr2 := "test" + strconv.Itoa(ID2)
	path := "./test/Test_manager_EnforceConnect" + strconv.Itoa(ID1)
	port := testPort + ID1

	type args struct {
		ChainCoord    *common.Coordinate
		routerConfig1 *router.Config
		routerConfig2 *router.Config
		Config1       *Config
		Config2       *Config
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "string",
			args: args{
				ChainCoord: &common.Coordinate{},
				routerConfig1: &router.Config{
					Network: "mock:" + addr1,
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router1/",
						BanEvilScore: 100,
					},
				},
				Config1: &Config{
					StorePath: path + "/peer1/",
				},
				routerConfig2: &router.Config{
					Network: "mock:" + addr2,
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router2/",
						BanEvilScore: 100,
					},
				},
				Config2: &Config{
					StorePath: path + "/peer2/",
				},
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			addr1 = addr1 + ":" + strconv.Itoa(port)
			addr2 = addr2 + ":" + strconv.Itoa(port)
			r1, _ := router.NewRouter(tt.args.routerConfig1)
			pm1, _ := NewManager(tt.args.ChainCoord, r1, tt.args.Config1)

			r2, _ := router.NewRouter(tt.args.routerConfig2)
			pm2, _ := NewManager(tt.args.ChainCoord, r2, tt.args.Config2)

			err := pm2.AddNode(addr1)

			time.Sleep(time.Second * 6)

			pm1.StartManage()
			pm2.StartManage()

			pm2.EnforceConnect()

			{
				for {
					for _, addr := range pm1.ConnectedList() {
						if addr == addr2 {
							goto EndFor
						}
					}
					time.Sleep(time.Second)
				}
			EndFor:
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_multi_chain_send(t *testing.T) {
	testLock.Lock()
	defer testLock.Unlock()
	ID1 := int(atomic.AddInt32(&testID, 1))
	ID2 := int(atomic.AddInt32(&testID, 1))
	addr1 := "test" + strconv.Itoa(ID1)
	addr2 := "test" + strconv.Itoa(ID2)
	path := "./test/Test_multi_chain_send" + strconv.Itoa(ID1)
	port := testPort + ID1

	type args struct {
		ChainCoord1   *common.Coordinate
		ChainCoord2   *common.Coordinate
		routerConfig1 *router.Config
		routerConfig2 *router.Config
		Config1_1     *Config
		Config1_2     *Config
		Config2_1     *Config
		Config2_2     *Config
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "string",
			args: args{
				ChainCoord1: common.NewCoordinate(0, 1),
				ChainCoord2: common.NewCoordinate(0, 2),
				routerConfig1: &router.Config{
					Network: "mock:" + addr1,
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router1/",
						BanEvilScore: 100,
					},
				},
				Config1_1: &Config{
					StorePath: path + "/peer1_1/",
				},
				Config1_2: &Config{
					StorePath: path + "/peer1_2/",
				},
				routerConfig2: &router.Config{
					Network: "mock:" + addr2,
					Port:    port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    path + "/router2/",
						BanEvilScore: 100,
					},
				},
				Config2_1: &Config{
					StorePath: path + "/peer2_1/",
				},
				Config2_2: &Config{
					StorePath: path + "/peer2_2/",
				},
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			addr1 = addr1 + ":" + strconv.Itoa(port)
			addr2 = addr2 + ":" + strconv.Itoa(port)

			wg := sync.WaitGroup{}
			wg.Add(4)

			creator := func(r router.Router, ChainCoord *common.Coordinate, config *Config) (*testMessage, error) {
				pm, _ := NewManager(ChainCoord, r, config)
				mm := message.NewManager()

				tm := &testMessage{
					pm: pm,
				}
				func(tm *testMessage) {
					tm.onRecv = func(p mesh.Peer, msg message.Type, r io.Reader) error {
						m, err := mm.ParseMessage(r, msg)
						if err != nil {
							return err
						}
						if t, ok := m.(*testMessage); ok {
							if len(tm.List) == 0 {
								tm.From = t.From
								tm.List[strconv.Itoa(len(tm.List))] = peermessage.ConnectInfo{
									Address: t.From,
								}
								tm.pm.BroadCast(tm)
								wg.Done()
							}

							return nil
						}
						return errors.New("is not test message")
					}
					pm.RegisterEventHandler(tm)

					tm.List = map[string]peermessage.ConnectInfo{}
					mm.SetCreator(testMessageType, func(r io.Reader, mt message.Type) (message.Message, error) {
						tm := &testMessage{}
						tm.ReadFrom(r)
						return tm, nil
					})
				}(tm)

				pm.AddNode(addr1)
				pm.StartManage()

				return tm, nil
			}

			r1, err := router.NewRouter(tt.args.routerConfig1)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tmR1C1, err := creator(r1, tt.args.ChainCoord1, tt.args.Config1_1)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			tmR1C2, err := creator(r1, tt.args.ChainCoord2, tt.args.Config1_2)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			r2, err := router.NewRouter(tt.args.routerConfig2)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tmR2C1, err := creator(r2, tt.args.ChainCoord1, tt.args.Config2_1)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tmR2C2, err := creator(r2, tt.args.ChainCoord2, tt.args.Config2_2)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			time.Sleep(time.Second)

			tmR2C1.From = "ChainCoord1 send"
			tmR2C1.pm.BroadCast(tmR2C1)

			tmR2C2.From = "ChainCoord2 send"
			tmR2C2.pm.BroadCast(tmR2C2)

			wg.Wait()

			if tmR1C1.From != tmR2C1.From {
				t.Errorf("except tmR1C1.From is equal with tmR2C1.From but tmR1C1.From is '%v' and tmR2C1.From is '%v'", tmR1C2.From, tmR2C2.From)
				return
			}
			if tmR1C2.From != tmR2C2.From {
				t.Errorf("except tmR1C2.From is equal with tmR2C2.From but tmR1C2.From is '%v' and tmR2C2.From is '%v'", tmR1C2.From, tmR2C2.From)
				return
			}

		})
	}
}

func TestNewByTime(t *testing.T) {
	type args struct {
		list    []int64
		timeout time.Duration
	}
	tests := []struct {
		name string
		args args
		want []bool
	}{
		{name: "test1", args: args{list: []int64{1, 45, 2, 789, 3, 6, 65}, timeout: time.Second * 3}, want: []bool{false, true, false, true, false, true, true}},
		{name: "test2", args: args{list: []int64{1}, timeout: time.Second * 0}, want: []bool{true}},
		{name: "test2", args: args{list: []int64{1, 2}, timeout: time.Second * 1}, want: []bool{false, true}},
		{
			name: "test3",
			args: args{
				list:    []int64{1, 45, 1, 789, 1, 3, 65, 3, 3, 3, 3, 3, 4},
				timeout: (time.Second * 3) + (time.Millisecond * 500),
			},
			want: []bool{false, true, false, true, false, false, true, false, false, false, false, false, true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewByTime()
			for i, v := range tt.args.list {
				b.Add(fmt.Sprintf("%v", i), v)
			}
			time.Sleep(tt.args.timeout)
			for i, w := range tt.want {
				key := fmt.Sprintf("%v", i)
				if got := b.IsBan(key); w != got {
					t.Errorf("i = %v isBan = %v, overTime = %v want %v", key, got, b.Map[key].OverTime, w)
				}
			}

		})
	}
}
