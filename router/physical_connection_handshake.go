package router

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"git.fleta.io/fleta/common/hash"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/common/util"
	"git.fleta.io/fleta/framework/log"
)

type handshake struct {
	RemoteAddr string
	Address    string
	Port       uint16
	Time       uint64
}

// WriteTo is a serialization function
func (h *handshake) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	if n, err := util.WriteString(w, h.RemoteAddr); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := util.WriteString(w, h.Address); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := util.WriteUint16(w, h.Port); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := util.WriteUint64(w, h.Time); err != nil {
		return wrote, err
	} else {
		wrote += n
	}

	return wrote, nil
}

// ReadFrom is a deserialization function
func (h *handshake) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	if v, n, err := util.ReadString(r); err != nil {
		return read, err
	} else {
		read += n
		h.RemoteAddr = v
	}
	if v, n, err := util.ReadString(r); err != nil {
		return read, err
	} else {
		read += n
		h.Address = v
	}
	if v, n, err := util.ReadUint16(r); err != nil {
		return read, err
	} else {
		read += n
		h.Port = v
	}
	if v, n, err := util.ReadUint64(r); err != nil {
		return read, err
	} else {
		read += n
		h.Time = v
	}

	return read, nil
}

func (h *handshake) hash() hash.Hash256 {
	bf := &bytes.Buffer{}
	h.WriteTo(bf)
	return hash.Hash(bf.Bytes())
}

func (pc *physicalConnection) handshakeProcess(body []byte, ChainCoord *common.Coordinate) (bool, error) {
	log.Debug("response handshake ", body[0], " ", pc.PConn.LocalAddr().String(), " ", pc.PConn.RemoteAddr().String())
	bf := bytes.NewBuffer(body)
	if bf.Len() == 0 {
		return false, ErrNotHandshakeFormate
	}
	h := &handshake{}
	h.ReadFrom(bf)
	if bf.Len() == 0 {
		pc.handshakeResponse(ChainCoord, body)
	} else if bf.Len() > 0 {
		h2 := &handshake{}
		h2.ReadFrom(bf)

		var targetH *handshake
		if h.RemoteAddr != pc.RemoteAddr().String() {
			targetH = h
		} else {
			targetH = h2
		}

		if targetH.Address == "" {
			addr := pc.RemoteAddr().String()
			if raddr, ok := pc.RemoteAddr().(*net.TCPAddr); ok {
				addr = raddr.IP.String()
			}
			pc.Address = fmt.Sprintf("%v:%v", addr, targetH.Port)
		} else {
			pc.Address = fmt.Sprintf("%v:%v", targetH.Address, targetH.Port)
		}

		pc.pingTime = time.Now().Sub(time.Unix(0, int64(targetH.Time)))
		if conn, new := pc.makeLogicalConnenction(ChainCoord, pc.pingTime); new == true {
			err := pc.r.acceptConn(conn, ChainCoord)
			if err != nil {
				log.Error("physicalConnection run acceptConn err : ", err)
			}
		}
		pc.handshakeResponse(ChainCoord, body)
		return true, nil
	}
	return false, nil
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

	pc.lConn.lock()
	defer pc.lConn.unlock()
	_, has := pc.lConn.load(*ChainCoord)

	bf := bytes.NewBuffer(body)
	if !has {
		h := &handshake{
			RemoteAddr: pc.RemoteAddr().String(),
			Address:    pc.r.localAddress(),
			Port:       uint16(pc.r.port()),
			Time:       uint64(time.Now().UnixNano()),
		}
		pc.handshakeTimeMap.Store(h.hash(), h)
		h.WriteTo(bf)
	}

	return pc._handshake(ChainCoord, bf.Bytes())
}

func (pc *physicalConnection) handshake(ChainCoord *common.Coordinate) (wrote int64, err error) {
	if pc == nil || pc.PConn == nil {
		return wrote, ErrNotConnected
	}

	pc.lConn.lock()
	defer pc.lConn.unlock()
	_, has := pc.lConn.load(*ChainCoord)

	if !has {
		h := &handshake{
			RemoteAddr: pc.RemoteAddr().String(),
			Address:    pc.r.localAddress(),
			Port:       uint16(pc.r.port()),
			Time:       uint64(time.Now().UnixNano()),
		}
		pc.handshakeTimeMap.Store(h.hash(), h)

		bf := &bytes.Buffer{}
		h.WriteTo(bf)

		return pc._handshake(ChainCoord, bf.Bytes())
	}

	return 0, nil
}

func (pc *physicalConnection) _handshake(ChainCoord *common.Coordinate, body []byte) (wrote int64, err error) {
	hs := pc.handshakeByte(ChainCoord, body)
	// go func(hs []byte, ChainCoord *common.Coordinate) {
	pc.writeLock.Lock()
	if n, err := pc.PConn.Write(hs); err == nil {
		wrote = int64(n)
	}
	pc.writeLock.Unlock()
	// }(hs, ChainCoord)

	return wrote, nil
}
