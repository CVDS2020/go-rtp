package rtp

import (
	"fmt"
	"gitee.com/sy_183/common/pool"
	"net"
	"sync/atomic"
	"time"
)

type Packet struct {
	*Layer
	Addr   net.Addr
	Time   time.Time
	Chunks []*pool.Data
	ref    atomic.Int64
	pool   pool.Pool[*Packet]
}

type PacketSetter func(packet *Packet)

func PacketAddr(addr net.Addr) PacketSetter {
	return func(packet *Packet) {
		packet.Addr = addr
	}
}

func PacketTime(time time.Time) PacketSetter {
	return func(packet *Packet) {
		packet.Time = time
	}
}

func PacketData(data *pool.Data) PacketSetter {
	return func(packet *Packet) {
		packet.Chunks = append(packet.Chunks[:0], data)
	}
}

func PacketChunks(chunks []*pool.Data) PacketSetter {
	return func(packet *Packet) {
		packet.Chunks = append(packet.Chunks[:0], chunks...)
	}
}

func PacketPool(pool pool.Pool[*Packet]) PacketSetter {
	return func(packet *Packet) {
		packet.pool = pool
	}
}

func NewPacket(layer *Layer, setters ...PacketSetter) *Packet {
	p := &Packet{Layer: layer}
	for _, setter := range setters {
		setter(p)
	}
	return p
}

func (p *Packet) Clear() {
	for _, chunk := range p.Chunks {
		chunk.Release()
	}
	p.Addr = nil
	p.Time = time.Time{}
	p.Chunks = p.Chunks[:0]
}

func (p *Packet) Release() {
	if c := p.ref.Add(-1); c == 0 {
		p.Clear()
		if p.pool != nil {
			p.pool.Put(p)
		}
	} else if c < 0 {
		panic(fmt.Errorf("repeat release packet, ref [%d -> %d]", c+1, c))
	}
}

func (p *Packet) AddRef() {
	if c := p.ref.Add(1); c <= 0 {
		panic(fmt.Errorf("invalid packet reference, ref [%d -> %d]", c-1, c))
	}
}

func (p *Packet) Use() *Packet {
	p.AddRef()
	return p
}
