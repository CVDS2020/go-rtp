package server

import (
	"gitee.com/sy_183/rtp/rtp"
	"net"
	"sync/atomic"
)

type TCPStream struct {
	abstractStream[TCPServer, net.TCPAddr]
	channel atomic.Pointer[TCPChannel]
	addrId  string
}

func newTCPStream(addrId string, server *TCPServer, localAddr *net.TCPAddr, remoteAddr net.Addr, ssrc int64, handler Handler, options ...Option) *TCPStream {
	stream := &TCPStream{addrId: addrId}
	stream.init(stream, server, localAddr, remoteAddr, ssrc, handler, options...)
	return stream
}

func (s *TCPStream) Send(layer *rtp.Layer) error {
	channel := s.channel.Load()
	if channel != nil {
		return channel.Send(layer)
	}
	return nil
}
