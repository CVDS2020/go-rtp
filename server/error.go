package server

import (
	"errors"
	"net"
	"time"
)

var (
	TooManyParseError  = errors.New("RTP连续解析错误太多")
	FrequentParseError = errors.New("RTP解析错误太频繁")
	BufferAllocError   = errors.New("RTP缓冲区申请失败")
)

type Err struct {
	Err error
}

func (e *Err) Error() string { return e.Err.Error() }

type UDPListenError struct {
	Err
	Addr *net.UDPAddr
}

type UDPCloseError struct {
	Err
	Listener *net.UDPConn
}

type TCPListenError struct {
	Err
	Addr *net.TCPAddr
}

type TCPCloseError struct {
	Err
	Listener *net.TCPListener
}

type TCPAcceptError struct {
	Err
}

type SetReadBufferError struct {
	Err
	ReadBuffer int
}

type SetWriteBufferError struct {
	Err
	WriteBuffer int
}

type SetKeepaliveError struct {
	Err
	Keepalive bool
}

type SetKeepalivePeriodError struct {
	Err
	KeepalivePeriod time.Duration
}

type SetNoDelayError struct {
	Err
	NoDelay bool
}

type TCPChannelCloseError struct {
	Err
}

type UDPReadError struct {
	Err
}

type TCPReadError struct {
	Err
}

type TCPReadTimeout struct {
	Err
}
