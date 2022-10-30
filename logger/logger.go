package logger

import (
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/log"
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

func Logger() *log.Logger {
	return logger
}
