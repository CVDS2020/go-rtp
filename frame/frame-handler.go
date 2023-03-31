package frame

import (
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"gitee.com/sy_183/rtp/server"
)

type FrameHandler interface {
	HandleFrame(stream server.Stream, frame *IncomingFrame)

	OnParseRTPError(stream server.Stream, err error) (keep bool)

	OnStreamClosed(stream server.Stream)
}

type FrameHandlerFunc struct {
	HandleFrameFn     func(stream server.Stream, frame *IncomingFrame)
	OnParseRTPErrorFn func(stream server.Stream, err error) (keep bool)
	OnStreamClosedFn  func(stream server.Stream)
}

func (h FrameHandlerFunc) HandleFrame(stream server.Stream, frame *IncomingFrame) {
	if handleFrameFn := h.HandleFrameFn; handleFrameFn != nil {
		handleFrameFn(stream, frame)
	}
}

func (h FrameHandlerFunc) OnParseRTPError(stream server.Stream, err error) (keep bool) {
	if onParseRTPErrorFn := h.OnParseRTPErrorFn; onParseRTPErrorFn != nil {
		return onParseRTPErrorFn(stream, err)
	}
	return true
}

func (h FrameHandlerFunc) OnStreamClosed(stream server.Stream) {
	if onStreamClosedFn := h.OnStreamClosedFn; onStreamClosedFn != nil {
		onStreamClosedFn(stream)
	}
}

type FrameRTPHandler struct {
	cur          *IncomingFrame
	frameHandler FrameHandler
	framePool    *pool.SyncPool[*IncomingFrame]
}

func newFrame(p *pool.SyncPool[*IncomingFrame]) *IncomingFrame {
	return &IncomingFrame{pool: p}
}

func NewFrameRTPHandler(frameHandler FrameHandler) *FrameRTPHandler {
	return &FrameRTPHandler{
		frameHandler: frameHandler,
		framePool:    pool.NewSyncPool(newFrame),
	}
}

func (h *FrameRTPHandler) HandlePacket(stream server.Stream, packet *rtp.IncomingPacket) (dropped, keep bool) {
	if h.cur != nil && (packet.Timestamp() != h.cur.Timestamp() || packet.PayloadType() != h.cur.PayloadType()) {
		h.frameHandler.HandleFrame(stream, h.cur)
		h.cur = nil
	}
	if h.cur == nil {
		h.cur = h.framePool.Get().Use()
	}
	h.cur.Append(packet)
	return false, true
}

func (h *FrameRTPHandler) OnParseError(stream server.Stream, err error) (keep bool) {
	return h.frameHandler.OnParseRTPError(stream, err)
}

func (h *FrameRTPHandler) OnStreamClosed(stream server.Stream) {
	if h.cur != nil && h.cur.Len() != 0 {
		h.frameHandler.HandleFrame(stream, h.cur)
		h.cur = nil
	}
	h.frameHandler.OnStreamClosed(stream)
}
