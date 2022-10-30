package frame

import (
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/rtp/rtp"
	"gitee.com/sy_183/rtp/server"
)

type FrameHandler interface {
	HandleFrame(stream server.Stream, frame *Frame)
	OnStreamClosed(stream server.Stream)
}

type FrameHandlerFunc struct {
	HandleFrameFn    func(stream server.Stream, frame *Frame)
	OnStreamClosedFn func(stream server.Stream)
}

func (h FrameHandlerFunc) HandleFrame(stream server.Stream, frame *Frame) {
	if handleFrameFn := h.HandleFrameFn; handleFrameFn != nil {
		handleFrameFn(stream, frame)
	}
}

func (h FrameHandlerFunc) OnStreamClosed(stream server.Stream) {
	if onStreamClosedFn := h.OnStreamClosedFn; onStreamClosedFn != nil {
		onStreamClosedFn(stream)
	}
}

type FrameRTPHandler struct {
	cur          *Frame
	frameHandler FrameHandler
	framePool    *pool.Pool[*Frame]
}

func NewFrameRTPHandler(frameHandler FrameHandler) *FrameRTPHandler {
	return &FrameRTPHandler{
		frameHandler: frameHandler,
		framePool: pool.NewPool(func(p *pool.Pool[*Frame]) *Frame {
			return &Frame{
				pool: p,
			}
		}),
	}
}

func (h *FrameRTPHandler) HandlePacket(stream server.Stream, packet *rtp.Packet) {
	defer packet.Release()
	if h.cur != nil && packet.Timestamp() != h.cur.Timestamp {
		h.frameHandler.HandleFrame(stream, h.cur)
		h.cur = nil
	}
	if h.cur == nil {
		h.cur = h.framePool.Get().Use()
	}
	h.cur.Append(packet)
}

func (h *FrameRTPHandler) OnStreamClosed(stream server.Stream) {
	if h.cur != nil && len(h.cur.Packets) != 0 {
		h.frameHandler.HandleFrame(stream, h.cur)
	}
	h.frameHandler.OnStreamClosed(stream)
}
