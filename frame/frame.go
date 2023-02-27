package frame

import (
	"encoding/binary"
	"fmt"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"gitee.com/sy_183/rtp/utils"
	"io"
	"sync/atomic"
	"time"
)

type Layer struct {
	Timestamp   uint32
	PayloadType uint8
	RTPLayers   []*rtp.Layer
}

func (f *Layer) Len() uint {
	return uint(len(f.RTPLayers))
}

func (f *Layer) Append(layer *rtp.Layer) {
	if len(f.RTPLayers) == 0 {
		f.Timestamp = layer.Timestamp()
		f.PayloadType = layer.PayloadType()
	} else if f.Timestamp != layer.Timestamp() || f.PayloadType != layer.PayloadType() {
		return
	}
	f.RTPLayers = append(f.RTPLayers, layer)
}

func (f *Layer) Clear() {
	f.RTPLayers = f.RTPLayers[:0]
	f.Timestamp = 0
	f.PayloadType = 0
}

type FullLayerWriter struct {
	Layer *Layer
}

func (f FullLayerWriter) Size() uint {
	var s uint
	for _, layer := range f.Layer.RTPLayers {
		s += layer.Size() + 2
	}
	return s
}

func (f FullLayerWriter) WriteTo(w io.Writer) (n int64, err error) {
	buf := [2]byte{}
	for _, layer := range f.Layer.RTPLayers {
		binary.BigEndian.PutUint16(buf[:], uint16(layer.Size()))
		if err = utils.Write(w, buf[:], &n); err != nil {
			return
		}
		if err = utils.WriteTo(layer, w, &n); err != nil {
			return
		}
	}
	return
}

func (f FullLayerWriter) Read(p []byte) (n int, err error) {
	w := utils.Writer{Buf: p}
	n64, _ := f.WriteTo(&w)
	return int(n64), io.EOF
}

type PayloadLayerWriter struct {
	Layer *Layer
}

func (f PayloadLayerWriter) Size() uint {
	var s uint
	for _, layer := range f.Layer.RTPLayers {
		s += layer.PayloadLength()
	}
	return s
}

func (f PayloadLayerWriter) WriteTo(w io.Writer) (n int64, err error) {
	for _, layer := range f.Layer.RTPLayers {
		if err = utils.WriteTo(&layer.PayloadContent, w, &n); err != nil {
			return
		}
	}
	return
}

func (f PayloadLayerWriter) Read(p []byte) (n int, err error) {
	w := utils.Writer{Buf: p}
	n64, _ := f.WriteTo(&w)
	return int(n64), io.EOF
}

type Frame struct {
	*Layer
	Packets []*rtp.Packet
	Start   time.Time
	End     time.Time
	ref     int64
	pool    *pool.SyncPool[*Frame]
}

func timeRangeExpand(start, end *time.Time, cur time.Time, timeNodeLen int) {
	if start.After(*end) {
		panic("start time after end time")
	}
	if start.IsZero() {
		*start = cur
		*end = cur
	} else {
		du := cur.Sub(*end)
		if du < 0 {
			guess := time.Duration(0)
			if timeNodeLen > 2 {
				guess = (end.Sub(*start)) / time.Duration(timeNodeLen-2)
			}
			offset := du - guess
			*start = start.Add(offset)
		}
		*end = cur
	}
}

func (f *Frame) StartTime() time.Time {
	return f.Start
}

func (f *Frame) EndTime() time.Time {
	return f.End
}

func (f *Frame) KeyFrame() bool {
	return false
}

func (f *Frame) Append(packet *rtp.Packet) {
	f.Layer.Append(packet.Layer)
	f.Packets = append(f.Packets, packet.Use())
	timeRangeExpand(&f.Start, &f.End, packet.Time, len(f.Packets))
}

var zeroTime = time.Time{}

func (f *Frame) Clear() {
	f.Layer.Clear()
	for _, packet := range f.Packets {
		packet.Release()
	}
	f.Packets = f.Packets[:0]
	f.Start, f.End = zeroTime, zeroTime
}

func (f *Frame) Release() {
	if c := atomic.AddInt64(&f.ref, -1); c == 0 {
		f.Clear()
		if f.pool != nil {
			f.pool.Put(f)
		}
	} else if c < 0 {
		panic(fmt.Errorf("frame repeat release, ref [%d -> %d]", c+1, c))
	}
}

func (f *Frame) AddRef() {
	if c := atomic.AddInt64(&f.ref, 1); c <= 0 {
		panic(fmt.Errorf("invalid frame reference, ref [%d -> %d]", c+1, c))
	}
}

func (f *Frame) Use() *Frame {
	f.AddRef()
	return f
}
