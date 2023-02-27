package server

import (
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"net"
	"time"
)

// An Option configures a Server.
type Option interface {
	apply(target any) any
}

// optionFunc wraps a func, so it satisfies the Option interface.
type optionFunc func(target any) any

func (f optionFunc) apply(target any) any {
	return f(target)
}

func WithPacketPoolProvider(provider pool.PoolProvider[*rtp.Packet]) Option {
	type packetPoolProviderSetter interface {
		setPacketPoolProvider(provider pool.PoolProvider[*rtp.Packet])
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(packetPoolProviderSetter); is {
			setter.setPacketPoolProvider(provider)
		}
		return nil
	})
}

func WithReadBufferPool(bufferPool pool.BufferPool) Option {
	type readBufferPoolSetter interface {
		setReadBufferPool(bufferPool pool.BufferPool)
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(readBufferPoolSetter); is {
			setter.setReadBufferPool(bufferPool)
		}
		return nil
	})
}

func WithReadBufferPoolProvider(provider func() pool.BufferPool) Option {
	type readBufferPoolProviderSetter interface {
		setReadBufferPoolProvider(provider func() pool.BufferPool)
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(readBufferPoolProviderSetter); is {
			setter.setReadBufferPoolProvider(provider)
		}
		return nil
	})
}

func WithWriteBufferPool(bufferPool pool.DataPool) Option {
	type writeBufferPoolSetter interface {
		setWriteBufferPool(bufferPool pool.DataPool)
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(writeBufferPoolSetter); is {
			setter.setWriteBufferPool(bufferPool)
		}
		return nil
	})
}

func WithWriteBufferPoolProvider(provider func() pool.DataPool) Option {
	type writeBufferPoolProvider interface {
		setWriteBufferPoolProvider(provider func() pool.DataPool)
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(writeBufferPoolProvider); is {
			setter.setWriteBufferPoolProvider(provider)
		}
		return nil
	})
}

func WithTimeout(timeout time.Duration) Option {
	type TimeoutSetter interface {
		setTimeout(timeout time.Duration)
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(TimeoutSetter); is {
			setter.setTimeout(timeout)
		}
		return nil
	})
}

func WithOnError(onError func(s Server, err error)) Option {
	type onErrorSetter interface {
		setOnError(onError func(s Server, err error))
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(onErrorSetter); is {
			setter.setOnError(onError)
		}
		return nil
	})
}

func WithCloseOnStreamClosed(enable bool) Option {
	type closeOnStreamClosedSetter interface {
		setCloseOnStreamClosed(enable bool)
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(closeOnStreamClosedSetter); is {
			setter.setCloseOnStreamClosed(enable)
		}
		return nil
	})
}

func WithOnAccept(onAccept func(s *TCPServer, conn *net.TCPConn) []Option) Option {
	type onAcceptSetter interface {
		setOnAccept(onAccept func(s *TCPServer, conn *net.TCPConn) []Option)
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(onAcceptSetter); is {
			setter.setOnAccept(onAccept)
		}
		return nil
	})
}

func WithOnChannelCreated(onChannelCreated func(s *TCPServer, channel *TCPChannel)) Option {
	type onChannelCreatedSetter interface {
		setOnChannelCreated(onChannelCreated func(s *TCPServer, channel *TCPChannel))
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(onChannelCreatedSetter); is {
			setter.setOnChannelCreated(onChannelCreated)
		}
		return nil
	})
}

func WithOnChannelError(onError func(c *TCPChannel, err error)) Option {
	type onChannelErrorSetter interface {
		setOnError(func(channel *TCPChannel, err error))
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(onChannelErrorSetter); is {
			setter.setOnError(onError)
		}
		return nil
	})
}

func WithOnChannelTimeout(onTimeout func(*TCPChannel)) Option {
	type onChannelTimeoutSetter interface {
		setOnTimeout(func(*TCPChannel))
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(onChannelTimeoutSetter); is {
			setter.setOnTimeout(onTimeout)
		}
		return nil
	})
}

func WithChannelOptions(options ...Option) Option {
	return optionFunc(func(target any) any {
		if ts, is := target.(*TCPServer); is {
			ts.channelOptions = append(ts.channelOptions, options...)
		}
		return nil
	})
}

func WithOnStreamTimeout(onTimeout func(Stream)) Option {
	type onStreamTimeoutSetter interface {
		setOnTimeout(func(Stream))
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(onStreamTimeoutSetter); is {
			setter.setOnTimeout(onTimeout)
		}
		return nil
	})
}

func WithOnLossPacket(onLossPacket func(stream Stream, loss int)) Option {
	type onPacketLossSetter interface {
		setOnLossPacket(onLossPacket func(stream Stream, loss int))
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(onPacketLossSetter); is {
			setter.setOnLossPacket(onLossPacket)
		}
		return nil
	})
}

func WithStreamCloseConn(closeConn bool) Option {
	type streamCloseConnSetter interface {
		setCloseConn(closeConn bool)
	}
	return optionFunc(func(target any) any {
		if setter, is := target.(streamCloseConnSetter); is {
			setter.setCloseConn(closeConn)
		}
		return nil
	})
}

func WithLogger(logger *log.Logger) Option {
	return optionFunc(func(target any) any {
		if lp, is := target.(log.LoggerProvider); is {
			lp.SetLogger(logger)
		}
		return nil
	})
}
