package server

import (
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/log"
	"io"
	"net"
	"testing"
)

const DefaultTimeLayout = "2006-01-02 15:04:05.999999999"

var logger = assert.Must(log.Config{
	Level: log.NewAtomicLevelAt(log.DebugLevel),
	Encoder: log.NewConsoleEncoder(log.ConsoleEncoderConfig{
		DisableCaller:     true,
		DisableFunction:   true,
		DisableStacktrace: true,
		EncodeLevel:       log.CapitalColorLevelEncoder,
		EncodeTime:        log.TimeEncoderOfLayout(DefaultTimeLayout),
		EncodeDuration:    log.SecondsDurationEncoder,
	}),
}.Build())

func TestConn(t *testing.T) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IP{0, 0, 0, 0}, Port: 5904})
	if err != nil {
		logger.Fatal("监听TCP连接错误", log.Error(err))
	}
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			logger.Fatal("接收TCP连接错误", log.Error(err))
		}
		logger.Info("接收到新的TCP连接", log.String("本端地址", conn.LocalAddr().String()), log.String("对端地址", conn.RemoteAddr().String()))
		go func() {
			_, err := io.Copy(io.Discard, conn)
			if err != nil {
				logger.ErrorWith("读取TCP连接数据错误", err)
				return
			}
			logger.Info("TCP连接已关闭")
		}()
	}
}
