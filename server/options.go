package server

import (
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/option"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"net"
	"time"
)

func WithPacketPoolProvider(provider pool.PoolProvider[*rtp.IncomingPacket]) option.AnyOption {
	type packetPoolProviderSetter interface {
		setPacketPoolProvider(provider pool.PoolProvider[*rtp.IncomingPacket])
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(packetPoolProviderSetter); is {
			setter.setPacketPoolProvider(provider)
		}
	})
}

func WithReadBufferPool(bufferPool pool.BufferPool) option.AnyOption {
	type readBufferPoolSetter interface {
		setReadBufferPool(bufferPool pool.BufferPool)
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(readBufferPoolSetter); is {
			setter.setReadBufferPool(bufferPool)
		}
	})
}

func WithReadBufferPoolProvider(provider func() pool.BufferPool) option.AnyOption {
	type readBufferPoolProviderSetter interface {
		setReadBufferPoolProvider(provider func() pool.BufferPool)
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(readBufferPoolProviderSetter); is {
			setter.setReadBufferPoolProvider(provider)
		}
	})
}

func WithWriteBufferPool(bufferPool pool.DataPool) option.AnyOption {
	type writeBufferPoolSetter interface {
		setWriteBufferPool(bufferPool pool.DataPool)
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(writeBufferPoolSetter); is {
			setter.setWriteBufferPool(bufferPool)
		}
	})
}

func WithWriteBufferPoolProvider(provider func() pool.DataPool) option.AnyOption {
	type writeBufferPoolProvider interface {
		setWriteBufferPoolProvider(provider func() pool.DataPool)
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(writeBufferPoolProvider); is {
			setter.setWriteBufferPoolProvider(provider)
		}
	})
}

func WithTimeout(timeout time.Duration) option.AnyOption {
	type TimeoutSetter interface {
		setTimeout(timeout time.Duration)
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(TimeoutSetter); is {
			setter.setTimeout(timeout)
		}
	})
}

func WithOnError(onError func(s Server, err error)) option.AnyOption {
	type onErrorSetter interface {
		setOnError(onError func(s Server, err error))
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(onErrorSetter); is {
			setter.setOnError(onError)
		}
	})
}

func WithCloseOnStreamClosed(enable bool) option.AnyOption {
	type closeOnStreamClosedSetter interface {
		setCloseOnStreamClosed(enable bool)
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(closeOnStreamClosedSetter); is {
			setter.setCloseOnStreamClosed(enable)
		}
	})
}

func WithOnAccept(onAccept func(s *TCPServer, conn *net.TCPConn) []option.AnyOption) option.AnyOption {
	type onAcceptSetter interface {
		setOnAccept(onAccept func(s *TCPServer, conn *net.TCPConn) []option.AnyOption)
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(onAcceptSetter); is {
			setter.setOnAccept(onAccept)
		}
	})
}

func WithOnChannelCreated(onChannelCreated func(s *TCPServer, channel *TCPChannel)) option.AnyOption {
	type onChannelCreatedSetter interface {
		setOnChannelCreated(onChannelCreated func(s *TCPServer, channel *TCPChannel))
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(onChannelCreatedSetter); is {
			setter.setOnChannelCreated(onChannelCreated)
		}
	})
}

func WithOnChannelError(onError func(c *TCPChannel, err error)) option.AnyOption {
	type onChannelErrorSetter interface {
		setOnError(func(channel *TCPChannel, err error))
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(onChannelErrorSetter); is {
			setter.setOnError(onError)
		}
	})
}

func WithOnChannelTimeout(onTimeout func(*TCPChannel)) option.AnyOption {
	type onChannelTimeoutSetter interface {
		setOnTimeout(func(*TCPChannel))
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(onChannelTimeoutSetter); is {
			setter.setOnTimeout(onTimeout)
		}
	})
}

func WithChannelOptions(options ...option.AnyOption) option.AnyOption {
	return option.AnyCustom(func(target any) {
		if ts, is := target.(*TCPServer); is {
			ts.channelOptions = append(ts.channelOptions, options...)
		}
	})
}

func WithOnStreamTimeout(onTimeout func(Stream)) option.AnyOption {
	type onStreamTimeoutSetter interface {
		setOnTimeout(func(Stream))
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(onStreamTimeoutSetter); is {
			setter.setOnTimeout(onTimeout)
		}
	})
}

func WithOnLossPacket(onLossPacket func(stream Stream, loss int)) option.AnyOption {
	type onPacketLossSetter interface {
		setOnLossPacket(onLossPacket func(stream Stream, loss int))
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(onPacketLossSetter); is {
			setter.setOnLossPacket(onLossPacket)
		}
	})
}

func WithStreamCloseConn(closeConn bool) option.AnyOption {
	type streamCloseConnSetter interface {
		setCloseConn(closeConn bool)
	}
	return option.AnyCustom(func(target any) {
		if setter, is := target.(streamCloseConnSetter); is {
			setter.setCloseConn(closeConn)
		}
	})
}

func WithLogger(logger *log.Logger) option.AnyOption {
	return option.AnyCustom(func(target any) {
		if lp, is := target.(log.LoggerProvider); is {
			lp.SetLogger(logger)
		}
	})
}
