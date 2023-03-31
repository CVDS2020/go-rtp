package server

import (
	"errors"
	"fmt"
	"gitee.com/sy_183/common/id"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/option"
	"gitee.com/sy_183/common/pool"
	mapUtils "gitee.com/sy_183/common/utils/map"
	"gitee.com/sy_183/rtp/rtp"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type TCPServer struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	addr     *net.TCPAddr
	listener closableWrapper[net.TCPListener]

	// 默认的通道配置项
	channelOptions []option.AnyOption
	// TCP连接通道
	channels map[string]*TCPChannel
	// 未匹配到通道的流
	streams map[string]*TCPStream
	// 通道和流的互斥锁
	mu sync.Mutex

	// 错误处理函数
	onError atomic.Pointer[func(s Server, err error)]
	// 接受TCP连接的处理函数
	onAccept atomic.Pointer[func(s *TCPServer, conn *net.TCPConn) []option.AnyOption]
	// tCP连接通道创建成功的处理函数
	onChannelCreated atomic.Pointer[func(s *TCPServer, channel *TCPChannel)]

	log.AtomicLogger
}

func newTCPServer(listener *net.TCPListener, addr *net.TCPAddr, options ...option.AnyOption) *TCPServer {
	var s *TCPServer
	if listener != nil {
		s = &TCPServer{addr: listener.Addr().(*net.TCPAddr)}
		s.listener.Set(listener, false)
	} else {
		s = &TCPServer{addr: addr}
	}
	s.channels = make(map[string]*TCPChannel)
	s.streams = make(map[string]*TCPStream)
	for _, opt := range options {
		opt.Apply(s)
	}
	if s.addr == nil {
		s.addr = &net.TCPAddr{IP: net.IP{0, 0, 0, 0}, Port: 5004}
	}
	if onError := s.onError.Load(); onError == nil {
		s.SetOnError(s.defaultOnError)
	}
	s.runner = lifecycle.NewWithRun(s.start, s.run, s.close)
	s.Lifecycle = s.runner
	return s
}

func NewTCPServer(addr *net.TCPAddr, options ...option.AnyOption) *TCPServer {
	return newTCPServer(nil, addr, options...)
}

func NewTCPServerWithListener(listener *net.TCPListener, options ...option.AnyOption) *TCPServer {
	return newTCPServer(listener, nil, options...)
}

func (s *TCPServer) setTimeout(timeout time.Duration) {
	s.channelOptions = append(s.channelOptions, WithTimeout(timeout))
}

func (s *TCPServer) setPacketPoolProvider(provider pool.PoolProvider[*rtp.IncomingPacket]) {
	s.channelOptions = append(s.channelOptions, WithPacketPoolProvider(provider))
}

func (s *TCPServer) setReadBufferPoolProvider(provider func() pool.BufferPool) {
	s.channelOptions = append(s.channelOptions, WithReadBufferPoolProvider(provider))
}

func (s *TCPServer) setWriteBufferPoolProvider(provider func() pool.DataPool) {
	s.channelOptions = append(s.channelOptions, WithWriteBufferPoolProvider(provider))
}

func (s *TCPServer) setOnError(onError func(s Server, err error)) {
	s.onError.Store(&onError)
}

func (s *TCPServer) setCloseOnStreamClosed(enable bool) {
	s.channelOptions = append(s.channelOptions, WithCloseOnStreamClosed(enable))
}

func (s *TCPServer) setOnAccept(onAccept func(s *TCPServer, conn *net.TCPConn) []option.AnyOption) {
	s.onAccept.Store(&onAccept)
}

func (s *TCPServer) setOnChannelCreated(onChannelCreated func(s *TCPServer, channel *TCPChannel)) {
	s.onChannelCreated.Store(&onChannelCreated)
}

func (s *TCPServer) newChannel(conn *net.TCPConn, options ...option.AnyOption) *TCPChannel {
	remoteTCPAddr := conn.RemoteAddr().(*net.TCPAddr)
	var localTCPAddr *net.TCPAddr
	if localAddr := conn.LocalAddr(); localAddr != nil {
		localTCPAddr = localAddr.(*net.TCPAddr)
	}
	addrID := id.GenerateTcpAddrId(remoteTCPAddr)
	ch := &TCPChannel{
		server:     s,
		localAddr:  localTCPAddr,
		remoteAddr: remoteTCPAddr,
		addrID:     addrID,
	}
	ch.conn.Set(conn, false)
	ch.SetCloseOnStreamClosed(true)
	for _, opt := range options {
		opt.Apply(ch)
	}
	if timeout := ch.Timeout(); timeout != 0 && timeout < time.Second {
		ch.SetTimeout(timeout)
	}
	if ch.readBufferPool == nil {
		ch.readBufferPool = pool.NewDefaultBufferPool(rtp.DefaultBufferPoolSize, rtp.DefaultBufferReverse, pool.ProvideSlicePool[*pool.Buffer])
	}
	if ch.writeBufferPool == nil {
		ch.writeBufferPool = pool.NewStaticDataPool(rtp.DefaultWriteBufferSize, pool.ProvideSlicePool[*pool.Data])
	}
	if ch.packetPool == nil {
		ch.setPacketPoolProvider(pool.ProvideSyncPool[*rtp.IncomingPacket])
	}
	if onError := ch.onError.Load(); onError == nil {
		ch.SetOnError(ch.defaultOnError)
	}
	if ch.Logger() == nil {
		if logger := s.Logger(); logger != nil {
			ch.CompareAndSwapLogger(nil, logger.Named(fmt.Sprintf("RTP服务TCP通道(%s <--> %s)", localTCPAddr, remoteTCPAddr)))
		}
	}
	ch.runner = lifecycle.NewWithRun(ch.start, ch.run, ch.close)
	ch.Lifecycle = ch.runner
	if onChannelCreated := s.GetOnChannelCreated(); onChannelCreated != nil {
		onChannelCreated(s, ch)
	}
	return ch
}

func (s *TCPServer) Addr() net.Addr {
	return s.addr
}

func (s *TCPServer) Listener() *net.TCPListener {
	return s.listener.Get()
}

func (s *TCPServer) GetOnError() func(s Server, err error) {
	if onError := s.onError.Load(); onError != nil {
		return *onError
	}
	return nil
}

func (s *TCPServer) SetOnError(onError func(s Server, err error)) Server {
	s.setOnError(onError)
	return s
}

func (s *TCPServer) handleError(err error) error {
	if onError := s.GetOnError(); onError != nil {
		onError(s, err)
	}
	return err
}

func (s *TCPServer) defaultOnError(_ Server, err error) {
	if logger := s.Logger(); logger != nil {
		switch e := err.(type) {
		case *TCPListenError:
			logger.ErrorWith("监听TCP连接失败", err, log.String("监听地址", e.Addr.String()))
		case *TCPAcceptError:
			logger.ErrorWith("接受TCP连接失败", err)
		case *TCPCloseError:
			logger.ErrorWith("关闭TCP连接失败", err)
		}
	}
}

func (s *TCPServer) GetOnAccept() func(s *TCPServer, conn *net.TCPConn) []option.AnyOption {
	if onAccept := s.onAccept.Load(); onAccept != nil {
		return *onAccept
	}
	return nil
}

func (s *TCPServer) SetOnAccept(onAccept func(s *TCPServer, conn *net.TCPConn) []option.AnyOption) *TCPServer {
	s.setOnAccept(onAccept)
	return s
}

func (s *TCPServer) GetOnChannelCreated() func(s *TCPServer, channel *TCPChannel) {
	if onChannelCreated := s.onChannelCreated.Load(); onChannelCreated != nil {
		return *onChannelCreated
	}
	return nil
}

func (s *TCPServer) SetOnChannelCreated(onChannelCreated func(s *TCPServer, channel *TCPChannel)) *TCPServer {
	s.setOnChannelCreated(onChannelCreated)
	return s
}

func (s *TCPServer) accept() (conn *net.TCPConn, closed bool, err error) {
	listener := s.listener.Get()
	conn, err = listener.AcceptTCP()
	if err != nil {
		if opErr, is := err.(*net.OpError); is && errors.Is(opErr.Err, net.ErrClosed) {
			return nil, true, nil
		}
		return nil, true, s.handleError(&TCPAcceptError{Err{err}})
	}
	return
}

func (s *TCPServer) start(lifecycle.Lifecycle) (err error) {
	listener, _, err := s.listener.Init(func() (*net.TCPListener, error) {
		listener, err := net.ListenTCP("tcp", s.addr)
		if err != nil {
			return nil, s.handleError(&TCPListenError{Err: Err{err}, Addr: s.addr})
		}
		lock.LockDo(&s.mu, func() {
			// 在listener启动之前添加的TCP流，需要在此时设置流过期定时器和当前时间，用于后续的流超时
			// 处理
			for _, stream := range s.streams {
				stream.timer.Init(func() { s.removeStream(stream) })
			}
		})
		return listener, nil
	}, false)

	if err != nil {
		return err
	} else if listener == nil {
		return lifecycle.NewStateClosedError("TCP服务")
	}
	return nil
}

func (s *TCPServer) run(lifecycle.Lifecycle) error {
	defer func() {
		s.listener.SetClosed()
		channels := lock.LockGet(&s.mu, func() []*TCPChannel {
			for _, stream := range s.streams {
				s.closeStream(stream)
			}
			for addrId := range s.streams {
				delete(s.streams, addrId)
			}
			return mapUtils.Values(s.channels)
		})
		for _, channel := range channels {
			channel.Close(nil)
		}
		for _, channel := range channels {
			<-channel.ClosedWaiter()
		}
	}()

	for {
		conn, closed, err := s.accept()
		if closed {
			return err
		} else if conn == nil {
			continue
		}

		options := s.channelOptions
		if onAccept := s.GetOnAccept(); onAccept != nil {
			options = append(options, onAccept(s, conn)...)
		}
		ch := s.newChannel(conn, options...)
		stream := lock.LockGet(&s.mu, func() *TCPStream {
			// 添加通道到TCP服务中
			s.channels[ch.addrID] = ch
			// 如果有TCP流匹配此通道，将此流从TCP服务中移除
			if stream := s.streams[ch.addrID]; stream != nil {
				delete(s.streams, ch.addrID)
				return stream
			}
			return nil
		})
		if stream != nil {
			// 绑定流到TCP通道，一定可以绑定成功，因为通道还未启动，不可能被关闭。此处不更新当前时间，
			// 因为绑定到TCP通道之前的时间也算作超时时间的一部分
			ch.bindStream(stream)
		} else {
			// 没有TCP流匹配此通道，初始化超时定时器用于通道的超时处理
			ch.timer.Init(func() { ch.Close(nil) })
		}

		// 当通道关闭时，从TCP服务中删除此通道
		ch.OnClosed(func(lifecycle.Lifecycle, error) {
			lock.LockDo(&s.mu, func() {
				if s.channels[ch.addrID] == ch {
					delete(s.channels, ch.addrID)
				}
			})
		})
		// 启动通道，一定可以启动成功，因为通道刚刚创建一定是第一次启动
		ch.Start()
	}
}

func (s *TCPServer) close(lifecycle.Lifecycle) error {
	if listener, err := s.listener.Close(); err != nil {
		return s.handleError(&TCPCloseError{Err: Err{err}, Listener: listener})
	}
	return nil
}

func (s *TCPServer) bindStream(stream *TCPStream) bool {
	addrId := stream.addrId
	return lock.LockGet(&s.listener, func() bool {
		if s.listener.CloseFlag() {
			return false
		}
		// 此函数将流绑定到TCP服务
		bindToServer := func() (old *TCPStream) {
			old = s.streams[addrId]
			s.streams[addrId] = stream
			if s.listener.Get() != nil {
				// listener已经启动，设置流过期定时器和当前时间用于后续的流超时处理
				stream.timer.Init(func() { s.removeStream(stream) })
			}
			return
		}
		channel, old := lock.LockGetDouble(&s.mu, func() (channel *TCPChannel, old *TCPStream) {
			if channel = s.channels[addrId]; channel != nil {
				return channel, nil
			}
			return nil, bindToServer()
		})
		if channel != nil {
			// 如果流匹配到TCP通道，则尝试与TCP通道绑定，并且此时listener一定启动完成
			if channel.bindStream(stream) {
				// 绑定成功，由于流是刚初始化完成的，需设置当前时间，用于后续的超时判断
				stream.timer.SetTime(time.Now())
				return true
			}
			// 通道已经执行关闭流程，无法绑定流，使用TCP服务绑定此流
			old = lock.LockGet(&s.mu, bindToServer)
		}
		if old != nil {
			s.closeStream(old)
		}
		return true
	})
}

func (s *TCPServer) Stream(remoteAddr net.Addr, ssrc int64, handler Handler, options ...option.AnyOption) (Stream, error) {
	tAddr, is := remoteAddr.(*net.TCPAddr)
	if !is {
		return nil, errors.New("")
	}
	stream := newTCPStream(id.GenerateTcpAddrId(tAddr), s, s.addr, remoteAddr, ssrc, handler, options...)
	if stream.Logger() == nil {
		if logger := s.Logger(); logger != nil {
			if ssrc >= 0 {
				stream.SetLogger(logger.Named(fmt.Sprintf("基于TCP的RTP流(远端地址:%s,SSRC:%d)", tAddr, ssrc)))
			} else {
				stream.SetLogger(logger.Named(fmt.Sprintf("基于TCP的RTP流(远端地址:%s)", tAddr)))
			}
		}
	}
	if !s.bindStream(stream) {
		return nil, lifecycle.NewStateClosedError("TCP服务")
	}
	return stream, nil
}

func (s *TCPServer) closeStream(stream *TCPStream) {
	stream.server.Store(nil)
	stream.timer.Close()
	stream.OnStreamClosed(stream)
}

func (s *TCPServer) removeStream(stream *TCPStream) {
	if !lock.LockGet(&s.mu, func() (removed bool) {
		if s.streams[stream.addrId] == stream {
			delete(s.streams, stream.addrId)
			return false
		}
		return true
	}) {
		s.closeStream(stream)
	}
}

func (s *TCPServer) RemoveStream(stream Stream) {
	if ts, is := stream.(*TCPStream); is {
		if channel := ts.channel.Load(); channel != nil {
			channel.removeStream(ts)
			return
		}
		s.removeStream(ts)
	}
}
