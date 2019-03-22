package router

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/fletaio/common/hash"

	"github.com/fletaio/common"
	"github.com/fletaio/common/util"
	"github.com/fletaio/framework/message"
)

//type define
var (
	HandshakeType = message.DefineType("handshake")
)

type handshake struct {
	RemoteAddr string
	ChainCoord *common.Coordinate
	Address    string
	Port       uint16
	Time       uint64
}

// WriteTo is a serialization function
func (h *handshake) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	if n, err := util.WriteUint64(w, uint64(HandshakeType)); err != nil {
		return wrote, err
	} else {
		wrote += n
	}

	if n, err := util.WriteString(w, h.RemoteAddr); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := h.ChainCoord.WriteTo(w); err != nil {
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
	if v, n, err := util.ReadUint64(r); err != nil {
		return read, err
	} else {
		read += n
		if v != uint64(HandshakeType) {
			return read, ErrNotHandshakeFormate
		}
	}
	if v, n, err := util.ReadString(r); err != nil {
		return read, err
	} else {
		read += n
		h.RemoteAddr = v
	}

	h.ChainCoord = &common.Coordinate{}
	if n, err := h.ChainCoord.ReadFrom(r); err != nil {
		return read, err
	} else {
		read += n
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

func (pc *RouterConn) handshakeSend(ChainCoord *common.Coordinate) {
	h := &handshake{
		RemoteAddr: pc.RemoteAddr().String(),
		ChainCoord: pc.r.chainCoord(),
		Address:    pc.r.localAddress(),
		Port:       uint16(pc.r.port()),
		Time:       uint64(time.Now().UnixNano()),
	}
	bf := &bytes.Buffer{}
	h.WriteTo(bf)

	pc.write(bf.Bytes(), UNCOMPRESSED)
}

func (pc *RouterConn) handshakeRecv() (*common.Coordinate, error) {
	body, err := pc.ReadConn()
	if err != nil {
		return nil, err
	}

	bf := bytes.NewBuffer(body)
	h := &handshake{}
	_, err = h.ReadFrom(bf)
	if err != nil {
		return nil, err
	}

	if h.Address == "" {
		addr := pc.RemoteAddr().String()
		if raddr, ok := pc.RemoteAddr().(*net.TCPAddr); ok {
			addr = raddr.IP.String()
		} else {
			addrs := strings.Split(addr, ":")
			addr = strings.Join(addrs[:len(addrs)-1], ":")
		}

		pc.Address = fmt.Sprintf("%v:%v", addr, h.Port)
	} else {
		pc.Address = fmt.Sprintf("%v:%v", h.Address, h.Port)
	}
	pc.pingTime = time.Now().Sub(time.Unix(0, int64(h.Time)))
	if err != nil {
		return nil, err
	}

	return h.ChainCoord, nil
}
