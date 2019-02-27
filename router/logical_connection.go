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

type dataCase struct {
	data   []byte
	size   int
	readed int
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
	c          *dataCase
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
			return 0, io.EOF
		}
		l.c = &dataCase{
			data: data,
			size: len(data),
		}

		if len(b) >= len(l.c.data) {
			copy(b[:], l.c.data[:len(l.c.data)])
			l.c.readed += len(l.c.data)
			return len(l.c.data), nil
		}
		copy(b[:], l.c.data[:len(b)])
		l.readBuf.Write(l.c.data[len(b):])
		l.c.readed += len(b)
		return len(b), nil
	}

	n, err := l.readBuf.Read(b)
	l.c.readed += n
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

func (l *logicalConnection) Reset() {
	bs := make([]byte, l.c.size-l.c.readed)
	log.Info(string(l.c.data))
	l.readBuf.Read(bs)

}

func (l *logicalConnection) PrintData() string {
	return string(l.c.data)
}

//RemoteAddr is remote address infomation
func (l *logicalConnection) RemoteAddr() net.Addr {
	return l.PConn.RemoteAddr()
}

//Write is write byte to buffer
func (l *logicalConnection) sendToLogical(data []byte) error {
	if l.isClosed == true {
		return io.EOF
	}
	l.sendChan <- data
	return nil
}

//Close is closes data communication channel
func (l *logicalConnection) Close() error {
	if l.isClosed != true {
		l.isClosed = true
		l.close <- true
		log.Debug("receiver close ", l.LocalAddr().String(), " ", l.ID())
		close(l.sendChan)
	}
	return nil
}
