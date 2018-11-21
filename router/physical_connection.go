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

	"github.com/satori/go.uuid"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/common/util"
	"git.fleta.io/fleta/framework/log"
)

type routerPhysical interface {
	removePhysicalConnenction(pc *physicalConnection) error
	acceptConn(conn *logicalConnection, ChainCoord *common.Coordinate) error
}

type physicalWriter interface {
	write(body []byte, ChainCoord *common.Coordinate, compression uint8) (int64, error)
	RemoteAddr() net.Addr
	LocalAddr() net.Addr
	sendClose(ChainCoord *common.Coordinate) error
}

//MAGICWORD Start of packet
const (
	MAGICWORD  = 'R'
	CLOSELCONN = 'C'

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

func (pc *physicalConnection) run() error {
	defer func() {
		if pc.lConn.len() == 0 {
			pc.r.removePhysicalConnenction(pc)
		}
	}()
	for {
		body, isHandshake, ChainCoord, err := pc.readConn()
		if err != nil {
			if err != io.EOF && err != io.ErrClosedPipe {
				log.Error("physicalConnection end ", err)
			}
			pc.Close()
			log.Debug("physicalConnection run end ", pc.localhost, " ", pc.PConn.RemoteAddr().String())
			return err
		}
		if isHandshake { // handshake
			log.Debug("response handshake ", body[0], " ", pc.localhost, " ", pc.PConn.RemoteAddr().String())
			if body[0] == FORONE {
				if len(body) > 17 {
					if uuid, err := uuid.FromBytes(body[1:17]); err != nil {
						pc.handshake(ChainCoord)
					} else {
						if len(body) > 32 {
							pc.handshakeResponse(ChainCoord, body[17:33])
						}
						if inow, has := pc.handshakeTimeMap.Load(uuid.String()); has {
							if now, ok := inow.(time.Time); ok {
								pc.pingTime = time.Now().Sub(now)
								if conn, new := pc.makeLogicalConnenction(ChainCoord, pc.pingTime); new == true {
									err := pc.r.acceptConn(conn, ChainCoord)
									if err != nil {
										log.Error("physicalConnection run acceptConn err : ", err)
									}
								}
							}
						} else {
							log.Error("no time map ", uuid.String())
							pc.handshake(ChainCoord)
						}

					}

				}

			} else if body[0] == REPEAT {
				pc.handshakeResponse(ChainCoord, body[1:len(body)])
			}

		} else {
			pc.sendToLogicalConn(body, ChainCoord)
		}
	}
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

	pc.writeLock.Lock()
	defer pc.writeLock.Unlock()

	if pc.PConn == nil {
		return wrote, ErrNotConnected
	}

	if n, err := util.WriteUint8(pc.PConn, MAGICWORD); err == nil {
		wrote += n
	} else {
		return wrote, err
	}

	if n, err := ChainCoord.WriteTo(pc.PConn); err == nil {
		wrote += int64(n)
	} else {
		return wrote, err
	}

	if n, err := util.WriteUint8(pc.PConn, compression); err == nil {
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
	if n, err := util.WriteUint32(pc.PConn, uint32(size)); err == nil {
		wrote += n
	} else {
		return wrote, err
	}

	if n, err := pc.PConn.Write(body); err == nil {
		wrote += int64(n)
	} else {
		return wrote, err
	}

	checksum := crc32.Checksum(body, IEEETable)

	if n, err := util.WriteUint32(pc.PConn, checksum); err == nil {
		wrote += n
	} else {
		return wrote, err
	}

	return wrote, nil
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
	pc.lConn.lock()
	defer pc.lConn.unlock()

	if lConn, has := pc.lConn.load(*ChainCoord); has {
		lConn.sendToLogical(bs)
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
			pc.lConn.delete(*ChainCoord)
		}(closeChan, ChainCoord)
	}
	new := !has
	return l, new
}

func (pc *physicalConnection) handshakeByte(ChainCoord *common.Coordinate, body []byte) (result []byte) {
	result = make([]byte, 0, 13) // 1+6+1+4+1

	result = append(result, HANDSHAKE)
	result = append(result, ChainCoord.Bytes()...)
	result = append(result, UNCOMPRESSED)

	BNum := make([]byte, 4)
	binary.LittleEndian.PutUint32(BNum, uint32(len(body)))
	result = append(result, BNum...)
	result = append(result, body...)
	return
}

func (pc *physicalConnection) handshakeResponse(ChainCoord *common.Coordinate, body []byte) (wrote int64, err error) {
	if pc == nil || pc.PConn == nil {
		return wrote, ErrNotConnected
	}

	_, has := pc.lConn.load(*ChainCoord)

	body = append([]byte{FORONE}, body...)
	if !has {
		id, _ := uuid.NewV1()

		now := time.Now()
		pc.handshakeTimeMap.Store(id.String(), now)
		body = append(body, id.Bytes()...)
	}

	return pc._handshake(ChainCoord, body)
}

func (pc *physicalConnection) handshake(ChainCoord *common.Coordinate) (wrote int64, err error) {
	if pc == nil || pc.PConn == nil {
		return wrote, ErrNotConnected
	}

	_, has := pc.lConn.load(*ChainCoord)

	if !has {
		id, _ := uuid.NewV1()
		now := time.Now()
		pc.handshakeTimeMap.Store(id.String(), now)

		bs := append([]byte{REPEAT}, id.Bytes()...)
		return pc._handshake(ChainCoord, bs)
	}

	return 0, nil
}

func (pc *physicalConnection) _handshake(ChainCoord *common.Coordinate, body []byte) (wrote int64, err error) {
	hs := pc.handshakeByte(ChainCoord, body)
	go func(hs []byte, ChainCoord *common.Coordinate) {
		pc.writeLock.Lock()
		log.Debug("handshake ", hs[:12], " ", body, " ", pc.localhost, " ", pc.PConn.RemoteAddr().String())
		if n, err := pc.PConn.Write(hs); err == nil {
			wrote = int64(n)
		}
		pc.writeLock.Unlock()
	}(hs, ChainCoord)

	return wrote, nil
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
