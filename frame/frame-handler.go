package frame

import (
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"gitee.com/sy_183/rtp/server"
)

type FrameHandler interface {
	HandleFrame(stream server.Stream, frame *Frame)

	OnParseRTPError(stream server.Stream, err error) (keep bool)

	OnStreamClosed(stream server.Stream)
}

type FrameHandlerFunc struct {
	HandleFrameFn     func(stream server.Stream, frame *Frame)
	OnParseRTPErrorFn func(stream server.Stream, err error) (keep bool)
	OnStreamClosedFn  func(stream server.Stream)
}

func (h FrameHandlerFunc) HandleFrame(stream server.Stream, frame *Frame) {
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
	cur          *Frame
	frameHandler FrameHandler
	framePool    *pool.SyncPool[*Frame]
}

func NewFrameRTPHandler(frameHandler FrameHandler) *FrameRTPHandler {
	return &FrameRTPHandler{
		frameHandler: frameHandler,
		framePool: pool.NewSyncPool(func(p *pool.SyncPool[*Frame]) *Frame {
			return &Frame{
				Layer: new(Layer),
				pool:  p,
			}
		}),
	}
}

func (h *FrameRTPHandler) HandlePacket(stream server.Stream, packet *rtp.Packet) (dropped, keep bool) {
	defer packet.Release()
	if h.cur != nil && (packet.Timestamp() != h.cur.Timestamp || packet.PayloadType() != h.cur.PayloadType) {
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
	if h.cur != nil && len(h.cur.Packets) != 0 {
		h.frameHandler.HandleFrame(stream, h.cur)
	}
	h.frameHandler.OnStreamClosed(stream)
}
