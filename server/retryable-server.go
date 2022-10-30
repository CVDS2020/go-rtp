package server

import (
	"gitee.com/sy_183/common/def"
	"gitee.com/sy_183/common/flag"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/timer"
	"time"
)

const (
	retryableStateInterrupt = 1 << iota
	retryableStateStopped
	retryableStateStarting
)

type RetryableServer struct {
	lifecycle.Lifecycle
	server Server

	onStarted func(s Server, err error)
	onClosed  func(s Server, err error)
	onClose   func(s Server, err error)

	lazyStart     bool
	retryInterval time.Duration
}

func RetryableSetLazyStart(lazyStart bool) func(s *RetryableServer) {
	return func(s *RetryableServer) {
		s.lazyStart = true
	}
}

func RetryableSetOnStarted(onStarted func(s Server, err error)) func(s *RetryableServer) {
	return func(s *RetryableServer) {
		s.onStarted = onStarted
	}
}

func RetryableSetOnClosed(onClosed func(s Server, err error)) func(s *RetryableServer) {
	return func(s *RetryableServer) {
		s.onClosed = onClosed
	}
}

func RetryableSetOnClose(onClose func(s Server, err error)) func(s *RetryableServer) {
	return func(s *RetryableServer) {
		s.onClose = onClose
	}
}

func NewRetryableServer(s Server, retryInterval time.Duration, setters ...func(s *RetryableServer)) *RetryableServer {
	rs := &RetryableServer{
		server:        s,
		retryInterval: retryInterval,
	}
	for _, setter := range setters {
		setter(rs)
	}
	if rs.onStarted == nil {
		rs.onStarted = func(s Server, err error) {
			if logger := s.Logger(); logger != nil {
				if err != nil {
					logger.ErrorWith("rtp server start error", err, log.String("server", s.Name()))
				}
			}
		}
	}
	if rs.onClosed == nil {
		rs.onClosed = func(s Server, err error) {
			if logger := s.Logger(); logger != nil {
				if err != nil {
					logger.ErrorWith("rtp server exit error", err, log.String("server", s.Name()))
				}
			}
		}
	}
	if rs.onClose == nil {
		rs.onClose = func(s Server, err error) {
			if logger := s.Logger(); logger != nil {
				if err != nil {
					logger.ErrorWith("rtp server close error", err, log.String("server", s.Name()))
				}
			}
		}
	}
	rs.retryInterval = def.SetDefault(rs.retryInterval, time.Second)
	_, rs.Lifecycle = lifecycle.New(s.Name(), lifecycle.Context(rs.run))
	return rs
}

func (s *RetryableServer) startServer() error {
	err := s.server.Start()
	if s.onStarted != nil {
		s.onStarted(s.server, err)
	}
	return err
}

func (s *RetryableServer) serverClosed(err error) {
	if s.onClosed != nil {
		s.onClosed(s.server, err)
	}
	return
}

func (s *RetryableServer) closeServer() error {
	err := s.server.Close(nil)
	if s.onClose != nil {
		s.onClose(s.server, err)
	}
	return err
}

func (s *RetryableServer) start() error {
	if !s.lazyStart {
		return s.server.Start()
	}
	return nil
}

func (s *RetryableServer) run(interrupter chan struct{}) error {
	var state int
	var startedChan = make(chan error, 1)
	var closedChan = make(chan error, 1)
	var startRetryTimer = timer.NewTimer(make(chan struct{}, 1))

	if s.lazyStart {
		startRetryTimer.Trigger()
	} else {
		s.server.AddClosedFuture(closedChan)
	}
	for {
		select {
		case <-startRetryTimer.C:
			// must be stopped, must not be starting
			if !flag.TestFlag(state, retryableStateInterrupt) {
				state = flag.MaskFlag(state, retryableStateStarting)
				go func() {
					startedChan <- s.startServer()
				}()
			} else if flag.TestFlag(state, retryableStateInterrupt) {
				return nil
			}
		case err := <-startedChan:
			state = flag.UnmaskFlag(state, retryableStateStarting)
			if err == nil {
				state = flag.UnmaskFlag(state, retryableStateStopped)
				s.server.AddClosedFuture(closedChan)
			}
			if !flag.TestFlag(state, retryableStateInterrupt) {
				if err != nil {
					startRetryTimer.After(s.retryInterval)
				}
			} else {
				if flag.TestFlag(state, retryableStateStopped) {
					return nil
				} else {
					s.closeServer()
				}
			}
		case err := <-closedChan:
			s.serverClosed(err)
			// must not be stopped or starting
			state = flag.MaskFlag(state, retryableStateStopped)
			if flag.TestFlag(state, retryableStateInterrupt) {
				return nil
			}
			startRetryTimer.After(s.retryInterval)
		case <-interrupter:
			state = flag.MaskFlag(state, retryableStateInterrupt)
			if flag.TestFlag(state, retryableStateStopped) {
				return nil
			}
			s.closeServer()
		}
	}
}

func (s *RetryableServer) Server() Server {
	return s.server
}
