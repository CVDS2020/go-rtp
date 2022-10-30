package server

import (
	"fmt"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"net"
	"sync/atomic"
	"time"
	_ "unsafe"
)

type UDPPacket struct {
	UDPAddr *net.UDPAddr
	*rtp.Data
}

type UDPServer struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultRunner
	name   string

	listener atomic.Pointer[net.UDPConn]
	addr     *net.UDPAddr

	readBuffer  int
	writeBuffer int

	stream atomic.Pointer[UDPStream]

	bufferPool    rtp.BufferPool
	bufferReverse uint
	packetPool    *pool.Pool[*rtp.Packet]
	writePool     *pool.DataPool

	onError func(s Server, err error)

	log.AtomicLogger
}

func newUDPServer(listener *net.UDPConn, addr *net.UDPAddr, options ...Option) *UDPServer {
	var s *UDPServer
	if listener != nil {
		s = &UDPServer{
			addr: listener.LocalAddr().(*net.UDPAddr),
		}
		s.listener.Store(listener)
	} else {
		s = &UDPServer{
			addr: addr,
		}
	}
	s.packetPool = pool.NewPool(func(p *pool.Pool[*rtp.Packet]) *rtp.Packet {
		return rtp.NewPacket(rtp.NewLayer(), rtp.PacketPool(p))
	})
	for _, option := range options {
		option.apply(s)
	}
	if s.addr == nil {
		s.addr = &net.UDPAddr{IP: net.IP{0, 0, 0, 0}, Port: 5004}
	}
	if s.name == "" {
		s.name = fmt.Sprintf("rtp udp server(%s)", s.addr.String())
	}
	if s.bufferPool == (rtp.BufferPool{}) {
		s.bufferPool = rtp.NewBufferPool(rtp.DefaultBufferPoolSize)
	}
	if s.bufferReverse == 0 {
		s.bufferReverse = rtp.DefaultBufferReverse
	}
	if s.writePool == nil {
		s.writePool = pool.NewDataPool(rtp.DefaultWriteBufferSize)
	}
	if s.onError == nil {
		s.onError = func(s Server, err error) {
			if logger := s.Logger(); logger != nil {
				switch e := err.(type) {
				case *UDPListenError:
					logger.ErrorWith("listen udp addr error", err, log.String("addr", e.Addr.String()))
				case *SetReadBufferError:
					logger.ErrorWith("set udp socket read buffer error", err, log.Int("read buffer", e.ReadBuffer))
				case *SetWriteBufferError:
					logger.ErrorWith("set udp socket write buffer error", err, log.Int("write buffer", e.WriteBuffer))
				case *UDPReadError:
					logger.ErrorWith("udp server read error", err)
				}
			}
		}
	}
	s.runner, s.Lifecycle = lifecycle.New(s.name, lifecycle.Core(s.start, s.run, s.close))
	return s
}

func NewUDPServer(addr *net.UDPAddr, options ...Option) *UDPServer {
	return newUDPServer(nil, addr, options...)
}

func NewUDPServerWithListener(listener *net.UDPConn, options ...Option) *UDPServer {
	return newUDPServer(listener, nil, options...)
}

func UDPServerProvider(options ...Option) ServerProvider {
	return func(addr *net.IPAddr, port uint16, opts ...Option) Server {
		return NewUDPServer(&net.UDPAddr{
			IP:   addr.IP,
			Port: int(port),
			Zone: addr.Zone,
		}, append(opts, options...)...)
	}
}

func (s *UDPServer) setName(name string) {
	s.name = name
}

func (s *UDPServer) setSocketBuffer(readBuffer, writeBuffer int) {
	s.readBuffer, s.writeBuffer = readBuffer, writeBuffer
}

func (s *UDPServer) setBufferPool(bufferPool rtp.BufferPool) {
	s.bufferPool = bufferPool
}

func (s *UDPServer) setBufferReverse(reverse uint) {
	s.bufferReverse = reverse
}

func (s *UDPServer) setWritePool(pool *pool.DataPool) {
	s.writePool = pool
}

func (s *UDPServer) setOnError(onError func(s Server, err error)) {
	s.onError = onError
}

func (s *UDPServer) Name() string {
	return s.name
}

func (s *UDPServer) Addr() net.Addr {
	return s.addr
}

func (s *UDPServer) handleError(err error) error {
	if s.onError != nil {
		s.onError(s, err)
	}
	return err
}

func (s *UDPServer) start() error {
	listener := s.listener.Load()
	if listener == nil {
		var err error
		listener, err = net.ListenUDP("udp", s.addr)
		if err != nil {
			return &UDPListenError{Err: Err{err}, Addr: s.addr}
		}
		s.listener.Store(listener)
	}
	if s.readBuffer > 0 {
		if err := listener.SetReadBuffer(s.readBuffer); err != nil {
			s.handleError(&SetReadBufferError{Err: Err{err}, ReadBuffer: s.readBuffer})
		}
	}
	if s.writeBuffer > 0 {
		if err := listener.SetWriteBuffer(s.writeBuffer); err != nil {
			s.handleError(&SetWriteBufferError{Err: Err{err}, WriteBuffer: s.writeBuffer})
		}
	}
	return nil
}

func (s *UDPServer) run() error {
	//size := s.dataPool.Size()
	buffer := s.bufferPool.Get()
	defer func() {
		if stream := s.stream.Load(); stream != nil {
			stream.Close()
		}
		buffer.Release()
		s.listener.Store(nil)
	}()
	listener := s.listener.Load()
	for {
		buf := buffer.Get()
		if uint(len(buf)) < s.bufferReverse {
			buffer.Release()
			buffer = s.bufferPool.Get()
			buf = buffer.Get()
		}
		//data := s.dataPool.Alloc(size)
		n, addr, err := listener.ReadFromUDP(buf)
		if err != nil {
			if opErr, is := err.(*net.OpError); is && opErr.Err.Error() == "use of closed network connection" {
				return nil
			}
			return s.handleError(&UDPReadError{Err{err}})
		}
		data := buffer.Alloc(uint(n))

		packet := s.packetPool.Get().Use()
		if err := packet.Unmarshal(data.Data); err != nil {
			data.Release()
			packet.Release()
			continue
		}
		packet.Addr = addr
		packet.Time = time.Now()
		packet.Chunks = append(packet.Chunks[:0], data)

		if stream := s.stream.Load(); stream != nil {
			stream.HandlePacket(stream, packet)
		} else {
			packet.Release()
		}
	}
}

func (s *UDPServer) close() error {
	listener := s.listener.Load()
	if err := listener.Close(); err != nil {
		return &UDPCloseError{Err: Err{err}, Listener: listener}
	}
	return nil
}

func (s *UDPServer) Stream(remoteAddr net.Addr, ssrc int64, handler Handler) Stream {
	stream := &UDPStream{}
	stream.self = stream
	stream.localAddr = s.addr
	stream.server.Store(s)
	stream.SetRemoteAddr(remoteAddr)
	stream.SetSSRC(ssrc)
	stream.SetHandler(handler)
	stream.SetOnLossPacket(nil)
	stream.SetLogger(s.Logger())
	if old := s.stream.Swap(stream); old != nil {
		old.server.Store(nil)
		old.OnStreamClosed(stream)
	}
	return stream
}

func (s *UDPServer) RemoveStream(stream Stream) {
	if us, is := stream.(*UDPStream); is {
		if s.stream.CompareAndSwap(us, nil) {
			us.server.Store(nil)
			us.OnStreamClosed(us)
		}
	}
}

func (s *UDPServer) SendTo(layer *rtp.Layer, addr *net.UDPAddr) error {
	listener := s.listener.Load()
	if listener != nil {
		size := layer.Size()
		data := s.writePool.Alloc(size)
		layer.Read(data.Data)
		_, err := listener.WriteTo(data.Data, addr)
		data.Release()
		return err
	}
	return nil
}
