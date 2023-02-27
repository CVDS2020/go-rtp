package server

import (
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"io"
	"net"
	"sync/atomic"
	"time"
)

type TCPChannel struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	server     *TCPServer
	conn       closableWrapper[net.TCPConn]
	localAddr  *net.TCPAddr
	remoteAddr *net.TCPAddr
	addrID     string

	stream atomic.Pointer[TCPStream]
	parser rtp.Parser
	packet *rtp.Packet

	timer timeoutTimer

	readBufferPool  pool.BufferPool
	writeBufferPool pool.DataPool
	dropBuffer      []byte
	packetPool      pool.Pool[*rtp.Packet]

	onError             atomic.Pointer[func(c *TCPChannel, err error)]
	onTimeout           atomic.Pointer[func(*TCPChannel)]
	closeOnStreamClosed atomic.Bool

	log.AtomicLogger
}

func (c *TCPChannel) setTimeout(timeout time.Duration) {
	c.timer.setTimeout(timeout)
}

func (c *TCPChannel) setPacketPoolProvider(provider pool.PoolProvider[*rtp.Packet]) {
	c.packetPool = provider(func(p pool.Pool[*rtp.Packet]) *rtp.Packet {
		return rtp.NewPacket(rtp.NewLayer(), rtp.PacketPool(p))
	})
}

func (c *TCPChannel) setReadBufferPool(bufferPool pool.BufferPool) {
	c.readBufferPool = bufferPool
}

func (c *TCPChannel) setReadBufferPoolProvider(provider func() pool.BufferPool) {
	c.readBufferPool = provider()
}

func (c *TCPChannel) setWriteBufferPool(bufferPool pool.DataPool) {
	c.writeBufferPool = bufferPool
}

func (c *TCPChannel) setWriteBufferPoolProvider(provider func() pool.DataPool) {
	c.writeBufferPool = provider()
}

func (c *TCPChannel) setOnError(onError func(s *TCPChannel, err error)) {
	c.onError.Store(&onError)
}

func (c *TCPChannel) setOnTimeout(onTimeout func(*TCPChannel)) {
	c.onTimeout.Store(&onTimeout)
}

func (c *TCPChannel) setCloseOnStreamClosed(enable bool) {
	c.closeOnStreamClosed.Store(enable)
}

func (c *TCPChannel) handleError(err error) error {
	if onError := c.GetOnError(); onError != nil {
		onError(c, err)
	}
	return err
}

func (c *TCPChannel) defaultOnError(_ *TCPChannel, err error) {
	if logger := c.Logger(); logger != nil {
		switch err.(type) {
		//case *SetReadBufferError:
		//	logger.ErrorWith("设置 SOCKET 读缓冲区失败", err, log.Int("缓冲区大小", e.ReadBuffer))
		//case *SetWriteBufferError:
		//	logger.ErrorWith("设置 SOCKET 写缓冲区失败", err, log.Int("缓冲区大小", e.WriteBuffer))
		//case *SetKeepaliveError:
		//	logger.ErrorWith("设置TCP连接是否开启 KEEPALIVE 失败", err, log.Bool("是否开启", e.Keepalive))
		//case *SetKeepalivePeriodError:
		//	logger.ErrorWith("设置TCP连接 KEEPALIVE 间隔失败", err, log.Duration("KEEPALIVE 间隔", e.KeepalivePeriod))
		//case *SetNoDelayError:
		//	logger.ErrorWith("设置TCP连接是否开启 NO_DELAY 失败", err, log.Bool("是否开启", e.NoDelay))
		case *TCPReadError:
			logger.ErrorWith("TCP连接读取数据错误", err)
		case *TCPReadTimeout:
			logger.Warn("一段时间内没有接收到数据，关闭TCP连接")
		case *TCPChannelCloseError:
			logger.ErrorWith("关闭TCP连接失败", err)
		default:
			logger.ErrorWith("TCP连接发生错误", err)
		}
	}
}

func (c *TCPChannel) Conn() *net.TCPConn {
	return c.conn.Get()
}

func (c *TCPChannel) LocalAddr() *net.TCPAddr {
	return c.localAddr
}

func (c *TCPChannel) RemoteAddr() *net.TCPAddr {
	return c.remoteAddr
}

func (c *TCPChannel) Timeout() time.Duration {
	return c.timer.Timeout()
}

func (c *TCPChannel) SetTimeout(timeout time.Duration) *TCPChannel {
	c.timer.SetTimeout(timeout)
	return c
}

func (c *TCPChannel) GetOnError() func(s *TCPChannel, err error) {
	if onError := c.onError.Load(); onError != nil {
		return *onError
	}
	return nil
}

func (c *TCPChannel) SetOnError(onError func(s *TCPChannel, err error)) *TCPChannel {
	c.setOnError(onError)
	return c
}

func (c *TCPChannel) GetOnTimeout() func(*TCPChannel) {
	if onTimeout := c.onTimeout.Load(); onTimeout != nil {
		return *onTimeout
	}
	return nil
}

func (c *TCPChannel) SetOnTimeout(onTimeout func(*TCPChannel)) *TCPChannel {
	c.setOnTimeout(onTimeout)
	return c
}

func (c *TCPChannel) CloseOnStreamClosed() bool {
	return c.closeOnStreamClosed.Load()
}

func (c *TCPChannel) SetCloseOnStreamClosed(enable bool) *TCPChannel {
	c.setCloseOnStreamClosed(enable)
	return c
}

func (c *TCPChannel) read() (data *pool.Data, closed bool, err error) {
	readBufferPool := c.readBufferPool
	buf := readBufferPool.Get()
	if buf == nil {
		// TCP通道读取缓冲区申请失败，可能是因为流量过大导致，所以可以利用缓冲区做限流
		c.Close(nil)
		return nil, true, c.handleError(BufferAllocError)
	}

	conn := c.conn.Get()
	stream := c.stream.Load()

	var deadline time.Time
	if stream != nil {
		deadline = stream.timer.Deadline()
	}
	if !deadline.IsZero() {
		conn.SetReadDeadline(deadline)
	}

	n, err := conn.Read(buf)
	if err != nil {
		if netErr, is := err.(net.Error); is && netErr.Timeout() {
			if stream != nil {
				if onTimeout := stream.GetOnTimeout(); onTimeout != nil {
					onTimeout(stream)
				}
				if c.removeStream(stream) {
					return nil, true, nil
				}
			}
			return nil, false, nil
		} else if err == io.EOF {
			return nil, true, nil
		} else if opErr, is := err.(*net.OpError); is && errors.Is(opErr.Err, net.ErrClosed) {
			return nil, true, nil
		}
		return nil, true, c.handleError(&TCPReadError{Err{Err: err}})
	}
	return readBufferPool.Alloc(uint(n)), false, nil
}

func (c *TCPChannel) start(lifecycle.Lifecycle) error {
	conn := c.conn.Get()
	if conn == nil {
		return errors.New("TCP连接已关闭")
	}
	return nil
}

func (c *TCPChannel) run(lifecycle.Lifecycle) error {
	defer func() {
		c.conn.SetClosed()
		if stream := c.stream.Swap(nil); stream != nil {
			c.closeStream(stream)
		}
		c.timer.Close()
		if c.packet != nil {
			c.packet.Release()
			c.packet = nil
		}
	}()

	keepChooser := NewDefaultKeepChooser(5, 5)

	// read and parse
	// c.parser.ID = '*' // GB28181 RTP over TCP
	// c.parser.ID = '$' // RTSP RTP over TCP
	for {
		data, closed, err := c.read()
		if closed {
			return err
		} else if data == nil {
			continue
		}

		for p := data.Data; len(p) > 0; {
			var ok bool
			if c.packet == nil {
				// 从RTP包的池中获取，并且添加至解析器
				c.packet = c.packetPool.Get().Use()
				c.parser.Layer = c.packet.Layer
			}
			if ok, p, err = c.parser.Parse(p); ok {
				// 解析RTP包成功
				c.packet.Chunks = append(c.packet.Chunks, data.Use())
				c.packet.Addr = c.remoteAddr
				c.packet.Time = time.Now()
				if stream := c.stream.Load(); stream != nil {
					// 将RTP包交给流处理，流处理器必须在使用完RTP包后将其释放
					dropped, keep := stream.HandlePacket(stream, c.packet)
					if !keep && c.removeStream(stream) {
						data.Release()
						return nil
					}
					if !dropped {
						stream.timer.SetTime(time.Now())
					}
					c.packet = nil
				} else {
					// 没有处理RTP包的流，释放RTP包中的数据，然后继续使用此RTP包作为接下来解析器解析
					// 的载体
					c.packet.Clear()
					keepChooser.OnSuccess()
				}
			} else if err != nil {
				// 解析RTP包出错，释放RTP包中的数据，如果可以继续解析则使用此RTP包作为接下来解析器解析
				// 的载体
				c.packet.Clear()
				if stream := c.stream.Load(); stream != nil {
					// 使用RTP流处理解析错误，并决定是否继续解析
					if !stream.OnParseError(stream, err) && c.removeStream(stream) {
						data.Release()
						return nil
					}
				} else {
					// 使用默认的策略决定是否继续解析
					if !keepChooser.OnError(err) {
						c.Close(nil)
						data.Release()
						return nil
					}
				}
			} else {
				// 解析RTP包未完成，需要将此块数据的引用添加到此RTP包中
				c.packet.Chunks = append(c.packet.Chunks, data.Use())
			}
		}

		data.Release()
	}
}

func (c *TCPChannel) close(lifecycle.Lifecycle) error {
	if _, err := c.conn.Close(); err != nil {
		return c.handleError(&TCPChannelCloseError{Err: Err{err}})
	}
	return nil
}

func (c *TCPChannel) Send(layer *rtp.Layer) error {
	conn := c.conn.Get()
	if conn != nil {
		size := layer.Size()
		data := c.writeBufferPool.Alloc(size)
		layer.Read(data.Data)
		_, err := conn.Write(data.Data)
		data.Release()
		return err
	}
	return nil
}

func (c *TCPChannel) bindStream(stream *TCPStream) bool {
	return lock.LockGet(&c.conn, func() bool {
		if c.conn.CloseFlag() {
			return false
		}
		// 如果此流之前由TCP服务管理，则可能含有超时定时器，需要先将定时器关闭
		stream.timer.Close()
		c.timer.Close()
		if old := c.stream.Swap(stream); old != nil {
			c.closeStream(old)
		}
		return true
	})
}

func (c *TCPChannel) closeStream(stream *TCPStream) {
	stream.channel.Store(nil)
	stream.server.Store(nil)
	stream.OnStreamClosed(stream)
}

func (c *TCPChannel) removeStream(stream *TCPStream) (closed bool) {
	if c.stream.CompareAndSwap(stream, nil) {
		c.closeStream(stream)
		if c.closeOnStreamClosed.Load() && stream.CloseConn() {
			c.Close(nil)
			return true
		}
		lock.LockDo(&c.conn, func() {
			if !c.conn.CloseFlag() {
				c.timer.Init(func() { c.Close(nil) })
			}
		})
	}
	return false
}
