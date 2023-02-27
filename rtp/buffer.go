package rtp

import (
	"fmt"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/unit"
	"sync/atomic"
)

var DebugLogger = assert.Must(log.Config{
	Level: log.NewAtomicLevelAt(log.DebugLevel),
	Encoder: log.NewConsoleEncoder(log.ConsoleEncoderConfig{
		EncodeLevel: log.CapitalColorLevelEncoder,
	}),
}.Build())

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
	if c == 0 {
		b.buffer = b.raw
		b.pool.pool.Put(b)
	} else if c < 0 {
		panic(fmt.Errorf("repeat release buffer(%p), ref [%d -> %d]", b, c+1, c))
	}
}

func (b *Buffer) AddRef() {
	c := b.ref.Add(1)
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
	pool *pool.SyncPool[*Buffer]
}

var makeCount atomic.Int64
var useCount atomic.Int64
var freeCount atomic.Int64

func NewBufferPool(size uint) BufferPool {
	p := BufferPool{}
	p.pool = pool.NewSyncPool(func(_ *pool.SyncPool[*Buffer]) *Buffer {
		//DebugLogger.Error("make data", log.Uint("size", size))
		raw := make([]byte, size)
		b := &Buffer{
			raw:    raw,
			buffer: raw,
			pool:   p,
		}
		return b
	})
	return p
}

func (p BufferPool) Get() *Buffer {
	b := p.pool.Get().Use()
	return b
}
