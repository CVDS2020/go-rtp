package server

import (
	"gitee.com/sy_183/rtp/rtp"
)

type Handler interface {
	HandlePacket(stream Stream, packet *rtp.IncomingPacket) (dropped, keep bool)

	OnParseError(stream Stream, err error) (keep bool)

	OnStreamClosed(stream Stream)
}

type HandlerFunc struct {
	HandlePacketFn   func(stream Stream, packet *rtp.IncomingPacket) (dropped, keep bool)
	OnParseErrorFn   func(stream Stream, err error) (keep bool)
	OnStreamClosedFn func(stream Stream)
}

func (h HandlerFunc) HandlePacket(stream Stream, packet *rtp.IncomingPacket) (dropped, keep bool) {
	if handlePacketFn := h.HandlePacketFn; handlePacketFn != nil {
		return handlePacketFn(stream, packet)
	} else {
		packet.Release()
		return true, true
	}
}

func (h HandlerFunc) OnParseError(stream Stream, err error) (keep bool) {
	if onParseErrorFn := h.OnParseErrorFn; onParseErrorFn != nil {
		return onParseErrorFn(stream, err)
	}
	return true
}

func (h HandlerFunc) OnStreamClosed(stream Stream) {
	if onStreamClosedFn := h.OnStreamClosedFn; onStreamClosedFn != nil {
		onStreamClosedFn(stream)
	}
}
