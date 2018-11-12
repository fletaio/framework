package router

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/framework/log"
)

type logicalConnection struct {
	ChainCoord        *common.Coordinate
	chainSideReceiver net.Conn
	receiver          *receiver
}

type receiver struct {
	recvChan   <-chan []byte
	sendChan   chan<- []byte
	readBuf    bytes.Buffer
	writeBuf   bytes.Buffer
	localAddr  net.Addr
	remoteAddr net.Addr
	isClosed   bool
	closeLock  sync.Mutex
}

type addr struct {
	network string
	address string
}

func (c *addr) Network() string {
	return c.network
}
func (c *addr) String() string {
	return c.address
}

func newReceiver(recvChan <-chan []byte, sendChan chan<- []byte, localhost string, remoteAddr net.Addr) *receiver {
	return &receiver{
		recvChan:   recvChan,
		sendChan:   sendChan,
		remoteAddr: remoteAddr,
		localAddr: &addr{
			network: "tcp",
			address: localhost,
		},
	}
}

func (r *receiver) Read(b []byte) (int, error) {
	if r.readBuf.Len() == 0 {
		data, ok := <-r.recvChan
		if !ok {
			r.Close()
			return 0, io.EOF
		}
		if len(b) >= len(data) {
			copy(b[:], data[:len(data)])
			return len(data), nil
		}
		copy(b[:], data[:len(b)])
		r.readBuf.Write(data[len(b):])
		return len(b), nil
	}

	n, err := r.readBuf.Read(b)
	return n, err
}

//Recv is receive
func (r *receiver) Recv() ([]byte, error) {
	data, ok := <-r.recvChan
	if !ok {
		r.Close()
		return nil, io.EOF
	}
	return data, nil
}

//Write is write byte to buffer
func (r *receiver) Write(data []byte) (int, error) {
	n, err := r.writeBuf.Write(data)
	if err != nil {
		return n, err
	}
	if r.writeBuf.Len() > 4 {
		err = r.Flush()
	}
	return n, err
}

//Flush sends all of the buffered data to Connection.
func (r *receiver) Flush() (err error) {
	if r.isClosed {
		return io.EOF
	}
	defer func() {
		if rc := recover(); rc != nil {
			if _, is := rc.(error); is {
				err = io.EOF
			}
		}
	}()
	if r.writeBuf.Len() == 0 {
		log.Debug("receiver Flush empty")
		r.sendChan <- []byte{}
		return
	}

	bs := make([]byte, r.writeBuf.Len())
	r.writeBuf.Read(bs)
	r.sendChan <- bs

	return
}

func (r *receiver) SetDeadline(t time.Time) error {
	return nil
}
func (r *receiver) SetReadDeadline(t time.Time) error {
	return nil
}
func (r *receiver) SetWriteDeadline(t time.Time) error {
	return nil
}

//LocalAddr is local address infomation
func (r *receiver) LocalAddr() net.Addr {
	return r.localAddr
}

//RemoteAddr is remote address infomation
func (r *receiver) RemoteAddr() net.Addr {
	return r.remoteAddr
}

//Close is closes data communication channel
func (r *receiver) Close() error {
	if r.isClosed != true {
		r.isClosed = true
		log.Debug("receiver close ", r.localAddr.String(), " ", r.remoteAddr.String())
		close(r.sendChan)
	}
	return nil
}
