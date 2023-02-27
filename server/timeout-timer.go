package server

import (
	"gitee.com/sy_183/common/lock"
	"sync"
	"sync/atomic"
	"time"
)

type timeoutTimer struct {
	time      time.Time
	timeout   atomic.Int64
	timer     *time.Timer
	timerLock sync.Mutex
}

func (t *timeoutTimer) Time() time.Time {
	return t.time
}

func (t *timeoutTimer) SetTime(time time.Time) {
	t.time = time
}

func (t *timeoutTimer) Timeout() time.Duration {
	return time.Duration(t.timeout.Load())
}

func (t *timeoutTimer) Deadline() time.Time {
	if timeout := t.Timeout(); timeout > 0 {
		return t.time.Add(timeout)
	}
	return time.Time{}
}

func (t *timeoutTimer) setTimeout(timeout time.Duration) {
	t.timeout.Store(int64(timeout))
}

func (t *timeoutTimer) SetTimeout(timeout time.Duration) {
	lock.LockDo(&t.timerLock, func() {
		t.setTimeout(timeout)
		if t.timer != nil {
			if timeout <= 0 {
				t.timer.Stop()
				t.timer = nil
			} else {
				deadline := t.time.Add(timeout)
				if now := time.Now(); deadline.After(now) {
					t.timer.Reset(deadline.Sub(now))
				} else {
					t.timer.Reset(0)
				}
			}
		}
	})
}

func (t *timeoutTimer) Init(f func()) {
	lock.LockDo(&t.timerLock, func() {
		if t.timer != nil {
			return
		}
		t.SetTime(time.Now())
		if timeout := t.Timeout(); timeout > 0 {
			t.timer = time.AfterFunc(timeout, func() {
				f()
				t.Close()
			})
		}
	})
}

func (t *timeoutTimer) Close() {
	lock.LockDo(&t.timerLock, func() {
		if t.timer != nil {
			t.timer.Stop()
			t.timer = nil
		}
	})
}
