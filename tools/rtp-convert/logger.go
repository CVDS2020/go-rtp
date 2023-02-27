package main

import (
	"gitee.com/sy_183/common/log"
)

const DefaultTimeLayout = "2006-01-02 15:04:05.999999999"

var logger *log.Logger

func Logger() *log.Logger {
	return logger
}
