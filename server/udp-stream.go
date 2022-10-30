package server

import (
	"gitee.com/sy_183/rtp/rtp"
	"net"
)

type UDPStream struct {
	abstractStream[UDPServer, net.UDPAddr]
}

func (s *UDPStream) Send(layer *rtp.Layer) error {
	server := s.server.Load()
	if server != nil {
		return server.SendTo(layer, s.remoteAddr.Load())
	}
	return nil
}
