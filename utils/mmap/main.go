package main

import (
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/common/uns"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"
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

type Mmap struct {
	fp   *os.File
	fd   uintptr
	data []byte
	prt  uintptr
	mu   sync.Mutex
}

func Open(name string, flag int, perm os.FileMode) (mmap *Mmap, err error) {
	fp, err := os.OpenFile(name, flag|os.O_RDWR, perm)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Append(err, fp.Close())
		}
	}()

	info, err := fp.Stat()
	if err != nil {
		return nil, err
	}

	m := &Mmap{
		fp: fp,
		fd: fp.Fd(),
	}

	if size := info.Size(); size > 0 {
		if err := m.doMmap(uintptr(size)); err != nil {
			return nil, err
		}
	}

	return m, nil
}

func (m *Mmap) doMmap(length uintptr) error {
	logger.Debug("mmap", log.Int("length", int(length)))
	ptr, _, errno := syscall.Syscall6(syscall.SYS_MMAP, 0, length, syscall.PROT_WRITE|syscall.PROT_READ, syscall.MAP_SHARED, m.fd, 0)
	if errno != 0 {
		return errno
	}

	m.prt = ptr
	m.data = uns.MakeBytes(unsafe.Pointer(ptr), int(length), int(length))
	return nil
}

func (m *Mmap) doMRemap(length uintptr) error {
	if int(length) == len(m.data) {
		return nil
	}
	logger.Debug("mremap", log.Int("oldLength", len(m.data)), log.Int("newLength", int(length)))
	ptr, _, errno := syscall.Syscall6(syscall.SYS_MREMAP, m.prt, uintptr(len(m.data)), length, 1, 0, 0)
	if errno != 0 {
		return errno
	}

	m.prt = ptr
	m.data = uns.MakeBytes(unsafe.Pointer(ptr), int(length), int(length))
	return nil
}

func (m *Mmap) doGrow(size int) error {
	if size > len(m.data) {
		logger.Debug("truncate", log.Int("size", size))
		if err := m.fp.Truncate(int64(size)); err != nil {
			return err
		}
	}
	if m.prt == 0 {
		return m.doMmap(uintptr(size))
	}
	if size > len(m.data) {
		return m.doMRemap(uintptr(size))
	}
	return nil
}

func (m *Mmap) doSync() error {
	if m.prt == 0 {
		return nil
	}
	logger.Debug("sync")
	_, _, errno := syscall.Syscall(syscall.SYS_MSYNC, m.prt, uintptr(len(m.data)), syscall.MS_SYNC)
	if errno != 0 {
		return errno
	}
	return nil
}

func (m *Mmap) Grow(size int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doGrow(size)
}

func (m *Mmap) Write(data []byte, offset int, sync bool) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	size := len(data) + offset
	if err := m.doGrow(size); err != nil {
		return 0, err
	}

	if len(data) > 0 {
		copy(m.data[offset:size], data)
	}
	if sync {
		if err := m.doSync(); err != nil {
			return 0, err
		}
	}
	return size, nil
}

func (m *Mmap) Sync() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doSync()
}

func (m *Mmap) Close() (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.prt != 0 {
		logger.Debug("munmap")
		if _, _, errno := syscall.Syscall(syscall.SYS_MUNMAP, m.prt, uintptr(len(m.data)), 0); errno != 0 {
			err = errno
		}
	}
	if e := m.fp.Close(); e != nil {
		return errors.Append(err, e)
	}
	return nil
}

func main() {
	mmap, err := Open(os.Args[1], os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		logger.Fatal("打开文件失败", log.Error(err))
	}

	if err = mmap.Grow(4096 * unit.MeBiByte); err != nil {
		logger.Fatal("扩充文件失败", log.Error(err))
	}

	defer func() {
		if err := mmap.Close(); err != nil {
			logger.ErrorWith("关闭文件失败", err)
		}
	}()

	var offset int
	for i := 0; i < 4096; i++ {
		offset, err = mmap.Write(make([]byte, unit.MeBiByte), offset, false)
		if err != nil {
			logger.Fatal("写文件失败", log.Error(err))
		}
		logger.Info("写文件成功", log.Int("offset", offset))
		//if i%10 == 0 {
		//	if err := mmap.Sync(); err != nil {
		//		logger.Fatal("同步文件失败", log.Error(err))
		//	}
		//}
		time.Sleep(time.Millisecond * 400)
	}
}
