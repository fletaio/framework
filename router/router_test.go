package router

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"git.fleta.io/fleta/framework/router/evilnode"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/framework/log"
)

var _port = 3000

func Test_removePort(t *testing.T) {
	type args struct {
		addr string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr error
	}{
		{
			name: "string",
			args: args{
				addr: "test:test",
			},
			want:    "test:test",
			wantErr: ErrNotFoundPort,
		},
		{
			name: "num",
			args: args{
				addr: "test:123",
			},
			want:    "test",
			wantErr: nil,
		},
		{
			name: "notincludeport",
			args: args{
				addr: "test",
			},
			want:    "test",
			wantErr: ErrNotFoundPort,
		},
		{
			name: "multicolon",
			args: args{
				addr: "[test:test:test:test]:123",
			},
			want:    "[test:test:test:test]",
			wantErr: nil,
		},
		{
			name: "multicolonNotNum",
			args: args{
				addr: "[test:test:test:test]:test",
			},
			want:    "[test:test:test:test]:test",
			wantErr: ErrNotFoundPort,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := removePort(tt.args.addr)
			if got != tt.want {
				t.Errorf("removePort() = %v, want %v", got, tt.want)
			}
			if err != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_router_Connecte(t *testing.T) {
	var Port = _port
	_port++
	type args struct {
		ChainCoord *common.Coordinate
		Config1    *Config
		Config2    *Config
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
				Config1: &Config{
					Network: "mock:test1",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/debug1/",
						BanEvilScore: 100,
					},
				},
				Config2: &Config{
					Network: "mock:test2",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/debug2/",
						BanEvilScore: 100,
					},
				},
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r1, _ := NewRouter(tt.args.Config1)
			r2, _ := NewRouter(tt.args.Config2)

			r1.AddListen(tt.args.ChainCoord)
			r2.AddListen(tt.args.ChainCoord)

			r2.Request(fmt.Sprintf("test1:%v", Port), tt.args.ChainCoord)

			wg := sync.WaitGroup{}
			wg.Add(2)

			var ping1 time.Duration
			var ping2 time.Duration
			go func() {
				_, ping, _ := r1.Accept(tt.args.ChainCoord)
				ping1 = ping
				log.Info("ping1 ", ping1)
				wg.Done()
			}()
			go func() {
				_, ping, _ := r2.Accept(tt.args.ChainCoord)
				ping2 = ping
				log.Info("ping2 ", ping2)
				wg.Done()
			}()
			wg.Wait()

			if ((ping1 > 0) == (ping2 > 0)) != tt.want {
				t.Errorf("ping1 = %v, ping2 = %v, want %v", ping1, ping2, tt.want)
			}
		})
	}
}

func Test_router_Connecte_send(t *testing.T) {
	var Port = _port
	_port++
	type args struct {
		ChainCoord *common.Coordinate
		Config1    *Config
		Config2    *Config
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
				Config1: &Config{
					Network: "mock:send1",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/send1/",
						BanEvilScore: 100,
					},
				},
				Config2: &Config{
					Network: "mock:send2",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/send2/",
						BanEvilScore: 100,
					},
				},
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r1, _ := NewRouter(tt.args.Config1)
			r2, _ := NewRouter(tt.args.Config2)

			r1.AddListen(tt.args.ChainCoord)
			r2.AddListen(tt.args.ChainCoord)

			r2.Request("send1:3002", tt.args.ChainCoord)

			wg := sync.WaitGroup{}
			wg.Add(2)

			var readConn net.Conn
			var writeConn net.Conn
			go func() {
				conn, _, _ := r1.Accept(tt.args.ChainCoord)
				readConn = conn
				wg.Done()
			}()
			go func() {
				conn, _, _ := r2.Accept(tt.args.ChainCoord)
				writeConn = conn
				wg.Done()
			}()
			wg.Wait()

			strChan := make(chan string)
			go func() {
				bs := make([]byte, 1024)
				n, _ := readConn.Read(bs)
				strChan <- string(bs[:n])
			}()

			go func() {
				writeConn.Write([]byte("sendTest"))
			}()

			result := <-strChan

			if (result == "sendTest") != tt.want {
				t.Errorf("result = %v, want %v", result, tt.want)
			}
		})
	}
}

func Test_router_MultyCoord_Connecte_send(t *testing.T) {
	var Port = _port
	_port++
	type args struct {
		ChainCoord1 *common.Coordinate
		ChainCoord2 *common.Coordinate
		Config1     *Config
		Config2     *Config
		Config3     *Config
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
				ChainCoord1: common.NewCoordinate(0, 0),
				ChainCoord2: common.NewCoordinate(0, 1),
				Config1: &Config{
					Network: "mock:send1",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/send1/",
						BanEvilScore: 100,
					},
				},
				Config2: &Config{
					Network: "mock:send2",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/send2/",
						BanEvilScore: 100,
					},
				},
				Config3: &Config{
					Network: "mock:send3",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/send3/",
						BanEvilScore: 100,
					},
				},
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r1, _ := NewRouter(tt.args.Config1)
			r2, _ := NewRouter(tt.args.Config2)
			r3, _ := NewRouter(tt.args.Config3)

			r1.AddListen(tt.args.ChainCoord1)
			r1.AddListen(tt.args.ChainCoord2)
			r2.AddListen(tt.args.ChainCoord1)
			r3.AddListen(tt.args.ChainCoord2)

			r2.Request(fmt.Sprintf("send1:%v", Port), tt.args.ChainCoord1)
			r3.Request(fmt.Sprintf("send1:%v", Port), tt.args.ChainCoord2)

			{
				wg := sync.WaitGroup{}
				wg.Add(2)

				var readConn net.Conn
				var writeConn net.Conn
				go func() {
					conn, _, _ := r1.Accept(tt.args.ChainCoord1)
					readConn = conn
					wg.Done()
				}()
				go func() {
					conn, _, _ := r2.Accept(tt.args.ChainCoord1)
					writeConn = conn
					wg.Done()
				}()
				wg.Wait()

				strChan := make(chan string)
				go func() {
					bs := make([]byte, 1024)
					n, _ := readConn.Read(bs)
					strChan <- string(bs[:n])
				}()

				go func() {
					writeConn.Write([]byte("sendTest"))
				}()

				result := <-strChan

				if (result == "sendTest") != tt.want {
					t.Errorf("result = %v, want %v", result, tt.want)
				}
			}

			{
				wg := sync.WaitGroup{}
				wg.Add(2)

				var readConn net.Conn
				var writeConn net.Conn
				go func() {
					conn, _, _ := r1.Accept(tt.args.ChainCoord2)
					readConn = conn
					wg.Done()
				}()
				go func() {
					conn, _, _ := r3.Accept(tt.args.ChainCoord2)
					writeConn = conn
					wg.Done()
				}()
				wg.Wait()

				strChan := make(chan string)
				go func() {
					bs := make([]byte, 1024)
					n, _ := readConn.Read(bs)
					strChan <- string(bs[:n])
				}()

				go func() {
					writeConn.Write([]byte("sendTest"))
				}()

				result := <-strChan

				if (result == "sendTest") != tt.want {
					t.Errorf("result = %v, want %v", result, tt.want)
				}
			}
		})
	}
}

func Test_router_Connecte_request_to_local(t *testing.T) {
	var Port = _port
	_port++
	type args struct {
		ChainCoord *common.Coordinate
		Config1    *Config
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "requesttolocal",
			args: args{
				ChainCoord: &common.Coordinate{},
				Config1: &Config{
					Network: "mock:requesttolocal",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/debug3/",
						BanEvilScore: 100,
					},
				},
			},
			wantErr: ErrCannotRequestToLocal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r1, _ := NewRouter(tt.args.Config1)

			r1.AddListen(tt.args.ChainCoord)

			err := r1.Request(fmt.Sprintf("requesttolocal:%v", Port), tt.args.ChainCoord)

			if err != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_router_Time_store(t *testing.T) {
	type args struct {
		period time.Duration
		size   int64
		sleep  time.Duration
		key    string
		value  string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "inTime",
			args: args{
				period: time.Second,
				size:   3,
				sleep:  0,
				key:    "key",
				value:  "value",
			},
			want: true,
		},
		{
			name: "timeout",
			args: args{
				period: time.Second,
				size:   3,
				sleep:  time.Second * 4,
				key:    "key",
				value:  "value",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := true

			m := NewTimerMap(tt.args.period, tt.args.size)

			m.Store(tt.args.key, tt.args.value)
			time.Sleep(tt.args.sleep)
			k, _ := m.Load(tt.args.key)

			if (k == tt.args.value) != tt.want {
				t.Errorf("result = %v, want %v", result, tt.want)
			}
		})
	}
}

func Test_EvilScore(t *testing.T) {
	var Port = _port
	_port++
	type args struct {
		ChainCoord *common.Coordinate
		Config     *Config
	}
	tests := []struct {
		name string
		args args
		want uint16
	}{
		{
			name: "string",
			args: args{
				ChainCoord: &common.Coordinate{},
				Config: &Config{
					Network: "mock:evilscore",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/evilscore/",
						BanEvilScore: 100,
					},
				},
			},
			want: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.RemoveAll(tt.args.Config.EvilNodeConfig.StorePath)

			targetAddr := "testaddr:33000"
			em := evilnode.NewManager(&tt.args.Config.EvilNodeConfig)
			if em.IsBanNode(targetAddr) {
				t.Errorf("not expect ban first")
				return
			}

			em.TellOn(targetAddr, evilnode.KindOfEvil(1000))

			value := !em.IsBanNode(targetAddr)
			if value {
				t.Errorf("expect ban value true but %v", value)
				return
			}
		})
	}
}

func Test_router_UpdateEvilScore(t *testing.T) {
	var Port = _port
	_port++
	type args struct {
		ChainCoord *common.Coordinate
		Config1    *Config
		Config2    *Config
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr error
	}{
		{
			name: "string",
			args: args{
				ChainCoord: &common.Coordinate{},
				Config1: &Config{
					Network: "mock:evilscore1",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/evilscore1/",
						BanEvilScore: 100,
					},
				},
				Config2: &Config{
					Network: "mock:evilscore2",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/evilscore2/",
						BanEvilScore: 100,
					},
				},
			},
			want:    "NotConnected",
			wantErr: ErrCanNotConnectToEvilNode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.RemoveAll(tt.args.Config1.EvilNodeConfig.StorePath)
			os.RemoveAll(tt.args.Config2.EvilNodeConfig.StorePath)
			r1, _ := NewRouter(tt.args.Config1)
			r2, _ := NewRouter(tt.args.Config2)

			r1.AddListen(tt.args.ChainCoord)
			r2.AddListen(tt.args.ChainCoord)

			wg := sync.WaitGroup{}
			wg.Add(2)
			r2.Request(fmt.Sprintf("evilscore1:%v", Port), tt.args.ChainCoord)

			var readConn net.Conn
			go func() {
				conn, _, _ := r1.Accept(tt.args.ChainCoord)
				readConn = conn
				wg.Done()
			}()
			go func() {
				r2.Accept(tt.args.ChainCoord)
				wg.Done()
			}()

			wg.Wait()

			r1.EvilNodeManager().TellOn(readConn.RemoteAddr().String(), evilnode.KindOfEvil(200))
			readConn.Close()

			time.Sleep(time.Second * 3)

			err := r1.Request(fmt.Sprintf("evilscore2:%v", Port), tt.args.ChainCoord)

			if err != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}

			wg.Add(2)

			acceptChan := make(chan string)

			r2.Request(fmt.Sprintf("evilscore1:%v", Port), tt.args.ChainCoord)
			go func() {
				wg.Done()
				r1.Accept(tt.args.ChainCoord)
				acceptChan <- "Connected"
			}()
			go func() {
				wg.Done()
				r2.Accept(tt.args.ChainCoord)
				acceptChan <- "Connected"
			}()
			wg.Wait()

			now := time.Now()
			earliest := now.Add(time.Second * 3)
			ctx, cancel := context.WithDeadline(context.Background(), earliest)
			defer cancel()

			var result string

			select {
			case <-ctx.Done():
				result = tt.want
			case result = <-acceptChan:
			}

			if result != tt.want {
				t.Errorf("result = %v, want %v", result, tt.want)
			}
		})
	}
}

func Test_Compress(t *testing.T) {
	var Port = _port
	_port++
	type args struct {
		ChainCoord *common.Coordinate
		Config1    *Config
		Config2    *Config
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "string",
			args: args{
				ChainCoord: &common.Coordinate{},
				Config1: &Config{
					Network: "mock:Compress1",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/Compress1/",
						BanEvilScore: 100,
					},
				},
				Config2: &Config{
					Network: "mock:Compress2",
					Port:    Port,
					EvilNodeConfig: evilnode.Config{
						StorePath:    "./test/Compress2/",
						BanEvilScore: 100,
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r1, _ := NewRouter(tt.args.Config1)
			r2, _ := NewRouter(tt.args.Config2)

			r1.AddListen(tt.args.ChainCoord)
			r2.AddListen(tt.args.ChainCoord)

			r2.Request(fmt.Sprintf("Compress1:%v", Port), tt.args.ChainCoord)

			wg := sync.WaitGroup{}
			wg.Add(2)

			msg := make([]byte, 1024*1025)
			for i := 0; i < len(msg); i++ {
				msg[i] = uint8(rand.Uint32() / 4)
			}

			var readConn net.Conn
			var writeConn net.Conn
			go func() {
				conn, _, _ := r1.Accept(tt.args.ChainCoord)
				readConn = conn
				wg.Done()
			}()
			go func() {
				conn, _, _ := r2.Accept(tt.args.ChainCoord)
				writeConn = conn
				wg.Done()
			}()
			wg.Wait()

			wg.Add(1)
			strChan := make(chan string)
			go func() {
				wg.Wait()
				bs := make([]byte, 1024*1025)
				n, _ := readConn.Read(bs)
				strChan <- string(bs[:n])
			}()

			go func() {
				writeConn.Write(msg)
				wg.Done()
			}()

			result := <-strChan

			if (result == string(msg)) != tt.want {
				t.Errorf("result = %v, want %v", result, tt.want)
			}
		})
	}
}
