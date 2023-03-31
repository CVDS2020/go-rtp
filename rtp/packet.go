package rtp

import (
	"gitee.com/sy_183/common/pool"
	"net"
	"time"
)

type Packet interface {
	Layer

	AddRelation(relation pool.Reference)

	Clear()

	pool.Reference
}

func UsePacket(packet Packet) Packet {
	packet.AddRef()
	return packet
}

type IncomingPacket struct {
	*IncomingLayer
	addr      net.Addr
	time      time.Time
	relations pool.Relations
	pool      pool.Pool[*IncomingPacket]
}

func NewIncomingPacket(layer *IncomingLayer, pool pool.Pool[*IncomingPacket]) *IncomingPacket {
	return &IncomingPacket{IncomingLayer: layer, pool: pool}
}

func ProvideIncomingPacket(p pool.Pool[*IncomingPacket]) *IncomingPacket {
	return NewIncomingPacket(NewIncomingLayer(), p)
}

func (p *IncomingPacket) Addr() net.Addr {
	return p.addr
}

func (p *IncomingPacket) SetAddr(addr net.Addr) {
	p.addr = addr
}

func (p *IncomingPacket) Time() time.Time {
	return p.time
}

func (p *IncomingPacket) SetTime(t time.Time) {
	p.time = t
}

func (p *IncomingPacket) AddRelation(relation pool.Reference) {
	p.relations.AddRelation(relation)
}

func (p *IncomingPacket) Clear() {
	p.relations.Clear()
	p.addr = nil
	p.time = time.Time{}
}

func (p *IncomingPacket) Release() bool {
	if p.relations.Release() {
		p.Clear()
		if p.pool != nil {
			p.pool.Put(p)
		}
		return true
	}
	return false
}

func (p *IncomingPacket) AddRef() {
	p.relations.AddRef()
}

func (p *IncomingPacket) Use() *IncomingPacket {
	p.AddRef()
	return p
}

type DefaultPacket struct {
	DefaultLayer
	relations pool.Relations
	pool      pool.Pool[*DefaultPacket]
}

func NewDefaultPacket(pool pool.Pool[*DefaultPacket], payload Payload) *DefaultPacket {
	p := &DefaultPacket{pool: pool}
	p.Init(payload)
	return p
}

func DefaultPacketProvider(payloadProvider func() Payload) func(pool pool.Pool[*DefaultPacket]) *DefaultPacket {
	return func(pool pool.Pool[*DefaultPacket]) *DefaultPacket {
		return NewDefaultPacket(pool, payloadProvider())
	}
}

func (p *DefaultPacket) AddRelation(relation pool.Reference) {
	p.relations.AddRelation(relation)
}

func (p *DefaultPacket) Clear() {
	p.relations.Clear()
	p.Header.Clear()
	if p.payload != nil {
		p.payload.Clear()
	}
}

func (p *DefaultPacket) Release() bool {
	if p.relations.Release() {
		p.Clear()
		if p.pool != nil {
			p.pool.Put(p)
		}
		return true
	}
	return false
}

func (p *DefaultPacket) AddRef() {
	p.relations.AddRef()
}

func (p *DefaultPacket) Use() *DefaultPacket {
	p.AddRef()
	return p
}
