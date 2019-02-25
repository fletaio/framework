package router

import (
	"bytes"
	"io"
	"net"
	"time"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/framework/log"
)

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

type logicalConnection struct {
	ChainCoord *common.Coordinate
	recvChan   <-chan []byte
	sendChan   chan<- []byte
	PConn      physicalWriter
	ping       time.Duration
	readBuf    bytes.Buffer
	isClosed   bool
	close      chan<- bool
}

func newLConn(sendPConn physicalWriter, ChainCoord *common.Coordinate, ping time.Duration, close chan<- bool) *logicalConnection {
	dataChan := make(chan []byte, 2048)

	return &logicalConnection{
		ChainCoord: ChainCoord,
		recvChan:   dataChan,
		sendChan:   dataChan,
		PConn:      sendPConn,
		ping:       ping,
		close:      close,
	}
}

func (l *logicalConnection) Read(b []byte) (int, error) {
	if l.readBuf.Len() == 0 {
		data, ok := <-l.recvChan
		if !ok {
			l.Close()
			return 0, io.EOF
		}
		if len(b) >= len(data) {
			copy(b[:], data[:len(data)])
			return len(data), nil
		}
		copy(b[:], data[:len(b)])
		l.readBuf.Write(data[len(b):])
		return len(b), nil
	}

	n, err := l.readBuf.Read(b)
	return n, err
}

//Write is write byte to buffer
func (l *logicalConnection) Write(data []byte) (int, error) {
	var compression = UNCOMPRESSED
	if len(data) > 1048576 {
		compression = COMPRESSED
	}
	n, err := l.PConn.write(data, l.ChainCoord, compression)
	return int(n), err
}

func (l *logicalConnection) SetDeadline(t time.Time) error {
	return nil
}
func (l *logicalConnection) SetReadDeadline(t time.Time) error {
	return nil
}
func (l *logicalConnection) SetWriteDeadline(t time.Time) error {
	return nil
}

//LocalAddr is local address infomation
func (l *logicalConnection) LocalAddr() net.Addr {
	return l.PConn.LocalAddr()
}

//ID is Node ID
func (l *logicalConnection) ID() string {
	return l.PConn.ID()
}

//RemoteAddr is remote address infomation
func (l *logicalConnection) RemoteAddr() net.Addr {
	return l.PConn.RemoteAddr()
}

//Write is write byte to buffer
func (l *logicalConnection) sendToLogical(data []byte) {
	l.sendChan <- data
}

//Close is closes data communication channel
func (l *logicalConnection) Close() error {
	if l.isClosed != true {
		l.isClosed = true
		log.Debug("receiver close ", l.LocalAddr().String(), " ", l.ID())
		l.PConn.sendClose(l.ChainCoord)
		close(l.sendChan)
		l.close <- true
	}
	return nil
}
