package router

import (
	"bytes"
	"compress/gzip"
	"hash/crc32"
	"log"
	"net"
	"sync"
	"time"

	"github.com/fletaio/common"
	"github.com/fletaio/common/util"
)

type routerPhysical interface {
	acceptConn(pc *RouterConn) error
	localAddress() string
	chainCoord() *common.Coordinate
	removeRouterConn(conn net.Conn)
	unsafeRemoveRouterConn(conn net.Conn)
	port() int
}

//MAGICWORD Start of packet
const (
	MAGICWORD = 'R'
	HEARTBIT  = 'H'
)

//compression types
const (
	UNCOMPRESSED = uint8(0)
	COMPRESSED   = uint8(1)
)

// IEEETable is common table of CRC-32 polynomial.
var IEEETable = crc32.MakeTable(crc32.IEEE)

type RouterConn struct {
	writeLock sync.Mutex
	pConn     net.Conn
	pingTime  time.Duration
	isClose   bool

	r         routerPhysical
	localhost string

	connChan chan *readConn
	connBuff bytes.Buffer

	heartBitTime time.Time

	readBuf bytes.Buffer
	c       *dataCase

	Address string
}

type readConn struct {
	n   int
	err error
	bs  []byte
}

func newRouterConn(addr string, conn net.Conn, r routerPhysical) *RouterConn {
	pc := &RouterConn{
		pConn:        conn,
		isClose:      false,
		r:            r,
		heartBitTime: time.Now(),
	}
	pc.checkHeartBit()
	return pc

}

func (pc *RouterConn) checkHeartBit() {
	go func() {
		for {
			time.Sleep(3 * time.Second)
			passed := time.Now().Sub(pc.heartBitTime)
			if passed > 5*time.Second {
				log.Println("no heartbit while", passed, "/", 5*time.Second, ":", pc.ID(), pc.LocalAddr().String(), pc.RemoteAddr().String())
				pc.Close()
				return
			}
		}
	}()
}

func (pc *RouterConn) ID() string {
	return pc.Address
}

func (pc *RouterConn) Write(body []byte) (int, error) {
	return pc.write(body, UNCOMPRESSED)
}

func (pc *RouterConn) write(body []byte, compression uint8) (int, error) {
	var wrote int
	var buffer bytes.Buffer

	pc.writeLock.Lock()
	defer pc.writeLock.Unlock()

	if pc.pConn == nil {
		return wrote, ErrNotConnected
	}

	if n, err := util.WriteUint8(&buffer, MAGICWORD); err == nil {
		wrote += int(n)
	} else {
		return wrote, err
	}

	if n, err := pc.r.chainCoord().WriteTo(&buffer); err == nil {
		wrote += int(n)
	} else {
		return wrote, err
	}

	if n, err := util.WriteUint8(&buffer, compression); err == nil {
		wrote += int(n)
	} else {
		return wrote, err
	}

	if compression == COMPRESSED {
		buf := &bytes.Buffer{}
		gw := gzip.NewWriter(buf)

		if _, err := gw.Write(body); err != nil {
			return wrote, err
		}
		if err := gw.Flush(); err != nil {
			return wrote, err
		}
		if err := gw.Close(); err != nil {
			return wrote, err
		}

		body = buf.Bytes()
	}

	size := len(body)
	if n, err := util.WriteUint32(&buffer, uint32(size)); err == nil {
		wrote += int(n)
	} else {
		return wrote, err
	}

	if n, err := buffer.Write(body); err == nil {
		wrote += int(n)
	} else {
		return wrote, err
	}

	checksum := crc32.Checksum(body, IEEETable)

	if n, err := util.WriteUint32(&buffer, checksum); err == nil {
		wrote += int(n)
	} else {
		return wrote, err
	}

	errCh := make(chan error)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
		_, err := pc.pConn.Write(buffer.Bytes())
		if err != nil {
			pc.Close()
		}
		errCh <- err
	}()
	wg.Wait()
	deadTimer := time.NewTimer(5 * time.Second)
	select {
	case <-deadTimer.C:
		pc.Close()
		return wrote, <-errCh
	case err := <-errCh:
		deadTimer.Stop()
		return wrote, err
	}
}

type dataCase struct {
	data   []byte
	size   int
	readed int
}

func (pc *RouterConn) Read(b []byte) (int, error) {
	if pc.readBuf.Len() == 0 {
		data, err := pc.ReadConn()
		if err != nil {
			return 0, err
		}
		pc.c = &dataCase{
			data: data,
			size: len(data),
		}

		if len(b) >= len(pc.c.data) {
			copy(b[:], pc.c.data[:len(pc.c.data)])
			pc.c.readed += len(pc.c.data)
			return len(pc.c.data), nil
		}
		copy(b[:], pc.c.data[:len(b)])
		pc.readBuf.Write(pc.c.data[len(b):])
		pc.c.readed += len(b)
		return len(b), nil
	}

	n, err := pc.readBuf.Read(b)
	pc.c.readed += n
	return n, err
}

func (pc *RouterConn) ReadConn() (body []byte, returnErr error) {
	ChainCoord := &common.Coordinate{}
	var bs []byte
	var err error
	for {
		bs, err = pc.readBytes(1) // header 1
		if err != nil {
			returnErr = err
			return
		}
		if bs[0] == HEARTBIT {
			pc.heartBitTime = time.Now()
		} else {
			break
		}
	}
	header, err := pc.readBytes(11) // 6 + 1 + 4
	if err != nil {
		returnErr = err
		return
	}
	bs = append(bs, header...)

	bf := bytes.NewBuffer(bs[1:7])
	ChainCoord.ReadFrom(bf)

	compression := uint8(bs[7])

	bodySize := util.BytesToUint32(bs[8:])
	body, err = pc.readBytes(bodySize)
	if err != nil {
		returnErr = err
		return
	}

	if MAGICWORD != uint8(bs[0]) {
		returnErr = ErrPacketNotStartedMagicword
		return
	}

	checksum := crc32.Checksum(body, IEEETable)

	if compression == COMPRESSED {
		var buf bytes.Buffer
		gr, err := gzip.NewReader(bytes.NewBuffer(body))
		defer gr.Close()
		_, err = buf.ReadFrom(gr)
		if err != nil {
			returnErr = err
			return
		}
		body = buf.Bytes()
	} else if compression != UNCOMPRESSED {
		returnErr = ErrNotMatchCompressionType
		return
	}

	checksumBs, err := pc.readBytes(4)
	if err != nil {
		returnErr = err
		return
	}

	readedChecksum := util.BytesToUint32(checksumBs)
	if readedChecksum != checksum {
		returnErr = ErrInvalidIntegrity
		return
	}

	return
}

func (pc *RouterConn) readBytes(n uint32) (read []byte, returnErr error) {
	pc.SetDeadline(time.Now().Add(time.Second * 15))
	bs := make([]byte, n)
	_, err := util.FillBytes(pc.pConn, bs)
	if err != nil { //has error
		return nil, err
	}
	return bs, nil
}

func (pc *RouterConn) SendHeartBit() {
	_, err := pc.pConn.Write([]byte{HEARTBIT})
	if err != nil {
		pc.pConn.Close()
	}
}

func (pc *RouterConn) RemoteAddr() net.Addr {
	return pc.pConn.RemoteAddr()
}
func (pc *RouterConn) LocalAddr() net.Addr {
	return pc.pConn.LocalAddr()
}

//Close is used to sever all physical connections and logical connections related to physical connections
func (pc *RouterConn) Close() (err error) {
	pc.isClose = true
	err = pc.pConn.Close()
	pc.r.removeRouterConn(pc.pConn)
	return
}

//LockFreeClose is used to sever all physical connections and logical connections related to physical connections without lock
func (pc *RouterConn) LockFreeClose() (err error) {
	pc.isClose = true
	err = pc.pConn.Close()
	pc.r.unsafeRemoveRouterConn(pc.pConn)
	return err
}

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
//
// A deadline is an absolute time after which I/O operations
// fail with a timeout (see type Error) instead of
// blocking. The deadline applies to all future and pending
// I/O, not just the immediately following call to Read or
// Write. After a deadline has been exceeded, the connection
// can be refreshed by setting a deadline in the future.
//
// An idle timeout can be implemented by repeatedly extending
// the deadline after successful Read or Write calls.
//
// A zero value for t means I/O operations will not time out.
func (pc *RouterConn) SetDeadline(t time.Time) error {
	return pc.pConn.SetDeadline(t)
}

// SetReadDeadline sets the deadline for future Read calls
// and any currently-blocked Read call.
// A zero value for t means Read will not time out.
func (pc *RouterConn) SetReadDeadline(t time.Time) error {
	return pc.pConn.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (pc *RouterConn) SetWriteDeadline(t time.Time) error {
	return pc.pConn.SetWriteDeadline(t)
}
