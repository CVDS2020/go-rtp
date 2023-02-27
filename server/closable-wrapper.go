package server

import (
	"gitee.com/sy_183/common/lock"
	"io"
	"sync"
	"sync/atomic"
)

type closableWrapper[T any] struct {
	closable atomic.Pointer[T]
	close    atomic.Bool
	sync.Mutex
}

func (c *closableWrapper[T]) InitUnlock(provider func() (*T, error), replace bool) (new, old *T, err error) {
	old = c.Get()
	if c.CloseFlag() || (old != nil && !replace) {
		return nil, old, nil
	}
	closer, err := provider()
	if err != nil {
		return nil, old, err
	}
	c.closable.Store(closer)
	return closer, old, nil
}

func (c *closableWrapper[T]) Init(provider func() (*T, error), replace bool) (new, old *T, err error) {
	return lock.LockGetTriple(c, func() (new, old *T, err error) {
		return c.InitUnlock(provider, replace)
	})
}

func (c *closableWrapper[T]) SetUnlock(closer *T, replace bool) (new, old *T) {
	old = c.closable.Load()
	if c.CloseFlag() || (old != nil && !replace) {
		return nil, old
	}
	c.closable.Store(closer)
	return closer, old
}

func (c *closableWrapper[T]) Set(closer *T, replace bool) (new, old *T) {
	return lock.LockGetDouble(c, func() (new, old *T) {
		return c.SetUnlock(closer, replace)
	})
}

func (c *closableWrapper[T]) Get() *T {
	return c.closable.Load()
}

func (c *closableWrapper[T]) CloseUnlock() (*T, error) {
	closable := c.Get()
	if !c.close.CompareAndSwap(false, true) {
		return closable, nil
	}
	if closable != nil {
		if closer, ok := any(closable).(io.Closer); ok {
			return closable, closer.Close()
		}
	}
	return closable, nil
}

func (c *closableWrapper[T]) Close() (*T, error) {
	return lock.LockGetDouble(c, c.CloseUnlock)
}

func (c *closableWrapper[T]) CloseFlag() bool {
	return c.close.Load()
}

func (c *closableWrapper[T]) SetClosed() *T {
	return lock.LockGet(c, func() *T {
		c.close.Store(true)
		return c.closable.Swap(nil)
	})
}

func (c *closableWrapper[T]) Reset() *T {
	return lock.LockGet(c, func() *T {
		c.close.Store(false)
		return c.closable.Swap(nil)
	})
}
