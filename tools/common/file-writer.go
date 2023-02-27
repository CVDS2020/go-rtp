package common

import (
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lifecycle/task"
	"gitee.com/sy_183/common/log"
	"os"
	"sync/atomic"
)

type FileWriter struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	file         string
	fp           *os.File
	taskExecutor *task.TaskExecutor
	buffer1      []byte
	buffer2      atomic.Pointer[[]byte]

	log.AtomicLogger
}

func (w *FileWriter) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	fp, err := os.OpenFile(w.file, os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0644)
	if err != nil {
		return w.Logger().ErrorWith("打开文件失败", err)
	}
	w.fp = fp
	w.taskExecutor.StartFunc()(w, interrupter)
	return nil
}

func (w *FileWriter) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	w.taskExecutor.RunFunc()(w, interrupter)
	if err := w.fp.Close(); err != nil {
		w.Logger().ErrorWith("关闭文件失败", err)
	}
	return nil
}

func (w *FileWriter) Write(data []byte) {
	if len(data) > cap(w.buffer1) {

		if ok, err := w.taskExecutor.Try(task.Func(func() {
			if len(w.buffer1) > 0 {
				_, err := w.fp.Write(w.buffer1)
				if err != nil {
					w.Logger().ErrorWith("写文件出错", err)
					return
				}
			}
			_, err := w.fp.Write(data)
			if err != nil {
				w.Logger().ErrorWith("写文件出错", err)
			}
		})); err != nil {
			w.Logger().ErrorWith("尝试写文件失败", err)
		} else if !ok {
			w.Logger().Warn("写文件任务已满")
		}
	}
}
