package server

import "gitee.com/sy_183/rtp/rtp"

type Handler interface {
	HandlePacket(stream Stream, packet *rtp.Packet)
	OnStreamClosed(stream Stream)
}

type HandlerFunc struct {
	HandlePacketFn   func(stream Stream, packet *rtp.Packet)
	OnStreamClosedFn func(stream Stream)
}

func (h HandlerFunc) HandlePacket(stream Stream, packet *rtp.Packet) {
	if handlePacketFn := h.HandlePacketFn; handlePacketFn != nil {
		handlePacketFn(stream, packet)
	} else {
		packet.Release()
	}
}

func (h HandlerFunc) OnStreamClosed(stream Stream) {
	if onStreamClosedFn := h.OnStreamClosedFn; onStreamClosedFn != nil {
		onStreamClosedFn(stream)
	}
}
