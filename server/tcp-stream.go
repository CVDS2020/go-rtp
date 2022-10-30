package server

import (
	"gitee.com/sy_183/rtp/rtp"
	"net"
	"sync/atomic"
)

type TCPStream struct {
	abstractStream[TCPServer, net.TCPAddr]
	channel atomic.Pointer[TCPChannel]
	addrID  string
}

func (s *TCPStream) Send(layer *rtp.Layer) error {
	channel := s.channel.Load()
	if channel != nil {
		return channel.Send(layer)
	}
	return nil
}
