package server

import (
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/rtp/rtp"
	"time"
)

type KeepChooser interface {
	OnSuccess()

	OnError(err error) (keep bool)

	Reset()
}

type defaultKeepChooser struct {
	secErrorCounter  *container.Queue[time.Time]
	maxSerializedErr int
	serializedErrors int
}

func NewDefaultKeepChooser(secMaxErr int, maxSerializedErr int) KeepChooser {
	if secMaxErr < 0 {
		secMaxErr = 5
	} else if secMaxErr > 100 {
		secMaxErr = 100
	}
	if maxSerializedErr < 0 {
		maxSerializedErr = 5
	}
	return &defaultKeepChooser{
		secErrorCounter:  container.NewQueue[time.Time](secMaxErr),
		maxSerializedErr: maxSerializedErr,
	}
}

func (c *defaultKeepChooser) OnSuccess() {
	if c.serializedErrors > 0 {
		c.serializedErrors = 0
	}
}

func (c *defaultKeepChooser) OnError(err error) (keep bool) {
	if c.serializedErrors++; c.serializedErrors > c.maxSerializedErr {
		return false
	}
	now := time.Now()
	for {
		head, exist := c.secErrorCounter.Head()
		if exist && now.Sub(head) > time.Second {
			c.secErrorCounter.Pop()
		} else {
			break
		}
	}
	if !c.secErrorCounter.Push(now) {
		return false
	}
	return true
}

func (c *defaultKeepChooser) Reset() {
	c.secErrorCounter.Clear()
	c.serializedErrors = 0
}

type keepChooserHandler struct {
	handler     Handler
	keepChooser KeepChooser
}

func KeepChooserHandler(handler Handler, chooser KeepChooser) Handler {
	return keepChooserHandler{handler: handler, keepChooser: chooser}
}

func DefaultKeepChooserHandler(handler Handler, secMaxErr int, maxSerializedErr int) Handler {
	return keepChooserHandler{handler: handler, keepChooser: NewDefaultKeepChooser(secMaxErr, maxSerializedErr)}
}

func (h keepChooserHandler) HandlePacket(stream Stream, packet *rtp.Packet) (dropped, keep bool) {
	h.keepChooser.OnSuccess()
	return h.handler.HandlePacket(stream, packet)
}

func (h keepChooserHandler) OnParseError(stream Stream, err error) (keep bool) {
	if !h.handler.OnParseError(stream, err) {
		return false
	}
	return h.keepChooser.OnError(err)
}

func (h keepChooserHandler) OnStreamClosed(stream Stream) {
	h.handler.OnStreamClosed(stream)
	h.keepChooser.Reset()
}
