package frame

import (
	"fmt"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"gitee.com/sy_183/rtp/utils"
	"io"
	"sync/atomic"
	"time"
)

type FrameLayer struct {
	Timestamp uint32
	Layers    []*rtp.Layer
}

func (f *Frame) Len() uint {
	return uint(len(f.Layers))
}

func (f *Frame) Size() uint {
	var s uint
	for _, layer := range f.Layers {
		s += layer.PayloadLength()
	}
	return s
}

func (l *FrameLayer) Append(layer *rtp.Layer) {
	l.Layers = append(l.Layers, layer)
}

func (l *FrameLayer) WriteTo(w io.Writer) (n int64, err error) {
	for _, layer := range l.Layers {
		if err = utils.WriteTo(&layer.PayloadContent, w, &n); err != nil {
			return
		}
	}
	return
}

func (l *FrameLayer) Read(p []byte) (n int, err error) {
	w := utils.Writer{Buf: p}
	n64, _ := l.WriteTo(&w)
	return int(n64), io.EOF
}

type Frame struct {
	FrameLayer
	Packets []*rtp.Packet
	Start   time.Time
	End     time.Time
	ref     int64
	pool    *pool.Pool[*Frame]
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
	if len(f.Packets) == 0 {
		f.Timestamp = packet.Timestamp()
	} else if packet.Timestamp() != f.Timestamp {
		return
	}
	f.FrameLayer.Append(packet.Layer)
	f.Packets = append(f.Packets, packet.Use())
	timeRangeExpand(&f.Start, &f.End, packet.Time, len(f.Packets))
}

var zeroTime = time.Time{}

func (f *Frame) Release() {
	if c := atomic.AddInt64(&f.ref, -1); c == 0 {
		f.Layers = f.Layers[:0]
		for _, packet := range f.Packets {
			packet.Release()
		}
		f.Packets = f.Packets[:0]
		f.Timestamp = 0
		f.Start, f.End = zeroTime, zeroTime
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
