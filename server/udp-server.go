package server

import (
	"errors"
	"fmt"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/option"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"net"
	"strings"
	"sync/atomic"
	"time"
	_ "unsafe"
)

type UDPServer struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	addr     *net.UDPAddr
	listener closableWrapper[net.UDPConn]

	readBuffer  atomic.Int64
	writeBuffer atomic.Int64

	stream atomic.Pointer[UDPStream]

	readBufferPool  pool.BufferPool
	writeBufferPool pool.DataPool
	packetPool      pool.Pool[*rtp.IncomingPacket]

	onError             atomic.Pointer[func(s Server, err error)]
	closeOnStreamClosed atomic.Bool

	log.AtomicLogger
}

func newUDPServer(listener *net.UDPConn, addr *net.UDPAddr, options ...option.AnyOption) *UDPServer {
	var s *UDPServer
	if listener != nil {
		s = &UDPServer{addr: listener.LocalAddr().(*net.UDPAddr)}
		s.listener.Set(listener, false)
	} else {
		s = &UDPServer{addr: addr}
	}
	for _, opt := range options {
		opt.Apply(s)
	}
	if s.addr == nil {
		s.addr = &net.UDPAddr{IP: net.IP{0, 0, 0, 0}, Port: 5004}
	}
	if s.readBufferPool == nil {
		s.readBufferPool = pool.NewDefaultBufferPool(rtp.DefaultBufferPoolSize, rtp.DefaultBufferReverse, pool.ProvideSyncPool[*pool.Buffer])
	}
	if s.writeBufferPool == nil {
		s.writeBufferPool = pool.NewStaticDataPool(rtp.DefaultWriteBufferSize, pool.ProvideSyncPool[*pool.Data])
	}
	if s.packetPool == nil {
		s.setPacketPoolProvider(pool.ProvideSyncPool[*rtp.IncomingPacket])
	}
	if onError := s.onError.Load(); onError == nil {
		s.SetOnError(s.defaultOnError)
	}
	s.runner = lifecycle.NewWithRun(s.start, s.run, s.close, lifecycle.WithSelf(s))
	s.Lifecycle = s.runner
	return s
}

func NewUDPServer(addr *net.UDPAddr, options ...option.AnyOption) *UDPServer {
	return newUDPServer(nil, addr, options...)
}

func NewUDPServerWithListener(listener *net.UDPConn, options ...option.AnyOption) *UDPServer {
	return newUDPServer(listener, nil, options...)
}

func UDPServerProvider(options ...option.AnyOption) ServerProvider {
	return func(m *Manager, port uint16, opts ...option.AnyOption) Server {
		ipAddr := m.Addr()
		return NewUDPServer(&net.UDPAddr{
			IP:   ipAddr.IP,
			Port: int(port),
			Zone: ipAddr.Zone,
		}, append(opts, options...)...)
	}
}

func (s *UDPServer) setPacketPoolProvider(provider pool.PoolProvider[*rtp.IncomingPacket]) {
	s.packetPool = provider(rtp.ProvideIncomingPacket)
}

func (s *UDPServer) setReadBufferPool(bufferPool pool.BufferPool) {
	s.readBufferPool = bufferPool
}

func (s *UDPServer) setReadBufferPoolProvider(provider func() pool.BufferPool) {
	s.readBufferPool = provider()
}

func (s *UDPServer) setWriteBufferPool(bufferPool pool.DataPool) {
	s.writeBufferPool = bufferPool
}

func (s *UDPServer) setWriteBufferPoolProvider(provider func() pool.DataPool) {
	s.writeBufferPool = provider()
}

func (s *UDPServer) setOnError(onError func(s Server, err error)) {
	s.onError.Store(&onError)
}

func (s *UDPServer) setCloseOnStreamClosed(enable bool) {
	s.closeOnStreamClosed.Store(enable)
}

func (s *UDPServer) Addr() net.Addr {
	return s.addr
}

func (s *UDPServer) Listener() *net.UDPConn {
	return s.listener.Get()
}

func (s *UDPServer) GetOnError() func(s Server, err error) {
	if onError := s.onError.Load(); onError != nil {
		return *onError
	}
	return nil
}

func (s *UDPServer) SetOnError(onError func(s Server, err error)) Server {
	s.setOnError(onError)
	return s
}

func (s *UDPServer) handleError(err error) error {
	if onError := s.GetOnError(); onError != nil {
		onError(s, err)
	}
	return err
}

func (s *UDPServer) defaultOnError(_ Server, err error) {
	if logger := s.Logger(); logger != nil {
		switch e := err.(type) {
		case *UDPListenError:
			logger.ErrorWith("监听UDP连接失败", err, log.String("监听地址", e.Addr.String()))
		case *SetReadBufferError:
			logger.ErrorWith("设置 SOCKET 读缓冲区失败", err, log.Int("缓冲区大小", e.ReadBuffer))
		case *SetWriteBufferError:
			logger.ErrorWith("设置 SOCKET 写缓冲区失败", err, log.Int("缓冲区大小", e.WriteBuffer))
		case *UDPReadError:
			logger.ErrorWith("UDP读取数据错误", err)
		case *UDPCloseError:
			logger.ErrorWith("关闭UDP连接失败", err)
		}
	}
}

func (s *UDPServer) CloseOnStreamClosed() bool {
	return s.closeOnStreamClosed.Load()
}

func (s *UDPServer) SetCloseOnStreamClosed(enable bool) Server {
	s.setCloseOnStreamClosed(enable)
	return s
}

func (s *UDPServer) read() (data *pool.Data, addr *net.UDPAddr, closed bool, err error) {
	readBufferPool := s.readBufferPool
	buf := readBufferPool.Get()
	if buf == nil {
		// UDP读取缓冲区申请失败，可能是因为流量过大导致，所以可以利用缓冲区做限流
		s.Close(nil)
		return nil, nil, true, s.handleError(BufferAllocError)
	}

	listener := s.listener.Get()
	stream := s.stream.Load()

	var deadline time.Time
	if stream != nil {
		deadline = stream.timer.Deadline()
	}
	if !deadline.IsZero() {
		listener.SetReadDeadline(deadline)
	}

	n, addr, err := listener.ReadFromUDP(buf)
	if err != nil {
		if netErr, is := err.(net.Error); is && netErr.Timeout() {
			if stream != nil {
				if onTimeout := stream.GetOnTimeout(); onTimeout != nil {
					onTimeout(stream)
				}
				if s.removeStream(stream) {
					return nil, nil, true, nil
				}
			}
			return nil, nil, false, nil
		} else if opErr, is := err.(*net.OpError); is && errors.Is(opErr.Err, net.ErrClosed) {
			return nil, nil, true, nil
		}
		return nil, nil, true, s.handleError(&UDPReadError{Err{err}})
	}
	return readBufferPool.Alloc(uint(n)), addr, false, nil
}

func (s *UDPServer) start(lifecycle.Lifecycle) (err error) {
	listener, _, err := s.listener.Init(func() (*net.UDPConn, error) {
		listener, err := net.ListenUDP("udp", s.addr)
		if err != nil {
			return nil, s.handleError(&UDPListenError{Err: Err{err}, Addr: s.addr})
		}
		if stream := s.stream.Load(); stream != nil {
			// listener已启动，设置stream当前时间用于后续的超时判断
			stream.timer.SetTime(time.Now())
		}
		return listener, nil
	}, false)

	if err != nil {
		return err
	} else if listener == nil {
		return lifecycle.NewStateClosedError("UDP服务")
	}
	return nil
}

func (s *UDPServer) run(lifecycle.Lifecycle) error {
	defer func() {
		s.listener.SetClosed() // 将listener设置为关闭，此时stream将不会被添加或修改
		if stream := s.stream.Swap(nil); stream != nil {
			s.closeStream(stream)
		}
	}()

	for {
		data, addr, closed, err := s.read()
		if closed {
			return err
		} else if data == nil {
			continue
		}

		if stream := s.stream.Load(); stream != nil {
			packet := s.packetPool.Get().Use()
			if err := packet.Unmarshal(data.Data); err != nil {
				data.Release()
				packet.Release()
				if !stream.OnParseError(stream, err) {
					if s.removeStream(stream) {
						return nil
					}
				}
				continue
			}
			packet.SetAddr(addr)
			packet.SetTime(time.Now())
			packet.AddRelation(data)
			dropped, keep := stream.HandlePacket(stream, packet)
			if !keep && s.removeStream(stream) {
				return nil
			}
			if !dropped {
				stream.timer.SetTime(time.Now())
			}
		} else {
			data.Release()
		}
	}
}

func (s *UDPServer) close(lifecycle.Lifecycle) error {
	if listener, err := s.listener.Close(); err != nil {
		return s.handleError(&UDPCloseError{Err: Err{err}, Listener: listener})
	}
	return nil
}

// bindStream 绑定UDP流到UDP服务，如果UDP服务已经执行关闭流程则绑定失败，返回false，如果UDP服务已经
// 绑定过流，则关闭旧的UDP流
func (s *UDPServer) bindStream(stream *UDPStream) bool {
	return lock.LockGet(&s.listener, func() bool {
		if s.listener.CloseFlag() {
			return false
		}
		if s.listener.Get() != nil {
			// listener已启动，设置stream当前时间用于后续的超时判断
			stream.timer.SetTime(time.Now())
		}
		if old := s.stream.Swap(stream); old != nil {
			s.closeStream(old)
		}
		return true
	})
}

func (s *UDPServer) Stream(remoteAddr net.Addr, ssrc int64, handler Handler, options ...option.AnyOption) (Stream, error) {
	var uAddr *net.UDPAddr
	if remoteAddr != nil {
		addr, is := remoteAddr.(*net.UDPAddr)
		if !is {
			return nil, errors.New("")
		}
		uAddr = addr
	}
	stream := newUDPStream(s, s.addr, uAddr, ssrc, handler, options...)
	if stream.Logger() == nil {
		if logger := s.Logger(); logger != nil {
			items := []string{fmt.Sprintf("本端地址:%s", s.addr)}
			if remoteAddr != nil {
				items = append(items, fmt.Sprintf("远端地址:%s", remoteAddr))
			}
			if ssrc >= 0 {
				items = append(items, fmt.Sprintf("SSRC:%d", ssrc))
			}
			stream.SetLogger(logger.Named(fmt.Sprintf("基于UDP的RTP流(%s)", strings.Join(items, ","))))
		}
	}
	if !s.bindStream(stream) {
		return nil, errors.New("")
	}
	return stream, nil
}

func (s *UDPServer) closeStream(stream *UDPStream) {
	stream.server.Store(nil)
	stream.OnStreamClosed(stream)
}

// removeStream 移除一个UDP流，要求此流为当前UDP服务所管理的流，如果UDP服务设置了当流关闭时关闭服务，
// 则会关闭当前UDP服务
func (s *UDPServer) removeStream(stream *UDPStream) (close bool) {
	if s.stream.CompareAndSwap(stream, nil) {
		s.closeStream(stream)
		if s.closeOnStreamClosed.Load() && stream.CloseConn() {
			s.Close(nil)
			return true
		}
	}
	return false
}

func (s *UDPServer) RemoveStream(stream Stream) {
	if us, is := stream.(*UDPStream); is {
		s.removeStream(us)
	}
}

func (s *UDPServer) SendTo(layer rtp.Layer, addr *net.UDPAddr) error {
	listener := s.listener.Get()
	if listener != nil {
		size := layer.Size()
		data := s.writeBufferPool.Alloc(uint(size))
		layer.Read(data.Data)
		_, err := listener.WriteTo(data.Data, addr)
		data.Release()
		return err
	}
	return nil
}
