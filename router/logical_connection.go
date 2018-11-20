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
	ChainCoord    *common.Coordinate
	recvChan      <-chan []byte
	sendChan      chan<- []byte
	PConn         physicalWriter
	ping          time.Duration
	readBuf       bytes.Buffer
	isClosed      bool
	closeCallback func(cd *common.Coordinate)
}

func newLConn(recvChan <-chan []byte, sendChan chan<- []byte, sendPConn physicalWriter, ChainCoord *common.Coordinate, ping time.Duration, closeCallback func(cd *common.Coordinate)) *logicalConnection {
	return &logicalConnection{
		ChainCoord:    ChainCoord,
		recvChan:      recvChan,
		sendChan:      sendChan,
		PConn:         sendPConn,
		ping:          ping,
		closeCallback: closeCallback,
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
	n, err := l.PConn.write(data, l.ChainCoord)
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
		log.Debug("receiver close ", l.LocalAddr().String(), " ", l.RemoteAddr().String())
		close(l.sendChan)
		l.closeCallback(l.ChainCoord)
	}
	return nil
}
