package server

import (
	"fmt"
	"gitee.com/sy_183/common/flag"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"net"
	"sync"
	"time"
)

const (
	serverStateStarting = 1 << iota
	serverStateStopping
	serverStateStopped
)

type serverContext struct {
	server   *RetryableServer
	state    int
	used     uint
	rtpPort  uint16
	rtcpPort uint16
	mu       sync.Mutex
}

func (e *serverContext) lock() {
	e.mu.Lock()
}

func (e *serverContext) unlock() {
	e.mu.Unlock()
}

type Manager struct {
	lifecycle.Lifecycle
	name string
	addr *net.IPAddr

	// server options use to create server
	serverOptions []Option

	// used for port de duplication
	portSet map[uint16]struct{}
	// ports can be allocated
	ports Ports

	// used for create server
	serverProvider ServerProvider
	// server contexts use to store all server
	contexts []*serverContext

	// server max used count
	serverMaxUsed uint

	// server -> serverContext map, find the serverContext corresponding
	// to the server when it is released
	serverContext map[Server]*serverContext
	// alloc server contexts index
	curIndex int

	// alloc server max retry
	allocMaxRetry int
	// server exit or start failed restart interval
	serverRestartInterval time.Duration

	retryableSetters []func(s *RetryableServer)

	mu sync.Mutex

	log.AtomicLogger
}

type ServerProvider func(addr *net.IPAddr, port uint16, options ...Option) Server

func NewManager(addr *net.IPAddr, serverProvider ServerProvider, options ...ManagerOption) *Manager {
	m := &Manager{
		addr:           addr,
		portSet:        make(map[uint16]struct{}),
		serverProvider: serverProvider,
		serverContext:  make(map[Server]*serverContext),
	}

	for _, option := range options {
		option.apply(m)
	}

	if m.addr == nil {
		m.addr = &net.IPAddr{IP: net.IP{0, 0, 0, 0}}
	}
	if m.name == "" {
		m.name = fmt.Sprintf("rtp server manager(%s)", m.addr.String())
	}
	if m.serverMaxUsed == 0 {
		m.serverMaxUsed = 1
	}
	if m.serverRestartInterval == 0 {
		m.serverRestartInterval = time.Second
	}

	if len(m.ports) == 0 {
		m.ports = append(m.ports, Port{
			RTP:  5004,
			RTCP: 5104,
		})
	}

	m.contexts = make([]*serverContext, len(m.ports))
	if m.allocMaxRetry == 0 {
		m.allocMaxRetry = len(m.contexts)
	}

	for i, port := range m.ports {
		m.contexts[i] = &serverContext{
			state:    serverStateStopped,
			rtpPort:  port.RTP,
			rtcpPort: port.RTCP,
		}
	}

	_, m.Lifecycle = lifecycle.New(m.name, lifecycle.Context(m.run))
	return m
}

func (m *Manager) run(interrupter chan struct{}) error {
	<-interrupter
	var runningFutures []chan error
	var startingEndpoints []*serverContext
	var closedFutures []chan error

	for _, ctx := range m.contexts {
		ctx.lock()
		// stop??????server used??????0???????????????
		stop := ctx.used > 0
		if stop {
			// ???used?????????0
			ctx.used = 0
		}
		if flag.TestFlag(ctx.state, serverStateStarting) {
			// server????????????????????????????????????used???0??????????????????????????????
			// ????????????????????????????????????????????????channel
			runningFutures = append(runningFutures, ctx.server.AddRunningFuture(nil))
		} else if !flag.TestFlag(ctx.state, serverStateStopped) {
			// server????????????
			if stop {
				ctx.server.Close(nil)
			}
			// ?????????????????????????????????channel
			closedFutures = append(closedFutures, ctx.server.AddClosedFuture(nil))
		}
		ctx.unlock()
	}

	// ??????????????????staring???server????????????
	for i, future := range runningFutures {
		if err := <-future; err == nil {
			// ?????????????????????????????????????????????
			closedFutures = append(closedFutures, startingEndpoints[i].server.AddClosedFuture(nil))
		}
	}
	// ???????????????server??????
	for _, future := range closedFutures {
		<-future
	}
	return nil
}

func (m *Manager) Name() string {
	return m.name
}

func (m *Manager) setAddr(addr *net.IPAddr) {
	m.addr = addr
}

func (m *Manager) Addr() *net.IPAddr {
	return m.addr
}

func (m *Manager) alloc() *serverContext {
	var retryCount int
	var ctx *serverContext
retry:
	m.mu.Lock()
	for {
		ctx = m.contexts[m.curIndex]
		if m.curIndex++; m.curIndex == len(m.contexts) {
			m.curIndex = 0
		}
		ctx.lock() // first lock
		// server???????????????????????????
		// 1. server??????????????????????????????????????????????????????
		// 2. server????????????????????????????????????????????????????????????????????????????????????????????????
		if !flag.TestFlag(ctx.state, serverStateStopped) && !flag.TestFlag(ctx.state, serverStateStopping) && ctx.used < m.serverMaxUsed {
			// ???????????????????????????????????????????????????server?????????????????????
			ctx.used++
			ctx.unlock()
			m.mu.Unlock()
			return ctx
		}
		if flag.TestFlag(ctx.state, serverStateStopped) && !flag.TestFlag(ctx.state, serverStateStarting) {
			// ?????????????????????????????????server??????????????????
			if ctx.server == nil {
				server := m.serverProvider(m.addr, ctx.rtpPort, m.serverOptions...)
				ctx.server = NewRetryableServer(server, m.serverRestartInterval, m.retryableSetters...)
				_, ctx.server.Lifecycle = lifecycle.New("", lifecycle.Context(ctx.server.run))
				m.serverContext[server] = ctx
			}
			// ??????????????????(??????????????????????????????????????????????????????)server used??????????????????0???
			// ?????????used?????????????????????????????????starting
			ctx.used++
			ctx.state = flag.MaskFlag(ctx.state, serverStateStarting)
			ctx.unlock() // first unlock
			break
		}
		ctx.unlock() // first unlock
		if retryCount++; retryCount > m.allocMaxRetry {
			m.mu.Unlock()
			return nil
		}
	}
	m.mu.Unlock()

	// do start server
	err := ctx.server.Start()

	ctx.lock() // second lock
	// ????????????????????????server??????????????????????????????????????????????????????starting
	ctx.state = flag.UnmaskFlag(ctx.state, serverStateStarting)
	if err == nil {
		// server???????????????????????????stopped????????? ??????stopped??????
		ctx.state = flag.UnmaskFlag(ctx.state, serverStateStopped)
		if ctx.used == 0 {
			// ???????????????server????????????used????????????0(????????????manager)?????????????????????
			// server?????????server???stopping??????
			flag.TestFlag(ctx.state, serverStateStopping)
			ctx.server.Close(nil)
			go func() {
				// ??????server??????????????????????????????????????????server?????????stopping?????????
				// stopped
				<-ctx.server.AddClosedFuture(nil)
				ctx.lock()
				ctx.state = flag.SwapFlagMask(ctx.state, serverStateStopping, serverStateStopped)
				ctx.unlock()
			}()
			ctx.unlock() // second unlock
			return nil
		}
		ctx.unlock()
		return ctx
	}
	// server??????????????????????????????server????????????used????????????0(????????????manager)?????????????????????
	if ctx.used == 0 {
		ctx.unlock()
		return nil
	}
	// ???server used?????????0???????????????server
	ctx.used = 0
	ctx.unlock()
	if retryCount++; retryCount > m.allocMaxRetry {
		return nil
	}
	goto retry
}

func (m *Manager) Alloc() Server {
	endpoint := m.alloc()
	if endpoint == nil {
		return nil
	}
	return endpoint.server.server
}

func (m *Manager) free(ctx *serverContext) {
	ctx.lock()
	// ??????used???????????????endpoint?????????????????????????????????
	if ctx.used == 0 {
		ctx.unlock()
		return
	}
	if flag.TestFlag(ctx.state, serverStateStarting) {
		// ?????????????????????????????????????????????????????????????????????????????????????????????
		ctx.unlock()
		return
	}
	// used???????????????used???0??????????????????server
	if ctx.used--; ctx.used == 0 {
		// ??????????????????stopped?????????????????????starting???????????????stopped??????
		// ???????????????used?????????????????????used????????????????????????????????????????????????
		ctx.server.Close(nil)
		go func() {
			<-ctx.server.AddClosedFuture(nil)
			ctx.lock()
			ctx.state = flag.SwapFlagMask(ctx.state, serverStateStopping, serverStateStopped)
			ctx.unlock()
		}()
	}
	// endpoint???server?????????????????????????????????stop????????????????????????????????????
	// ???????????????????????????
	ctx.unlock()
}

func (m *Manager) Free(s Server) {
	m.mu.Lock()
	ctx := m.serverContext[s]
	if ctx == nil {
		m.mu.Lock()
		return
	}
	m.mu.Unlock()
	m.free(ctx)
}
