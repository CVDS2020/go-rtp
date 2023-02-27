package server

import (
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/rtp/rtp"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type Stream interface {
	Handler() Handler

	SetHandler(handler Handler) Stream

	SSRC() int64

	SetSSRC(ssrc int64) Stream

	LocalAddr() net.Addr

	RemoteAddr() net.Addr

	SetRemoteAddr(addr net.Addr) Stream

	Timeout() time.Duration

	SetTimeout(timeout time.Duration) Stream

	GetOnTimeout() func(Stream)

	SetOnTimeout(onTimeout func(Stream)) Stream

	OnLossPacket() func(stream Stream, loss int)

	SetOnLossPacket(onLossPacket func(stream Stream, loss int)) Stream

	CloseConn() bool

	SetCloseConn(enable bool) Stream

	Send(layer *rtp.Layer) error

	Close()

	Handler

	log.LoggerProvider
}

type abstractStream[S TCPServer | UDPServer, A net.TCPAddr | net.UDPAddr] struct {
	self   Stream
	server atomic.Pointer[S]

	handler     atomic.Pointer[Handler]
	handlerLock sync.Mutex

	ssrc       atomic.Int64
	localAddr  *A
	remoteAddr atomic.Pointer[A]
	addrID     string

	timer timeoutTimer

	isInit bool
	closed bool
	seq    uint16

	onTimeout    atomic.Pointer[func(Stream)]
	onLossPacket atomic.Pointer[func(stream Stream, loss int)]

	closeConn atomic.Bool

	log.AtomicLogger
}

func (s *abstractStream[S, A]) init(self Stream, server *S, localAddr *A, remoteAddr net.Addr, ssrc int64, handler Handler, options ...Option) {
	s.self = self
	s.server.Store(server)
	s.localAddr = localAddr
	self.SetRemoteAddr(remoteAddr)
	self.SetSSRC(ssrc)
	self.SetHandler(handler)
	self.SetCloseConn(true)
	for _, option := range options {
		option.apply(s)
	}
}

func (s *abstractStream[S, A]) setTimeout(timeout time.Duration) {
	s.timer.setTimeout(timeout)
}

func (s *abstractStream[S, A]) setOnTimeout(onTimeout func(Stream)) {
	s.onTimeout.Store(&onTimeout)
}

func (s *abstractStream[S, A]) setOnLossPacket(onLossPacket func(stream Stream, loss int)) {
	s.onLossPacket.Store(&onLossPacket)
}

func (s *abstractStream[S, A]) setCloseConn(enable bool) {
	s.closeConn.Store(enable)
}

func (s *abstractStream[S, A]) Handler() Handler {
	if handler := s.handler.Load(); handler != nil {
		return *handler
	}
	return nil
}

func (s *abstractStream[S, A]) SetHandler(handler Handler) Stream {
	s.handler.Store(&handler)
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

func (s *abstractStream[S, A]) Timeout() time.Duration {
	return s.timer.Timeout()
}

func (s *abstractStream[S, A]) SetTimeout(timeout time.Duration) Stream {
	s.timer.SetTimeout(timeout)
	return s
}

func (s *abstractStream[S, A]) GetOnTimeout() func(Stream) {
	if onTimeout := s.onTimeout.Load(); onTimeout != nil {
		return *onTimeout
	}
	return nil
}

func (s *abstractStream[S, A]) SetOnTimeout(onTimeout func(Stream)) Stream {
	s.setOnTimeout(onTimeout)
	return s
}

func (s *abstractStream[S, A]) OnLossPacket() func(stream Stream, loss int) {
	if onLossPacket := s.onLossPacket.Load(); onLossPacket != nil {
		return *onLossPacket
	}
	return nil
}

func (s *abstractStream[S, A]) SetOnLossPacket(onLossPacket func(stream Stream, loss int)) Stream {
	s.setOnLossPacket(onLossPacket)
	return s.self
}

func (s *abstractStream[S, A]) CloseConn() bool {
	return s.closeConn.Load()
}

func (s *abstractStream[S, A]) SetCloseConn(enable bool) Stream {
	s.setCloseConn(enable)
	return s
}

func (s *abstractStream[S, A]) Send(layer *rtp.Layer) error {
	panic("not implement")
}

func (s *abstractStream[S, A]) Close() {
	if server := s.server.Load(); server == nil {
		any(server).(Server).RemoveStream(s.self)
	}
}

func (s *abstractStream[S, A]) HandlePacket(stream Stream, packet *rtp.Packet) (dropped, keep bool) {
	addr, is := any(packet.Addr).(*A)
	if !is {
		return true, true
	}
	return lock.LockGetDouble(&s.handlerLock, func() (dropped, keep bool) {
		if s.closed {
			return true, false
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
				return true, true
			}
		}
		if !s.ssrc.CompareAndSwap(-1, int64(layer.SSRC())) {
			ssrc := s.ssrc.Load()
			if ssrc != int64(layer.SSRC()) {
				packet.Release()
				return true, true
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
		if handler := s.self.Handler(); handler != nil {
			return handler.HandlePacket(stream, packet)
		}
		return true, true
	})
}

func (s *abstractStream[S, A]) OnParseError(stream Stream, err error) (keep bool) {
	return lock.LockGet(&s.handlerLock, func() bool {
		if s.closed {
			return false
		}
		if handler := s.self.Handler(); handler != nil {
			return handler.OnParseError(stream, err)
		}
		return true
	})
}

func (s *abstractStream[S, A]) OnStreamClosed(stream Stream) {
	lock.LockDo(&s.handlerLock, func() {
		if s.closed {
			return
		}
		s.closed = true
		if handler := s.self.Handler(); handler != nil {
			handler.OnStreamClosed(stream)
		}
	})
}
