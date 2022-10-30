package server

import (
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"net"
)

type Server interface {
	lifecycle.Lifecycle

	log.LoggerProvider

	Name() string

	Addr() net.Addr

	Stream(remoteAddr net.Addr, ssrc int64, handler Handler) Stream

	RemoveStream(stream Stream)
}
