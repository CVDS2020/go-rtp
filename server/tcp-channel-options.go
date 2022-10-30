package server

import (
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"time"
)

// A TCPChannelOption configures a TCPChannel.
type TCPChannelOption interface {
	apply(c *TCPChannel) any
}

// tcpChannelOptionFunc wraps a func, so it satisfies the TCPChannelOption interface.
type tcpChannelOptionFunc func(c *TCPChannel) any

func (f tcpChannelOptionFunc) apply(c *TCPChannel) any {
	return f(c)
}

func WithTCPChannelName(name string) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		c.name = name
		return nil
	})
}

// WithTCPSocketBuffer option specify tcp connection socket read
// buffer and write buffer
func WithTCPSocketBuffer(readBuffer, writeBuffer int) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		c.readBuffer, c.readBuffer = readBuffer, writeBuffer
		return nil
	})
}

// WithKeepAlive option specify whether keepalive is enabled for
// tcp connection, default not enable
func WithKeepAlive(keepAlive bool) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		c.keepAlive = keepAlive
		return nil
	})
}

// WithKeepAlivePeriod option specify tcp connection keepalive period
// if keepalive not enable, this options will be ignored
func WithKeepAlivePeriod(keepAlivePeriod time.Duration) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		c.keepAlivePeriod = keepAlivePeriod
		return nil
	})
}

func WithDisableNoDelay(disable bool) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		c.disableNoDelay = disable
		return nil
	})
}

// WithTCPChannelBufferPoolSize option specify tcp connection read buffer
// pool size, buffer pool create buffer, buffer alloc buf use for tcp
// or udp read
func WithTCPChannelBufferPoolSize(size uint) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		if size > 0 {
			c.bufferPool = rtp.NewBufferPool(size)
		}
		return nil
	})
}

func WithTCPChannelBufferReverse(reverse uint) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		c.bufferReverse = reverse
		return nil
	})
}

func WithTCPChannelWritePool(pool *pool.DataPool) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		c.writePool = pool
		return nil
	})
}

func WithTCPChannelWriteBufferSize(size uint) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		if size > 0 {
			c.writePool = pool.NewDataPool(size)
		}
		return nil
	})
}

func WithTCPChannelOnError(onError func(c *TCPChannel, err error)) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		c.onError = onError
		return nil
	})
}

func WithTCPChannelOnClosed(onClosed func(channel *TCPChannel)) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		c.onClosed = onClosed
		return nil
	})
}

func WithTCPChannelLogger(logger *log.Logger) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		c.SetLogger(logger)
		return nil
	})
}

func WithTCPChannelPostHandler(handler func(c *TCPChannel)) TCPChannelOption {
	return tcpChannelOptionFunc(func(c *TCPChannel) any {
		c.postHandler = handler
		return nil
	})
}
