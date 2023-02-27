package server

// A TCPChannelOption configures a TCPChannel.
//type TCPChannelOption interface {
//	apply(c *TCPChannel) any
//}

// tcpChannelOptionFunc wraps a func, so it satisfies the TCPChannelOption interface.
type tcpChannelOptionFunc func(c *TCPChannel) any

func (f tcpChannelOptionFunc) apply(c *TCPChannel) any {
	return f(c)
}

// WithTCPSocketBuffer option specify tcp connection socket read
// buffer and write buffer
//func WithTCPSocketBuffer(readBuffer, writeBuffer int) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		c.readBuffer, c.readBuffer = readBuffer, writeBuffer
//		return nil
//	})
//}

// WithKeepAlive option specify whether keepalive is enabled for
// tcp connection, default not enable
//func WithKeepAlive(keepAlive bool) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		c.keepAlive = keepAlive
//		return nil
//	})
//}

// WithKeepAlivePeriod option specify tcp connection keepalive period
// if keepalive not enable, this options will be ignored
//func WithKeepAlivePeriod(keepAlivePeriod time.Duration) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		c.keepAlivePeriod = keepAlivePeriod
//		return nil
//	})
//}

//func WithDisableNoDelay(disable bool) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		c.disableNoDelay = disable
//		return nil
//	})
//}

//func WithTCPChannelTimeout(timeout time.Duration) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		c.timeout = timeout
//		return nil
//	})
//}

//func WithTCPChannelBufferPool(bufferPool pool.BufferPool) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		c.bufferPool = bufferPool
//		return nil
//	})
//}

//func WithTCPChannelBufferPoolConfig(size uint, poolType string) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		if size > 0 {
//			c.bufferPool = pool.NewBufferPool(size, poolType)
//		}
//		return nil
//	})
//}

//func WithTCPChannelWritePool(pool pool.DataPool) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		c.writePool = pool
//		return nil
//	})
//}

//func WithTCPChannelWriteStaticBufferConfig(size uint, poolType string) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		if size > 0 {
//			c.writePool = pool.NewStaticDataPool(size, poolType)
//		}
//		return nil
//	})
//}

//func WithTCPChannelWriteDynamicBufferConfig(min, max uint, poolType string) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		c.writePool = pool.NewDynamicDataPoolExp(min, max, poolType)
//		return nil
//	})
//}

//func WithTCPChannelOnError(onError func(c *TCPChannel, err error)) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		c.onError = onError
//		return nil
//	})
//}

//func WithTCPChannelLogger(logger *log.Logger) TCPChannelOption {
//	return tcpChannelOptionFunc(func(c *TCPChannel) any {
//		c.SetLogger(logger)
//		return nil
//	})
//}
