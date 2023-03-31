package rtp

import (
	"encoding/binary"
	"gitee.com/sy_183/common/errors"
	"io"
)

type Reader struct {
	Reader io.Reader
}

func (r *Reader) Read() (layer *IncomingLayer, err error) {
	buf := make([]byte, 2)
	p := buf
	var length uint16
	for {
		n, e := r.Reader.Read(p)
		err = e
		if n > 0 {
			p = p[n:]
			if len(p) == 0 {
				if len(buf) == 2 {
					length = binary.BigEndian.Uint16(buf)
					if length < 12 {
						return nil, errors.Append(err, PacketSizeNotEnoughError)
					}
					buf = make([]byte, length)
					buf = buf[:length]
					p = buf
				} else {
					layer = NewIncomingLayer()
					if e := layer.Unmarshal(buf); e != nil {
						return nil, errors.Append(err, e)
					}
					return layer, err
				}
			}
		}
		if e != nil {
			return nil, err
		}
	}
}
