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
		// stop表示server used不为0，需要关闭
		stop := ctx.used > 0
		if stop {
			// 将used标记为0
			ctx.used = 0
		}
		if flag.TestFlag(ctx.state, serverStateStarting) {
			// server正在启动，此时已经标记了used为0，启动后会自动关闭，
			// 这里只需要添加一个追踪启动完成的channel
			runningFutures = append(runningFutures, ctx.server.AddRunningFuture(nil))
		} else if !flag.TestFlag(ctx.state, serverStateStopped) {
			// server正在运行
			if stop {
				ctx.server.Close(nil)
			}
			// 添加一个追踪关闭完成的channel
			closedFutures = append(closedFutures, ctx.server.AddClosedFuture(nil))
		}
		ctx.unlock()
	}

	// 等待那些处于staring的server启动完成
	for i, future := range runningFutures {
		if err := <-future; err == nil {
			// 启动成功了，需要继续追踪其关闭
			closedFutures = append(closedFutures, startingEndpoints[i].server.AddClosedFuture(nil))
		}
	}
	// 等待所有的server关闭
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
		// server可以被分配的条件：
		// 1. server处于关闭状态，并且不是正在启动的状态
		// 2. server处于运行状态，并且不是正在关闭状态，并且分配总数小于最大分配限制
		if !flag.TestFlag(ctx.state, serverStateStopped) && !flag.TestFlag(ctx.state, serverStateStopping) && ctx.used < m.serverMaxUsed {
			// 属于第二种情况，可以立即分配，并将server的分配总数加一
			ctx.used++
			ctx.unlock()
			m.mu.Unlock()
			return ctx
		}
		if flag.TestFlag(ctx.state, serverStateStopped) && !flag.TestFlag(ctx.state, serverStateStarting) {
			// 属于第一种情况，需要将server启动后再分配
			if ctx.server == nil {
				server := m.serverProvider(m.addr, ctx.rtpPort, m.serverOptions...)
				ctx.server = NewRetryableServer(server, m.serverRestartInterval, m.retryableSetters...)
				_, ctx.server.Lifecycle = lifecycle.New("", lifecycle.Context(ctx.server.run))
				m.serverContext[server] = ctx
			}
			// 可以配分配的(已经被关闭并且不是处在正在打开状态的)server used一定被标记为0，
			// 需要将used加一，并且将状态转换为starting
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
	// 不管是否成功启动server，都不是正在打开的状态，需要取消标记starting
	ctx.state = flag.UnmaskFlag(ctx.state, serverStateStarting)
	if err == nil {
		// server启动成功，一定不是stopped状态， 取消stopped标记
		ctx.state = flag.UnmaskFlag(ctx.state, serverStateStopped)
		if ctx.used == 0 {
			// 如果在启动server的过程中used被标记为0(通过关闭manager)，那么立刻关闭
			// server并标记server为stopping状态
			flag.TestFlag(ctx.state, serverStateStopping)
			ctx.server.Close(nil)
			go func() {
				// 追踪server关闭情况，一旦关闭了，需要将server状态从stopping转变为
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
	// server启动失败，如果在启动server的过程中used被标记为0(通过关闭manager)，此时直接退出
	if ctx.used == 0 {
		ctx.unlock()
		return nil
	}
	// 将server used标记为0，重试申请server
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
	// 如果used为零，则此endpoint已经被释放或者正在释放
	if ctx.used == 0 {
		ctx.unlock()
		return
	}
	if flag.TestFlag(ctx.state, serverStateStarting) {
		// 此处不应该被执行，但由于调用者可能会重复释放，此时需要直接返回
		ctx.unlock()
		return
	}
	// used减一，如果used为0，则需要关闭server
	if ctx.used--; ctx.used == 0 {
		// 此处不可能是stopped，以为如果不是starting状态时处于stopped状态
		// 一定标记了used为零，而标记了used为零的在此函数开始时就被过滤掉了
		ctx.server.Close(nil)
		go func() {
			<-ctx.server.AddClosedFuture(nil)
			ctx.lock()
			ctx.state = flag.SwapFlagMask(ctx.state, serverStateStopping, serverStateStopped)
			ctx.unlock()
		}()
	}
	// endpoint的server正在启动，此时被标记了stop标志，启动后会自动关闭，
	// 这里什么都不需要做
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
