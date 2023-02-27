package common

import (
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
)

func OnStarting(name string, logger *log.Logger) lifecycle.OnStartingFunc {
	return func(lifecycle.Lifecycle) {
		if logger != nil {
			logger.Info(name + "正在启动")
		}
	}
}

func OnStarted(name string, logger *log.Logger) lifecycle.OnStartedFunc {
	return func(_ lifecycle.Lifecycle, err error) {
		if logger != nil {
			if err != nil {
				logger.ErrorWith(name+"启动失败", err)
			} else {
				logger.Info(name + "启动成功")
			}
		}
	}
}

func OnClose(name string, logger *log.Logger) lifecycle.OnStartedFunc {
	return func(_ lifecycle.Lifecycle, err error) {
		if logger != nil {
			if err != nil {
				logger.ErrorWith(name+"执行关闭操作失败", err)
			} else {
				logger.Info(name + "执行关闭操作成功")
			}
		}
	}
}

func OnClosed(name string, logger *log.Logger) lifecycle.OnStartedFunc {
	return func(_ lifecycle.Lifecycle, err error) {
		if logger != nil {
			if err != nil {
				logger.ErrorWith(name+"出现错误并退出", err)
			} else {
				logger.Info(name + "已退出")
			}
		}
	}
}
