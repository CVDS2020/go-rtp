package server

import (
	"gitee.com/sy_183/common/option"
	"time"
)

// An ManagerOption configures a MultiStreamManager.
type ManagerOption option.CustomOption[*Manager]

func WithServerOptions(options ...option.AnyOption) ManagerOption {
	return option.Custom[*Manager](func(m *Manager) {
		m.serverOptions = append(m.serverOptions, options...)
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
	return option.Custom[*Manager](func(m *Manager) {
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
	})
}

func WithPort(rtp uint16, rtcp uint16) ManagerOption {
	return option.Custom[*Manager](func(m *Manager) {
		var ins [2]bool
		_, ins[0] = m.portSet[rtp]
		_, ins[1] = m.portSet[rtcp]
		if ins == [2]bool{} {
			m.portSet[rtp], m.portSet[rtcp] = struct{}{}, struct{}{}
			m.ports = append(m.ports, Port{RTP: rtp, RTCP: rtcp})
		}
	})
}

func WithServerMaxUsed(maxUsed uint) ManagerOption {
	return option.Custom[*Manager](func(m *Manager) {
		m.serverMaxUsed = maxUsed
	})
}

func WithAllocMaxRetry(maxRetry int) ManagerOption {
	return option.Custom[*Manager](func(m *Manager) {
		m.allocMaxRetry = maxRetry
	})
}

func WithServerRestartInterval(interval time.Duration) ManagerOption {
	return option.Custom[*Manager](func(m *Manager) {
		m.serverRestartInterval = interval
	})
}
