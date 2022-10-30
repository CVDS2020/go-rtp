package server

import (
	"fmt"
	"gitee.com/sy_183/common/id"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"net"
	"sync"
)

// TCPChannelCloseError 这个结构体放在其他文件会报错？
type TCPChannelCloseError struct {
	Err
	Channel *TCPChannel
}

type TCPServer struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultRunner
	name   string

	listener *net.TCPListener
	addr     *net.TCPAddr

	channelOptions []TCPChannelOption

	channels map[string]*TCPChannel
	streams  map[string]*TCPStream
	mu       sync.Mutex

	onError         func(s Server, err error)
	onAccept        func(s *TCPServer, conn *net.TCPConn) []TCPChannelOption
	onChannelClosed func(s *TCPServer, channel *TCPChannel)

	log.AtomicLogger
}

func newTCPServer(listener *net.TCPListener, addr *net.TCPAddr, options ...Option) *TCPServer {
	var s *TCPServer
	if listener != nil {
		s = &TCPServer{
			addr:     listener.Addr().(*net.TCPAddr),
			listener: listener,
		}
	} else {
		s = &TCPServer{
			addr: addr,
		}
	}
	s.channels = make(map[string]*TCPChannel)
	s.streams = make(map[string]*TCPStream)
	for _, option := range options {
		option.apply(s)
	}
	if s.addr == nil {
		s.addr = &net.TCPAddr{IP: net.IP{0, 0, 0, 0}, Port: 5004}
	}
	if s.name == "" {
		s.name = fmt.Sprintf("rtp tcp server(%s)", s.addr.String())
	}
	if s.onError == nil {
		s.onError = func(s Server, err error) {
			if logger := s.Logger(); logger != nil {
				switch e := err.(type) {
				case *TCPListenError:
					logger.ErrorWith("listen tcp addr error", err, log.String("addr", e.Addr.String()))
				case *TCPAcceptError:
					logger.ErrorWith("tcp server accept error", err)
				case *TCPChannelCloseError:
					logger.ErrorWith("tcp channel close error", err, log.String("channel", e.Channel.Name()))
				}
			}
		}
	}
	s.runner, s.Lifecycle = lifecycle.New(s.name, lifecycle.Core(s.start, s.run, s.close))
	return s
}

func NewTCPServer(addr *net.TCPAddr, options ...Option) *TCPServer {
	return newTCPServer(nil, addr, options...)
}

func NewTCPServerWithListener(listener *net.TCPListener, options ...Option) *TCPServer {
	return newTCPServer(listener, nil, options...)
}

func (s *TCPServer) setName(name string) {
	s.name = name
}

func (s *TCPServer) setSocketBuffer(readBuffer, writeBuffer int) {
	s.channelOptions = append(s.channelOptions, WithTCPSocketBuffer(readBuffer, writeBuffer))
}

func (s *TCPServer) setBufferPoolSize(size uint) {
	s.channelOptions = append(s.channelOptions, WithTCPChannelBufferPoolSize(size))
}

func (s *TCPServer) setBufferReverse(reverse uint) {
	s.channelOptions = append(s.channelOptions, WithTCPChannelBufferReverse(reverse))
}

func (s *TCPServer) setWriteBufferSize(size uint) {
	s.channelOptions = append(s.channelOptions, WithTCPChannelWriteBufferSize(size))
}

func (s *TCPServer) setOnError(onError func(s Server, err error)) {
	s.onError = onError
}

func (s *TCPServer) Name() string {
	return s.name
}

func (s *TCPServer) Addr() net.Addr {
	return s.addr
}

func (s *TCPServer) newChannel(conn *net.TCPConn, options ...TCPChannelOption) *TCPChannel {
	remoteAddr := conn.RemoteAddr().(*net.TCPAddr)
	addrID := id.GenerateTcpAddrId(remoteAddr)
	ch := &TCPChannel{
		server:     s,
		remoteAddr: remoteAddr,
		addrID:     addrID,

		packetPool: pool.NewPool(func(p *pool.Pool[*rtp.Packet]) *rtp.Packet {
			return rtp.NewPacket(rtp.NewLayer(), rtp.PacketPool(p))
		}),
	}
	ch.conn.Store(conn)
	for _, option := range options {
		option.apply(ch)
	}
	if ch.name == "" {
		ch.name = fmt.Sprintf("rtp tcp channel(%s <--> %s)", conn.LocalAddr().String(), conn.RemoteAddr().String())
	}
	if ch.bufferPool == (rtp.BufferPool{}) {
		ch.bufferPool = rtp.NewBufferPool(rtp.DefaultBufferPoolSize)
	}
	if ch.bufferReverse == 0 {
		ch.bufferReverse = rtp.DefaultBufferReverse
	}
	if ch.writePool == nil {
		ch.writePool = pool.NewDataPool(rtp.DefaultWriteBufferSize)
	}
	if ch.onError == nil {
		ch.onError = func(c *TCPChannel, err error) {
			if logger := s.Logger(); logger != nil {
				switch e := err.(type) {
				case *SetReadBufferError:
					logger.ErrorWith("set tcp socket read buffer error", err, log.Int("read buffer", e.ReadBuffer))
				case *SetWriteBufferError:
					logger.ErrorWith("set tcp socket write buffer error", err, log.Int("write buffer", e.WriteBuffer))
				case *SetKeepaliveError:
					logger.ErrorWith("set tcp keepalive error", err, log.Bool("keepalive", e.Keepalive))
				case *SetKeepalivePeriodError:
					logger.ErrorWith("set tcp keepalive period error", err, log.Duration("keepalive period", e.KeepalivePeriod))
				case *SetNoDelayError:
					logger.ErrorWith("set tcp no delay error", err, log.Bool("no delay", e.NoDelay))
				case *TCPReadError:
					logger.ErrorWith("tcp connection read error", err)
				}
			}
		}
	}
	if logger := s.Logger(); logger != nil {
		ch.CompareAndSwapLogger(nil, logger)
	}
	if postHandler := ch.postHandler; postHandler != nil {
		postHandler(ch)
	}
	ch.runner, ch.OnceLifecycle = lifecycle.NewOnce(ch.name, lifecycle.RunFn(ch.run), lifecycle.CloseFn(ch.close))
	return ch
}

func (s *TCPServer) handleError(err error) error {
	if s.onError != nil {
		s.onError(s, err)
	}
	return err
}

func (s *TCPServer) start() error {
	if s.listener == nil {
		listener, err := net.ListenTCP("tcp", s.addr)
		if err != nil {
			return &TCPListenError{Err: Err{err}, Addr: s.addr}
		}
		s.listener = listener
	}
	return nil
}

func (s *TCPServer) run() error {
	defer func() {
		var futures []chan error
		var channels []*TCPChannel
		s.mu.Lock()
		for _, channel := range s.channels {
			futures = append(futures, channel.AddClosedFuture(nil))
			channels = append(channels, channel)
		}
		s.mu.Unlock()
		for _, channel := range channels {
			if err := channel.Close(nil); err != nil {
				s.handleError(&TCPChannelCloseError{Err: Err{err}, Channel: channel})
			}
		}
		for i, future := range futures {
			<-future
			if onChannelClosed := s.onChannelClosed; onChannelClosed != nil {
				onChannelClosed(s, channels[i])
			}
		}
		s.listener = nil
	}()
	for {
		conn, err := s.listener.AcceptTCP()
		if err != nil {
			if opErr, is := err.(*net.OpError); is && opErr.Err.Error() == "use of closed network connection" {
				return nil
			}
			return s.handleError(&TCPAcceptError{Err{err}})
		}
		options := s.channelOptions
		if onAccept := s.onAccept; onAccept != nil {
			options = append(options, onAccept(s, conn)...)
		}
		ch := s.newChannel(conn, options...)
		s.mu.Lock()
		s.channels[ch.addrID] = ch
		if stream := s.streams[ch.addrID]; stream != nil {
			ch.stream.Store(stream)
			stream.channel.Store(ch)
			delete(s.streams, ch.addrID)
		}
		s.mu.Unlock()
	}
}

func (s *TCPServer) close() error {
	err := s.listener.Close()
	if err != nil {
		return &TCPCloseError{Err: Err{err}, Listener: s.listener}
	}
	return nil
}

func (s *TCPServer) Stream(remoteAddr net.Addr, ssrc int64, handler Handler) Stream {
	tAddr, is := remoteAddr.(*net.TCPAddr)
	if !is {
		return nil
	}
	addrID := id.GenerateTcpAddrId(tAddr)
	stream := &TCPStream{addrID: addrID}
	stream.self = stream
	stream.localAddr = s.addr
	stream.server.Store(s)
	stream.SetRemoteAddr(remoteAddr)
	stream.SetSSRC(ssrc)
	stream.SetHandler(handler)
	stream.SetOnLossPacket(nil)
	stream.SetLogger(s.Logger())
	var old *TCPStream
	s.mu.Lock()
	if ch := s.channels[addrID]; ch != nil {
		// found the corresponding channel, bind the channel and stream,
		// if the channel is already bound to a stream, unbinding old stream
		// first
		old = ch.stream.Load()
		ch.stream.Store(stream)
		stream.channel.Store(ch)
	} else {
		// not found the corresponding channel
		old = s.streams[addrID]
		s.streams[addrID] = stream
	}
	s.mu.Unlock()
	if old != nil {
		old.server.Store(nil)
		old.OnStreamClosed(old)
	}
	return stream
}

func (s *TCPServer) RemoveStream(stream Stream) {
	if ts, is := stream.(*TCPStream); is {
		var ch *TCPChannel
		var removed bool
		s.mu.Lock()
		if ch = ts.channel.Load(); ch != nil {
			// stream is binding, unbinding stream and channel
			ch.stream.Store(nil)
			ts.channel.Store(nil)
		} else if s.streams[ts.addrID] == ts {
			// stream not binding, remove from unbound streams
			delete(s.streams, ts.addrID)
		} else {
			// stream has been removed
			removed = true
		}
		s.mu.Unlock()
		if ch != nil {
			ch.Close(nil)
		}
		if !removed {
			ts.server.Store(nil)
			ts.OnStreamClosed(ts)
		}
	}
}
