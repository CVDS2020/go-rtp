package server

import (
	"gitee.com/sy_183/common/pool"
	"sync/atomic"
)

type BufferPool struct {
	bufferPool       atomic.Pointer[pool.BufferPool]
	globalBufferPool atomic.Pointer[pool.Pool[*pool.Buffer]]
	curBuffer        *pool.Buffer
	cureBufferPool   pool.BufferPool
}

func (p *BufferPool) SetBufferPool(bufferPool pool.BufferPool) {
	p.bufferPool.Store(&bufferPool)
}

func (p *BufferPool) BufferPool() pool.BufferPool {
	if bufferPool := p.bufferPool.Load(); bufferPool != nil {
		return *bufferPool
	}
	return nil
}

func (p *BufferPool) SetGlobalBufferPool(bufferPool pool.Pool[*pool.Buffer]) {
	p.globalBufferPool.Store(&bufferPool)
}

func (p *BufferPool) GlobalBufferPool() pool.Pool[*pool.Buffer] {
	if bufferPool := p.globalBufferPool.Load(); bufferPool != nil {
		return *bufferPool
	}
	return nil
}

func (p *BufferPool) Get() (buf []byte) {
	if p.curBuffer != nil {
		buf = p.curBuffer.Get()
	} else if p.cureBufferPool = p.BufferPool(); p.cureBufferPool != nil {
		buf = p.cureBufferPool.Get()
	}
	if buf == nil {
		if p.curBuffer != nil {
			p.curBuffer.Release()
			p.curBuffer = nil
		} else if p.cureBufferPool != nil {
			p.cureBufferPool = nil
		}
		if globalBufferPool := p.GlobalBufferPool(); globalBufferPool != nil {
			if p.curBuffer = globalBufferPool.Get(); p.curBuffer != nil {
				buf = p.curBuffer.Use().Get()
			}
		}
	}
	return
}

func (p *BufferPool) Alloc(size uint) *pool.Data {
	if p.curBuffer != nil {
		return p.curBuffer.Alloc(size)
	} else if p.cureBufferPool != nil {
		return p.cureBufferPool.Alloc(size)
	}
	return nil
}
