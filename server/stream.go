package server

import (
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/rtp/rtp"
	"net"
	"sync"
	"sync/atomic"
	"unsafe"
)

type Stream interface {
	Handler() Handler

	SetHandler(handler Handler) Stream

	OnLossPacket() func(stream Stream, loss int)

	SetOnLossPacket(onLossPacket func(stream Stream, loss int)) Stream

	SSRC() int64

	SetSSRC(ssrc int64) Stream

	LocalAddr() net.Addr

	RemoteAddr() net.Addr

	SetRemoteAddr(addr net.Addr) Stream

	Send(layer *rtp.Layer) error

	Close()

	Handler

	log.LoggerProvider
}

type abstractStream[S TCPServer | UDPServer, A net.TCPAddr | net.UDPAddr] struct {
	self   Stream
	server atomic.Pointer[S]

	handler    atomic.Pointer[Handler]
	ssrc       atomic.Int64
	localAddr  *A
	remoteAddr atomic.Pointer[A]
	addrID     string

	isInit       bool
	seq          uint16
	onLossPacket atomic.Pointer[func(stream Stream, loss int)]

	mu sync.Mutex

	log.AtomicLogger
}

func (s *abstractStream[S, A]) Handler() Handler {
	return *s.handler.Load()
}

func (s *abstractStream[S, A]) SetHandler(handler Handler) Stream {
	s.handler.Store(&handler)
	return s.self
}

func (s *abstractStream[S, A]) OnLossPacket() func(stream Stream, loss int) {
	return *s.onLossPacket.Load()
}

func (s *abstractStream[S, A]) SetOnLossPacket(onLossPacket func(stream Stream, loss int)) Stream {
	s.onLossPacket.Store(&onLossPacket)
	return s.self
}

func (s *abstractStream[S, A]) SSRC() int64 {
	return s.ssrc.Load()
}

func (s *abstractStream[S, A]) SetSSRC(ssrc int64) Stream {
	s.ssrc.Store(ssrc)
	return s.self
}

func (s *abstractStream[S, A]) LocalAddr() net.Addr {
	return any(s.localAddr).(net.Addr)
}

func (s *abstractStream[S, A]) RemoteAddr() net.Addr {
	return any(s.remoteAddr.Load()).(net.Addr)
}

func (s *abstractStream[S, A]) SetRemoteAddr(addr net.Addr) Stream {
	if rAddr, is := any(addr).(*A); is {
		s.remoteAddr.Store(rAddr)
	}
	return s
}

func (s *abstractStream[S, A]) Send(layer *rtp.Layer) error {
	panic("not implement")
}

func (s *abstractStream[S, A]) Close() {
	server := s.server.Load()
	if server == nil {
		return
	}
	any(server).(Server).RemoveStream(s.self)
}

func (s *abstractStream[S, A]) HandlePacket(stream Stream, packet *rtp.Packet) {
	addr, is := any(packet.Addr).(*A)
	if !is {
		return
	}
	var uAddr = (*net.UDPAddr)(unsafe.Pointer(addr))
	var rAddr *A
	layer := packet.Layer
	if !s.remoteAddr.CompareAndSwap(nil, addr) {
		rAddr = s.remoteAddr.Load()
		ruAddr := (*net.UDPAddr)(unsafe.Pointer(rAddr))
		if !ruAddr.IP.Equal(uAddr.IP) ||
			ruAddr.Port != uAddr.Port ||
			ruAddr.Zone != uAddr.Zone {
			// packet addr not matched, dropped
			packet.Release()
			return
		}
	}
	if !s.ssrc.CompareAndSwap(-1, int64(layer.SSRC())) {
		ssrc := s.ssrc.Load()
		if ssrc != int64(layer.SSRC()) {
			packet.Release()
			return
		}
	}
	// count rtp packet loss
	if !s.isInit {
		s.isInit = true
	} else {
		if diff := packet.SequenceNumber() - s.seq; diff != 1 {
			if onLossPacket := s.self.OnLossPacket(); onLossPacket != nil {
				onLossPacket(s, int(diff-1))
			}
		}
	}
	s.seq = packet.SequenceNumber()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.self.Handler().HandlePacket(stream, packet)
}

func (s *abstractStream[S, A]) OnStreamClosed(stream Stream) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.self.Handler().OnStreamClosed(stream)
}
