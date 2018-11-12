package router

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
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
	acceptConn(conn net.Conn, ChainCoord *common.Coordinate) error
}

//MAGICWORD Start of packet
const MAGICWORD = 'R'

//HANDSHAKE Start of handshake packet
const (
	HANDSHAKE = 'H'
	FORONE    = 1
	REPEAT    = 8
)

//compression types
const (
	UNCOMPRESSED = uint8(0)
	COMPRESSED   = uint8(1)
)

// IEEETable is common table of CRC-32 polynomial.
var IEEETable = crc32.MakeTable(crc32.IEEE)

type physicalConnection struct {
	writeLock sync.Mutex
	PConn     net.Conn
	isClose   bool

	addr      RemoteAddr
	lConnLock sync.Mutex
	lConn     LConnMap //map[common.Coordinate]*logicalConnection
	r         routerPhysical
	localhost string

	connChan chan *readConn
	connBuff bytes.Buffer
}

type readConn struct {
	n   int
	err error
	bs  []byte
}

func newPhysicalConnection(addr RemoteAddr, localhost string, conn net.Conn, r routerPhysical) *physicalConnection {
	return &physicalConnection{
		addr:      addr,
		PConn:     conn,
		isClose:   false,
		lConn:     LConnMap{}, //map[common.Coordinate]*logicalConnection{},
		localhost: localhost,
		r:         r,
	}

}

func (pc *physicalConnection) run() error {
	defer func() {
		if pc.lConn.len() == 0 {
			pc.r.removePhysicalConnenction(pc)
		}
	}()
	for {
		body, ChainCoord, err := pc.readConn()
		if err != nil {
			if err != io.EOF && err != io.ErrClosedPipe {
				log.Error("physicalConnection run end ", err)
			}
			pc.Close()
			log.Debug("physicalConnection run end ", pc.localhost, " ", pc.PConn.RemoteAddr().String())
			return err
		}
		if len(body) == 1 && ChainCoord != nil { // handshake
			log.Debug("response handshake ", body[0], " ", pc.localhost, " ", pc.PConn.RemoteAddr().String())
			if body[0] == FORONE {

				pc.handshakeResponse(ChainCoord)
				if conn, new := pc.makeLogicalConnenction(ChainCoord); new == true {
					err := pc.r.acceptConn(conn, ChainCoord)
					if err != nil {
						log.Error("physicalConnection run acceptConn err : ", err)
					}
				}

			} else if body[0] == REPEAT {
				pc.handshakeResponse(ChainCoord)
			}

		} else {
			pc.sendToLogicalConn(body, ChainCoord)
		}
	}
}

func (pc *physicalConnection) readConn() (body []byte, ChainCoord *common.Coordinate, returnErr error) {
	ChainCoord = &common.Coordinate{}
	bs, err := pc.readBytes(12) // header 1 + 6 + 1 + 4
	if err != nil {
		return bs, nil, err
	}

	bf := bytes.NewBuffer(bs[1:7])
	ChainCoord.ReadFrom(bf)

	if HANDSHAKE == uint8(bs[0]) {
		body, err := pc.readBytes(1)
		if err != nil {
			return nil, ChainCoord, err
		}
		return body, ChainCoord, nil
	}
	if MAGICWORD != uint8(bs[0]) {
		return nil, ChainCoord, ErrPacketNotStartedMagicword
	}

	compression := uint8(bs[7])

	bodySize := util.BytesToUint32(bs[8:])
	body, err = pc.readBytes(bodySize)
	if err != nil {
		return nil, ChainCoord, err
	}

	checksum := crc32.Checksum(body, IEEETable)

	if compression == COMPRESSED {
		var buf bytes.Buffer
		gr, err := gzip.NewReader(bytes.NewBuffer(body))
		defer gr.Close()
		_, err = buf.ReadFrom(gr)
		if err != nil {
			return nil, ChainCoord, err
		}
		body = buf.Bytes()
	} else if compression != UNCOMPRESSED {
		return nil, ChainCoord, ErrNotMatchCompressionType
	}

	checksumBs, err := pc.readBytes(4)
	if err != nil {
		return nil, ChainCoord, err
	}

	readedChecksum := util.BytesToUint32(checksumBs)
	if readedChecksum != checksum {
		return nil, ChainCoord, ErrInvalidIntegrity
	}

	return
}

func (pc *physicalConnection) readBytes(n uint32) (read []byte, returnErr error) {
	readedN := uint32(0)
	for readedN < n {
		bs := make([]byte, n-readedN)
		readN, err := pc.PConn.Read(bs)
		if err != nil {
			return read, err
		}
		readedN += uint32(readN)
		read = append(read, bs[:readN]...)
	}
	return
}

func (pc *physicalConnection) sendToLogicalConn(bs []byte, ChainCoord *common.Coordinate) (err error) {
	if bs != nil {
		if lConn, has := pc.lConn.load(*ChainCoord); has {
			_, err = lConn.receiver.Write(bs)
		} else {
			return ErrNotFoundLogicalConnection
		}
		return
	}

	return nil
}

func (pc *physicalConnection) makeLogicalConnenction(ChainCoord *common.Coordinate) (net.Conn, bool) {
	pc.lConnLock.Lock()
	defer pc.lConnLock.Unlock()

	l, has := pc.lConn.load(*ChainCoord)
	if !has {
		cChan := make(chan []byte, 2048)
		rChan := make(chan []byte, 2048)

		rc := newReceiver(rChan, cChan, pc.localhost, pc.PConn.RemoteAddr())
		l = &logicalConnection{
			ChainCoord:        ChainCoord,
			chainSideReceiver: rc,
			receiver:          newReceiver(cChan, rChan, pc.localhost, pc.PConn.RemoteAddr()),
		}
		pc.lConn.store(*ChainCoord, l)

		go pc.runLConn(l)
	}
	new := !has
	return l.chainSideReceiver, new
}

func (pc *physicalConnection) handshakeByte(ChainCoord *common.Coordinate, body byte) (result []byte) {
	result = make([]byte, 0, 13) // 1+6+1+4+1

	result = append(result, HANDSHAKE)
	result = append(result, ChainCoord.Bytes()...)
	result = append(result, UNCOMPRESSED)

	BNum := make([]byte, 4)
	binary.LittleEndian.PutUint32(BNum, 1)
	result = append(result, BNum...)
	result = append(result, body)
	return

}

func (pc *physicalConnection) handshakeResponse(ChainCoord *common.Coordinate) (wrote int64, err error) {
	return pc._handshake(ChainCoord, FORONE)
}

func (pc *physicalConnection) handshake(ChainCoord *common.Coordinate) (wrote int64, err error) {
	return pc._handshake(ChainCoord, REPEAT)
}

func (pc *physicalConnection) _handshake(ChainCoord *common.Coordinate, body byte) (wrote int64, err error) {
	if pc.PConn == nil {
		return wrote, ErrNotConnected
	}

	_, has := pc.lConn.load(*ChainCoord)

	if !has {
		bs := pc.handshakeByte(ChainCoord, body)
		go func(has bool, bs []byte, ChainCoord *common.Coordinate) {
			for !has && pc.isClose != true {
				pc.writeLock.Lock()
				log.Debug("handshake ", bs[:12], " ", body, " ", pc.localhost, " ", pc.PConn.RemoteAddr().String())
				if n, err := pc.PConn.Write(bs); err == nil {
					wrote = int64(n)
				}
				pc.writeLock.Unlock()

				time.Sleep(time.Second * 30)

				_, has = pc.lConn.load(*ChainCoord)
			}
		}(has, bs, ChainCoord)
	}

	return wrote, nil
}

func (pc *physicalConnection) write(body []byte, ChainCoord *common.Coordinate) (wrote int64, err error) {
	var writer bytes.Buffer
	compression := UNCOMPRESSED
	if len(body) > 10240 {
		compression = COMPRESSED
	}

	if n, err := util.WriteUint8(&writer, MAGICWORD); err == nil {
		wrote += n
	} else {
		return wrote, err
	}

	if n, err := ChainCoord.WriteTo(&writer); err == nil {
		wrote += int64(n)
	} else {
		return wrote, err
	}

	if n, err := util.WriteUint8(&writer, compression); err == nil {
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
	if n, err := util.WriteUint32(&writer, uint32(size)); err == nil {
		wrote += n
	} else {
		return wrote, err
	}

	if n, err := writer.Write(body); err == nil {
		wrote += int64(n)
	} else {
		return wrote, err
	}

	checksum := crc32.Checksum(body, IEEETable)

	if n, err := util.WriteUint32(&writer, checksum); err == nil {
		wrote += n
	} else {
		return wrote, err
	}

	if pc.PConn == nil {
		return wrote, ErrNotConnected
	}

	pc.writeLock.Lock()
	bs := writer.Bytes()
	n, err := pc.PConn.Write(bs)
	pc.writeLock.Unlock()
	if err == nil {
		wrote = int64(n)
	} else {
		return wrote, err
	}

	return wrote, nil
}

func (pc *physicalConnection) runLConn(lc *logicalConnection) error {
	defer func() {
		pc.lConn.delete(*lc.ChainCoord)
		lc.receiver.Close()
		if pc.lConn.len() == 0 {
			pc.r.removePhysicalConnenction(pc)
		}
	}()
	for pc.isClose != true {
		bs, err := lc.receiver.Recv()
		if err != nil {
			return err
		}
		if _, err := pc.write(bs, lc.ChainCoord); err != nil {
			return err
		}
	}
	return io.EOF
}

//Close is used to sever all physical connections and logical connections related to physical connections
func (pc *physicalConnection) Close() (err error) {
	pc.isClose = true
	err = pc.PConn.Close()

	pc.lConn.Range(func(c common.Coordinate, lc *logicalConnection) bool {
		lc.receiver.Close()
		return true
	})

	return
}
