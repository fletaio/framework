package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/core/block"
	"git.fleta.io/fleta/core/data"
	"git.fleta.io/fleta/core/kernel"
	"git.fleta.io/fleta/core/message_def"
	"git.fleta.io/fleta/core/transaction"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// JRPCRequest is a jrpc request
type JRPCRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      interface{}    `json:"id"`
	Method  string         `json:"method"`
	Params  []*json.Number `json:"params"`
}

// JRPCResponse is a jrpc response
type JRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result"`
	Error   interface{} `json:"error"`
}

// EventNotify is a notification of events
type EventNotify struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// Handler handles a rpc method
type Handler func(kn *kernel.Kernel, ID interface{}, arg *Argument) (interface{}, error)

// Manager provides a rpc server
type Manager struct {
	sync.Mutex
	e            *echo.Echo
	funcMap      map[string]Handler
	eventLocker  []*sync.Mutex
	eventWatcher []*websocket.Conn
}

// NewManager returns a Manager
func NewManager() *Manager {
	rm := &Manager{
		e:            echo.New(),
		funcMap:      map[string]Handler{},
		eventLocker:  []*sync.Mutex{},
		eventWatcher: []*websocket.Conn{},
	}
	return rm
}

// Close terminates the rpc manager
func (rm *Manager) Close() {
	rm.e.Close()
}

// Add adds a rpc methods to the manager
func (rm *Manager) Add(Method string, fn Handler) {
	rm.Lock()
	defer rm.Unlock()

	rm.funcMap[Method] = fn
}

func (rm *Manager) handleJRPC(kn *kernel.Kernel, req *JRPCRequest) *JRPCResponse {
	args := []*string{}
	for _, v := range req.Params {
		if v == nil {
			args = append(args, nil)
		} else {
			args = append(args, (*string)(v))
		}
	}
	rm.Lock()
	fn := rm.funcMap[req.Method]
	rm.Unlock()

	kn.Lock()
	ret, err := fn(kn, req.ID, NewArgument(args))
	kn.Unlock()
	if req.ID == nil {
		return nil
	} else {
		res := &JRPCResponse{
			JSONRPC: req.JSONRPC,
			ID:      req.ID,
		}
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Result = ret
		}
		return res
	}
}

func (rm *Manager) handleEvent(noti *EventNotify) {
	conns := []*websocket.Conn{}
	locks := []*sync.Mutex{}
	rm.Lock()
	for _, conn := range rm.eventWatcher {
		conns = append(conns, conn)
	}
	for _, lock := range rm.eventLocker {
		locks = append(locks, lock)
	}
	rm.Unlock()

	data, err := json.Marshal(noti)
	if err != nil {
		return
	}
	for i := range conns {
		func(conn *websocket.Conn, lock *sync.Mutex) {
			errCh := make(chan error)
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				wg.Done()
				(*lock).Lock()
				err := conn.WriteMessage(websocket.TextMessage, data)
				(*lock).Unlock()
				errCh <- err
			}()
			wg.Wait()
			deadTimer := time.NewTimer(5 * time.Second)
			select {
			case <-deadTimer.C:
				conn.Close()
			case <-errCh:
				if err != nil {
					conn.Close()
				}
			}
		}(conns[i], locks[i])
	}
}

// Run runs a server to provide RPC a http service and a websocket service
func (rm *Manager) Run(kn *kernel.Kernel, Bind string) error {
	rm.e.Use(middleware.CORSWithConfig(middleware.DefaultCORSConfig))
	/*
		rm.e.Use(middleware.BasicAuth(func(username string, password string, c echo.Context) (bool, error) {
			if username == "joe" && password == "secret" {
				return true, nil
			}
			return false, nil
		}))
	*/
	rm.e.POST("/api/endpoints/http", func(c echo.Context) error {
		defer c.Request().Body.Close()
		dec := json.NewDecoder(c.Request().Body)
		dec.UseNumber()

		var req JRPCRequest
		if err := dec.Decode(&req); err != nil {
			return err
		}
		res := rm.handleJRPC(kn, &req)
		if res == nil {
			return c.NoContent(http.StatusOK)
		} else {
			return c.JSON(http.StatusOK, res)
		}
	})
	rm.e.GET("/api/endpoints/websocket", func(c echo.Context) error {
		conn, err := upgrader.Upgrade(c.Response().Writer, c.Request(), nil)
		if err != nil {
			return err
		}
		defer conn.Close()

		Type := strings.ToLower(c.QueryParam("type"))
		switch Type {
		case "event":
			rm.Lock()
			rm.eventWatcher = append(rm.eventWatcher, conn)
			rm.eventLocker = append(rm.eventLocker, &sync.Mutex{})
			rm.Unlock()

			defer func() {
				rm.Lock()
				eventWatcher := []*websocket.Conn{}
				eventLocker := []*sync.Mutex{}
				for i, c := range rm.eventWatcher {
					if c != conn {
						eventWatcher = append(eventWatcher, c)
						eventLocker = append(eventLocker, rm.eventLocker[i])
					}
				}
				rm.eventWatcher = eventWatcher
				rm.eventLocker = eventLocker
				rm.Unlock()
			}()
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return err
				}
			}
		default:
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					return err
				}
				dec := json.NewDecoder(bytes.NewReader(data))
				dec.UseNumber()

				var req JRPCRequest
				if err := dec.Decode(&req); err != nil {
					return err
				}
				res := rm.handleJRPC(kn, &req)
				if res != nil {
					errCh := make(chan error)
					var wg sync.WaitGroup
					wg.Add(1)
					go func() {
						wg.Done()
						err := conn.WriteJSON(res)
						errCh <- err
					}()
					wg.Wait()
					deadTimer := time.NewTimer(5 * time.Second)
					select {
					case <-deadTimer.C:
						return ErrClientTimeout
					case <-errCh:
						if err != nil {
							return err
						}
					}
				}
			}
		}
	})
	return rm.e.Start(Bind)
}

// OnProcessBlock called when processing a block to the chain (error prevent processing block)
func (rm *Manager) OnProcessBlock(kn *kernel.Kernel, b *block.Block, s *block.ObserverSigned, ctx *data.Context) error {
	return nil
}

// AfterProcessBlock called when processed block to the chain
func (rm *Manager) AfterProcessBlock(kn *kernel.Kernel, b *block.Block, s *block.ObserverSigned, ctx *data.Context) {
	rm.handleEvent(&EventNotify{
		Type: "AfterProcessBlock",
		Data: map[string]interface{}{
			"hash":     b.Header.Hash(),
			"header":   b.Header,
			"tx_count": len(b.Body.Transactions),
		},
	})
}

// OnPushTransaction called when pushing a transaction to the transaction pool (error prevent push transaction)
func (rm *Manager) OnPushTransaction(kn *kernel.Kernel, tx transaction.Transaction, sigs []common.Signature) error {
	return nil
}

// AfterPushTransaction called when pushed a transaction to the transaction pool
func (rm *Manager) AfterPushTransaction(kn *kernel.Kernel, tx transaction.Transaction, sigs []common.Signature) {
	rm.handleEvent(&EventNotify{
		Type: "AfterPushTransaction",
		Data: map[string]interface{}{
			"tx_hash": tx.Hash(),
			"tx":      tx,
		},
	})
}

// DoTransactionBroadcast called when a transaction need to be broadcast
func (rm *Manager) DoTransactionBroadcast(kn *kernel.Kernel, msg *message_def.TransactionMessage) {
	rm.handleEvent(&EventNotify{
		Type: "DoTransactionBroadcast",
		Data: map[string]interface{}{
			"tx_hash": msg.Tx.Hash(),
			"tx":      msg.Tx,
		},
	})
}

// DebugLog provides internal debug logs to handlers
func (rm *Manager) DebugLog(kn *kernel.Kernel, args ...interface{}) {
	if len(args) > 0 {
		str := fmt.Sprintln(args...)
		rm.handleEvent(&EventNotify{
			Type: "DebugLog",
			Data: map[string]interface{}{
				"log": str[:len(str)-1],
			},
		})
	}
}
