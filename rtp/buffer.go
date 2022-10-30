package rtp

import (
	"fmt"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/unit"
	"sync/atomic"
)

const (
	DefaultBufferPoolSize  = unit.KiBiByte * 256
	DefaultBufferReverse   = 2048
	DefaultWriteBufferSize = 2048
)

type Buffer struct {
	raw    []byte
	buffer []byte
	ref    atomic.Int64
	pool   BufferPool
}

func (b *Buffer) Get() []byte {
	return b.buffer
}

func (b *Buffer) Alloc(size uint) *Data {
	if size > uint(len(b.buffer)) {
		return &Data{Data: make([]byte, size)}
	}
	d := &Data{
		Data:   b.buffer[:size],
		buffer: b,
	}
	b.AddRef()
	b.buffer = b.buffer[size:]
	return d
}

func (b *Buffer) Release() {
	c := b.ref.Add(-1)
	//println(fmt.Sprintf("release buffer(%p) [%d -> %d]", b, c+1, c))
	if c == 0 {
		b.buffer = b.raw
		b.pool.pool.Put(b)
		//logger.Logger().Debug("free buffer", log.Int64("count", freeCount.Add(1)), log.Uintptr("buffer", uintptr(unsafe.Pointer(b))))
	} else if c < 0 {
		panic(fmt.Errorf("repeat release buffer(%p), ref [%d -> %d]", b, c+1, c))
	}
}

func (b *Buffer) AddRef() {
	c := b.ref.Add(1)
	//println(fmt.Sprintf("use buffer(%p) [%d -> %d]", b, c-1, c))
	if c <= 0 {
		panic(fmt.Errorf("invalid buffer(%p) reference, ref [%d -> %d]", b, c-1, c))
	}
}

func (b *Buffer) Use() *Buffer {
	b.AddRef()
	return b
}

type Data struct {
	Data   []byte
	buffer *Buffer
}

func (d *Data) Release() {
	if d.buffer != nil {
		d.buffer.Release()
	}
}

func (d *Data) AddRef() {
	if d.buffer != nil {
		d.buffer.AddRef()
	}
}

func (d *Data) Use() *Data {
	d.AddRef()
	return d
}

type BufferPool struct {
	pool *pool.Pool[*Buffer]
}

var makeCount atomic.Int64
var useCount atomic.Int64
var freeCount atomic.Int64

func NewBufferPool(size uint) BufferPool {
	p := BufferPool{}
	p.pool = pool.NewPool(func(_ *pool.Pool[*Buffer]) *Buffer {
		raw := make([]byte, size)
		b := &Buffer{
			raw:    raw,
			buffer: raw,
			pool:   p,
		}
		//logger.Logger().Debug("make buffer", log.Int64("count", makeCount.Add(1)), log.Uintptr("buffer", uintptr(unsafe.Pointer(b))))
		return b
	})
	return p
}

func (p BufferPool) Get() *Buffer {
	//fmt.Println("alloc new buffer")
	b := p.pool.Get().Use()
	//logger.Logger().Debug("use buffer", log.Int64("count", useCount.Add(1)), log.Uintptr("buffer", uintptr(unsafe.Pointer(b))))
	return b
}
