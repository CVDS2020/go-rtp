package server

import (
	"time"
)

// An ManagerOption configures a MultiStreamManager.
type ManagerOption interface {
	apply(m *Manager) any
}

// managerOptionFunc wraps a func, so it satisfies the ManagerOption interface.
type managerOptionFunc func(m *Manager) any

func (f managerOptionFunc) apply(m *Manager) any {
	return f(m)
}

func WithManagerName(name string) ManagerOption {
	return managerOptionFunc(func(m *Manager) any {
		m.name = name
		return nil
	})
}

func WithServerOptions(options ...Option) ManagerOption {
	return managerOptionFunc(func(m *Manager) any {
		m.serverOptions = append(m.serverOptions, options...)
		return nil
	})
}

type Port struct {
	RTP  uint16
	RTCP uint16
}

type Ports []Port

func WithPortRange(start uint16, end uint16, excludes ...uint16) ManagerOption {
	excludeSet := make(map[uint16]struct{})
	for _, exclude := range excludes {
		excludeSet[exclude] = struct{}{}
	}
	return managerOptionFunc(func(m *Manager) any {
		for i := start; i < end; i += 2 {
			var ins [4]bool
			_, ins[0] = excludeSet[i]
			_, ins[1] = excludeSet[i+1]
			_, ins[2] = m.portSet[i]
			_, ins[3] = m.portSet[i+1]
			if ins != [4]bool{} {
				continue
			}
			m.portSet[i], m.portSet[i+1] = struct{}{}, struct{}{}
			m.ports = append(m.ports, Port{RTP: i, RTCP: i + 1})
		}
		return nil
	})
}

func WithPort(rtp uint16, rtcp uint16) ManagerOption {
	return managerOptionFunc(func(m *Manager) any {
		var ins [2]bool
		_, ins[0] = m.portSet[rtp]
		_, ins[1] = m.portSet[rtcp]
		if ins == [2]bool{} {
			m.portSet[rtp], m.portSet[rtcp] = struct{}{}, struct{}{}
			m.ports = append(m.ports, Port{RTP: rtp, RTCP: rtcp})
		}
		return nil
	})
}

func WithServerMaxUsed(maxUsed uint) ManagerOption {
	return managerOptionFunc(func(m *Manager) any {
		m.serverMaxUsed = maxUsed
		return nil
	})
}

func WithAllocMaxRetry(maxRetry int) ManagerOption {
	return managerOptionFunc(func(m *Manager) any {
		m.allocMaxRetry = maxRetry
		return nil
	})
}

func WithRetryInterval(interval time.Duration) ManagerOption {
	return managerOptionFunc(func(m *Manager) any {
		m.serverRestartInterval = interval
		return nil
	})
}

func WithServerStarted(onStarted func(s Server, err error)) ManagerOption {
	return managerOptionFunc(func(m *Manager) any {
		m.retryableSetters = append(m.retryableSetters, RetryableSetOnStarted(onStarted))
		return nil
	})
}

func WithServerClosed(onClosed func(s Server, err error)) ManagerOption {
	return managerOptionFunc(func(m *Manager) any {
		m.retryableSetters = append(m.retryableSetters, RetryableSetOnClosed(onClosed))
		return nil
	})
}

func WithServerClose(onClose func(s Server, err error)) ManagerOption {
	return managerOptionFunc(func(m *Manager) any {
		m.retryableSetters = append(m.retryableSetters, RetryableSetOnClose(onClose))
		return nil
	})
}
