package server

import (
	"gitee.com/sy_183/common/option"
	"gitee.com/sy_183/rtp/rtp"
	"net"
)

type UDPStream struct {
	abstractStream[UDPServer, net.UDPAddr]
}

func newUDPStream(server *UDPServer, localAddr *net.UDPAddr, remoteAddr net.Addr, ssrc int64, handler Handler, options ...option.AnyOption) *UDPStream {
	stream := new(UDPStream)
	stream.init(stream, server, localAddr, remoteAddr, ssrc, handler, options...)
	return stream
}

func (s *UDPStream) Send(layer rtp.Layer) error {
	server := s.server.Load()
	if server != nil {
		return server.SendTo(layer, s.remoteAddr.Load())
	}
	return nil
}
