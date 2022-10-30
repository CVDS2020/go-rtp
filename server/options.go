package server

import (
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"net"
)

// An Option configures a Server.
type Option interface {
	apply(s Server) any
}

// optionFunc wraps a func, so it satisfies the Option interface.
type optionFunc func(s Server) any

func (f optionFunc) apply(s Server) any {
	return f(s)
}

// WithName option specify server name, if server is udp server,
// default name is "rtp udp server(<address>)", if server is tcp
// server, default name is "rtp tcp server(<address>)"
func WithName(name string) Option {
	type nameSetter interface {
		setName(name string)
	}
	return optionFunc(func(s Server) any {
		if setter, is := s.(nameSetter); is {
			setter.setName(name)
		}
		return nil
	})
}

// WithSocketBuffer option specify udp or tcp connection socket
// read buffer and write buffer
func WithSocketBuffer(readBuffer, writeBuffer int) Option {
	type socketBufferSetter interface {
		setSocketBuffer(readBuffer, writeBuffer int)
	}
	return optionFunc(func(s Server) any {
		if setter, is := s.(socketBufferSetter); is {
			setter.setSocketBuffer(readBuffer, writeBuffer)
		}
		return nil
	})
}

// WithBufferPoolSize option specify udp or tcp connection read buffer
// pool size, buffer pool create buffer, buffer alloc buf use for tcp
// or udp read
func WithBufferPoolSize(size uint) Option {
	type bufferPoolSetter interface {
		setBufferPool(bufferPool rtp.BufferPool)
	}
	type bufferPoolSizeSetter interface {
		setBufferPoolSize(size uint)
	}
	return optionFunc(func(s Server) any {
		if setter, is := s.(bufferPoolSetter); is && size > 0 {
			setter.setBufferPool(rtp.NewBufferPool(size))
		} else if setter, is := s.(bufferPoolSizeSetter); is {
			setter.setBufferPoolSize(size)
		}
		return nil
	})
}

func WithBufferReverse(reverse uint) Option {
	type bufferReverseSetter interface {
		setBufferReverse(reverse uint)
	}
	return optionFunc(func(s Server) any {
		if setter, is := s.(bufferReverseSetter); is {
			setter.setBufferReverse(reverse)
		}
		return nil
	})
}

type writePoolSetter interface {
	setWritePool(pool *pool.DataPool)
}

func WithWritePool(pool *pool.DataPool) Option {
	return optionFunc(func(s Server) any {
		if setter, is := s.(writePoolSetter); is {
			setter.setWritePool(pool)
		}
		return nil
	})
}

func WithWriteBufferSize(size uint) Option {
	type writeBufferSizeSetter interface {
		setWriteBufferSize(size uint)
	}
	return optionFunc(func(s Server) any {
		if setter, is := s.(writePoolSetter); is && size > 0 {
			setter.setWritePool(pool.NewDataPool(size))
		} else if setter, is := s.(writeBufferSizeSetter); is {
			setter.setWriteBufferSize(size)
		}
		return nil
	})
}

func WithOnError(onError func(s Server, err error)) Option {
	type onErrorSetter interface {
		setOnError(onError func(s Server, err error))
	}
	return optionFunc(func(s Server) any {
		if setter, is := s.(onErrorSetter); is {
			setter.setOnError(onError)
		}
		return nil
	})
}

func WithOnAccept(onAccept func(s *TCPServer, conn *net.TCPConn) []TCPChannelOption) Option {
	return optionFunc(func(s Server) any {
		if ts, is := s.(*TCPServer); is {
			ts.onAccept = onAccept
		}
		return nil
	})
}

func WithLogger(logger *log.Logger) Option {
	return optionFunc(func(s Server) any {
		s.SetLogger(logger)
		return nil
	})
}

func WithTCPChannelOptions(options ...TCPChannelOption) Option {
	return optionFunc(func(s Server) any {
		if ts, is := s.(*TCPServer); is {
			ts.channelOptions = append(ts.channelOptions, options...)
		}
		return nil
	})
}
