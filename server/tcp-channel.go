package server

import (
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"io"
	"net"
	"sync/atomic"
	"time"
)

type TCPChannel struct {
	lifecycle.OnceLifecycle
	runner *lifecycle.DefaultOnceRunner
	name   string

	server     *TCPServer
	conn       atomic.Pointer[net.TCPConn]
	remoteAddr *net.TCPAddr
	addrID     string

	stream atomic.Pointer[TCPStream]
	parser rtp.Parser
	packet *rtp.Packet

	readBuffer      int
	writeBuffer     int
	keepAlive       bool
	keepAlivePeriod time.Duration
	disableNoDelay  bool

	bufferPool    rtp.BufferPool
	bufferReverse uint
	packetPool    *pool.Pool[*rtp.Packet]
	writePool     *pool.DataPool

	onError     func(c *TCPChannel, err error)
	onClosed    func(c *TCPChannel)
	postHandler func(c *TCPChannel)

	log.AtomicLogger
}

func (c *TCPChannel) Name() string {
	return c.name
}

func (c *TCPChannel) handleError(err error) error {
	if c.onError != nil {
		c.onError(c, err)
	}
	return err
}

func (c *TCPChannel) run() error {
	buffer := c.bufferPool.Get()
	defer func() {
		if c.packet != nil {
			c.packet.Release()
			c.packet = nil
		}
		server := c.server
		server.mu.Lock()
		if server.channels[c.addrID] == c {
			delete(server.channels, c.addrID)
		}
		stream := c.stream.Load()
		if stream != nil {
			// stream is binding, unbinding stream and channel
			stream.channel.Store(nil)
			c.stream.Store(nil)
		}
		server.mu.Unlock()
		if stream != nil {
			stream.server.Store(nil)
			stream.OnStreamClosed(stream)
		}
		buffer.Release()
		c.conn.Store(nil)
		if c.onClosed != nil {
			c.onClosed(c)
		}
	}()

	conn := c.conn.Load()
	// set socket options
	if c.readBuffer != 0 {
		if err := conn.SetReadBuffer(c.readBuffer); err != nil {
			c.handleError(&SetReadBufferError{Err: Err{err}, ReadBuffer: c.readBuffer})
		}
	}
	if c.writeBuffer != 0 {
		if err := conn.SetWriteBuffer(c.writeBuffer); err != nil {
			c.handleError(&SetWriteBufferError{Err: Err{err}, WriteBuffer: c.writeBuffer})
		}
	}
	if c.keepAlive {
		if err := conn.SetKeepAlive(true); err != nil {
			c.handleError(&SetKeepaliveError{Err: Err{err}, Keepalive: true})
		} else if c.keepAlivePeriod != 0 {
			if err := conn.SetKeepAlivePeriod(c.keepAlivePeriod); err != nil {
				c.handleError(&SetKeepalivePeriodError{Err: Err{err}, KeepalivePeriod: c.keepAlivePeriod})
			}
		}
	}
	if c.disableNoDelay {
		if err := conn.SetNoDelay(false); err != nil {
			c.handleError(&SetNoDelayError{Err: Err{err}, NoDelay: false})
		}
	}

	// read and parse
	c.parser.ID = '*' // GB28181 RTP over TCP
	// c.parser.ID = '$' // RTSP RTP over TCP
	for {
		buf := buffer.Get()
		if uint(len(buf)) < c.bufferReverse {
			buffer.Release()
			buffer = c.bufferPool.Get()
			buf = buffer.Get()
		}
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				// peer close this connection
				return nil
			} else if opErr, is := err.(*net.OpError); is && opErr.Err.Error() == "use of closed network connection" {
				// self close this connection
				return nil
			}
			// other error, need handle
			c.handleError(&TCPReadError{Err{err}})
			return nil
		}
		data := buffer.Alloc(uint(n))

		for p := data.Data; len(p) > 0; {
			var ok bool
			if c.packet == nil {
				c.packet = c.packetPool.Get().Use()
				c.parser.Layer = c.packet.Layer
			}
			if ok, p, err = c.parser.Parse(p); ok {
				// parse completion and success
				c.packet.Chunks = append(c.packet.Chunks, data.Use())
				c.packet.Addr = c.remoteAddr
				c.packet.Time = time.Now()
				if stream := c.stream.Load(); stream != nil {
					// handle packet, handler must release this packet when
					// the use is completed
					stream.HandlePacket(stream, c.packet)
					c.packet = nil
				} else {
					// not stream handle this packet, parsing use this packet
					// continue
					c.packet.Clear()
				}
			} else if err != nil {
				// clear but not release packet, parsing use this packet continue
				c.packet.Clear()
			} else {
				// parse not completion, but need add data reference to packet
				c.packet.Chunks = append(c.packet.Chunks, data.Use())
			}
		}

		data.Release()
	}
}

func (c *TCPChannel) close() error {
	return c.conn.Load().Close()
}

func (c *TCPChannel) Send(layer *rtp.Layer) error {
	conn := c.conn.Load()
	if conn != nil {
		size := layer.Size()
		data := c.writePool.Alloc(size)
		layer.Read(data.Data)
		_, err := conn.Write(data.Data)
		data.Release()
		return err
	}
	return nil
}
