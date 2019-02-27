package router

import (
	"bytes"
	"compress/gzip"
	"hash/crc32"
	"io"
	"net"
	"sync"
	"time"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/common/util"
	"git.fleta.io/fleta/framework/log"
)

type routerPhysical interface {
	removePhysicalConnenction(pc *physicalConnection) error
	acceptConn(conn *logicalConnection, ChainCoord *common.Coordinate) error
	localAddress() string
	port() int
}

type physicalWriter interface {
	write(body []byte, ChainCoord *common.Coordinate, compression uint8) (int64, error)
	RemoteAddr() net.Addr
	LocalAddr() net.Addr
	ID() string
	sendClose(ChainCoord *common.Coordinate) error
}

//MAGICWORD Start of packet
const (
	MAGICWORD  = 'R'
	CLOSELCONN = 'C'

	HANDSHAKE = 'H'
)

//compression types
const (
	UNCOMPRESSED = uint8(0)
	COMPRESSED   = uint8(1)
)

// IEEETable is common table of CRC-32 polynomial.
var IEEETable = crc32.MakeTable(crc32.IEEE)

type physicalConnection struct {
	writeLock        sync.Mutex
	PConn            net.Conn
	pingTime         time.Duration
	handshakeTimeMap *TimerMap
	isClose          bool

	lConn     LConnMap
	r         routerPhysical
	localhost string

	connChan chan *readConn
	connBuff bytes.Buffer

	Address string
}

type readConn struct {
	n   int
	err error
	bs  []byte
}

func newPhysicalConnection(addr string, conn net.Conn, r routerPhysical) *physicalConnection {
	return &physicalConnection{
		PConn:            conn,
		isClose:          false,
		lConn:            LConnMap{},
		handshakeTimeMap: NewTimerMap(time.Second*10, 3),
		r:                r,
	}

}

func (pc *physicalConnection) ID() string {
	return pc.Address
}

func (pc *physicalConnection) doHandshake() error {
	for {
		isHandshake, err := pc._run()
		if err != nil || isHandshake == false {
			return err
		}
	}
}

func (pc *physicalConnection) run() error {
	defer func() {
		if pc.lConn.len() == 0 {
			pc.r.removePhysicalConnenction(pc)
		}
	}()
	for {
		_, err := pc._run()
		if err != nil {
			return err
		}
	}
}

func (pc *physicalConnection) _run() (bool, error) {
	body, isHandshake, ChainCoord, err := pc.readConn()
	if err != nil {
		if err != io.EOF && err != io.ErrClosedPipe {
			log.Error("physicalConnection end ", err)
		}
		pc.Close()
		log.Debug("physicalConnection run end ", pc.PConn.LocalAddr().String(), " ", pc.PConn.RemoteAddr().String())
		return isHandshake, err
	}
	if isHandshake { // handshake
		done, err := pc.handshakeProcess(body, ChainCoord)
		if err != nil {
			return isHandshake, err
		}
		if done == true {
			return false, nil
		}
	} else {
		err := pc.sendToLogicalConn(body, ChainCoord)
		if err != nil {
			return isHandshake, err
		}
	}
	return isHandshake, nil
}

func (pc *physicalConnection) sendClose(ChainCoord *common.Coordinate) error {
	pc.writeLock.Lock()
	defer pc.writeLock.Unlock()

	var wrote int64

	if n, err := util.WriteUint8(pc.PConn, CLOSELCONN); err == nil {
		wrote += n
	} else {
		return err
	}

	if n, err := ChainCoord.WriteTo(pc.PConn); err == nil {
		wrote += int64(n)
	} else {
		return err
	}

	if n, err := util.WriteUint8(pc.PConn, UNCOMPRESSED); err == nil {
		wrote += n
	} else {
		return err
	}

	if n, err := util.WriteUint32(pc.PConn, uint32(0)); err == nil {
		wrote += n
	} else {
		return err
	}
	return nil
}

func (pc *physicalConnection) write(body []byte, ChainCoord *common.Coordinate, compression uint8) (int64, error) {
	var wrote int64
	var buffer bytes.Buffer

	pc.writeLock.Lock()
	defer pc.writeLock.Unlock()

	if pc.PConn == nil {
		return wrote, ErrNotConnected
	}

	if n, err := util.WriteUint8(&buffer, MAGICWORD); err == nil {
		wrote += n
	} else {
		return wrote, err
	}

	if n, err := ChainCoord.WriteTo(&buffer); err == nil {
		wrote += int64(n)
	} else {
		return wrote, err
	}

	if n, err := util.WriteUint8(&buffer, compression); err == nil {
		wrote += n
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
		wrote += n
	} else {
		return wrote, err
	}

	if n, err := buffer.Write(body); err == nil {
		wrote += int64(n)
	} else {
		return wrote, err
	}

	checksum := crc32.Checksum(body, IEEETable)

	if n, err := util.WriteUint32(&buffer, checksum); err == nil {
		wrote += n
	} else {
		return wrote, err
	}

	errCh := make(chan error)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
		_, err := pc.PConn.Write(buffer.Bytes())
		if err != nil {
			pc.PConn.Close()
		}
		errCh <- err
	}()
	wg.Wait()
	deadTimer := time.NewTimer(5 * time.Second)
	select {
	case <-deadTimer.C:
		pc.PConn.Close()
		return wrote, <-errCh
	case err := <-errCh:
		return wrote, err
	}
}

func (pc *physicalConnection) readConn() (body []byte, isHandshake bool, ChainCoord *common.Coordinate, returnErr error) {
	ChainCoord = &common.Coordinate{}
	bs, err := pc.readBytes(12) // header 1 + 6 + 1 + 4
	if err != nil {
		returnErr = err
		return
	}

	bf := bytes.NewBuffer(bs[1:7])
	ChainCoord.ReadFrom(bf)

	compression := uint8(bs[7])

	bodySize := util.BytesToUint32(bs[8:])
	body, err = pc.readBytes(bodySize)
	if err != nil {
		returnErr = err
		return
	}

	if HANDSHAKE == uint8(bs[0]) {
		isHandshake = true
		return
	}
	if MAGICWORD != uint8(bs[0]) {
		returnErr = ErrPacketNotStartedMagicword
		if uint8(bs[0]) == CLOSELCONN {
			returnErr = io.EOF
		}
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

func (pc *physicalConnection) readBytes(n uint32) (read []byte, returnErr error) {
	bs := make([]byte, n)
	if _, err := util.FillBytes(pc.PConn, bs); err != nil {
		return nil, err
	}
	return bs, nil
}

func (pc *physicalConnection) sendToLogicalConn(bs []byte, ChainCoord *common.Coordinate) (err error) {
	pc.lConn.lock()
	defer pc.lConn.unlock()

	if lConn, has := pc.lConn.load(*ChainCoord); has {
		err = lConn.sendToLogical(bs)
	} else {
		return ErrNotFoundLogicalConnection
	}
	return
}

func (pc *physicalConnection) makeLogicalConnenction(ChainCoord *common.Coordinate, ping time.Duration) (*logicalConnection, bool) {
	pc.lConn.lock()
	defer pc.lConn.unlock()

	l, has := pc.lConn.load(*ChainCoord)
	if !has {
		closeChan := make(chan bool)
		l = newLConn(pc, ChainCoord, ping, closeChan)
		pc.lConn.store(*ChainCoord, l)

		go func(closeChan chan bool, ChainCoord *common.Coordinate) {
			<-closeChan
			pc.lConn.lock()
			defer pc.lConn.unlock()
			pc.lConn.delete(*ChainCoord)
		}(closeChan, ChainCoord)
	}
	new := !has
	return l, new
}

func (pc *physicalConnection) RemoteAddr() net.Addr {
	return pc.PConn.RemoteAddr()
}
func (pc *physicalConnection) LocalAddr() net.Addr {
	return pc.PConn.LocalAddr()
}

//Close is used to sever all physical connections and logical connections related to physical connections
func (pc *physicalConnection) Close() (err error) {
	pc.isClose = true
	err = pc.PConn.Close()

	pc.lConn.lock()
	pc.lConn.Range(func(c common.Coordinate, lc *logicalConnection) bool {
		lc.Close()
		return true
	})
	pc.lConn.unlock()

	return
}
