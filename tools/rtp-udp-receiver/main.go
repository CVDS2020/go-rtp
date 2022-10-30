package main

import (
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/config"
	"gitee.com/sy_183/common/def"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	svc "gitee.com/sy_183/common/system/service"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/rtp/frame"
	"gitee.com/sy_183/rtp/server"
	"github.com/spf13/cobra"
	"net"
	"os"
)

func init() {
	cobra.MousetrapHelpText = ""
}

type Config struct {
	Help          bool
	Addr          *net.IPAddr
	PortStart     uint16
	PortEnd       uint16
	ReadBuffer    int
	WriteBuffer   int
	BufferSize    uint
	BufferReverse uint
	DebugFrame    bool
	LogOutput     string
}

var args = component.Pointer[Config]{Init: func() *Config {
	c := new(Config)
	config.Handle(c)
	command := cobra.Command{
		Use:   "rtp-udp-receiver",
		Short: "rtp udp receive application",
		Long:  "rtp udp receive application",
		Run: func(cmd *cobra.Command, args []string) {
			c.Help = false
		},
	}
	var addr string
	command.Flags().StringVarP(&addr, "listen-addr", "l", "0.0.0.0", "specify listen ip address")
	command.Flags().Uint16VarP(&c.PortStart, "port-start", "s", 5004, "specify listen port start")
	command.Flags().Uint16VarP(&c.PortEnd, "port-end", "e", 5024, "specify listen port end")
	command.Flags().IntVar(&c.ReadBuffer, "read-buffer", unit.MeBiByte, "specify system socket read buffer")
	command.Flags().IntVar(&c.WriteBuffer, "write-buffer", unit.MeBiByte, "specify system socket write buffer")
	command.Flags().UintVar(&c.BufferSize, "buffer", unit.KiBiByte*256, "specify udp read buffer")
	command.Flags().UintVar(&c.BufferReverse, "buffer-reverse", 2048, "specify udp read buffer reverse")
	command.Flags().BoolVar(&c.DebugFrame, "debug-frame", false, "debug rtp parsed frame count")
	command.Flags().StringVarP(&c.LogOutput, "log-output", "o", "stdout", "log output path")
	err := command.Execute()
	logger = assert.Must(log.Config{
		Level: log.NewAtomicLevelAt(log.DebugLevel),
		Encoder: log.NewConsoleEncoder(log.ConsoleEncoderConfig{
			DisableCaller:     true,
			DisableFunction:   true,
			DisableStacktrace: true,
			EncodeLevel:       log.CapitalColorLevelEncoder,
			EncodeTime:        log.TimeEncoderOfLayout(DefaultTimeLayout),
			EncodeDuration:    log.SecondsDurationEncoder,
		}),
		OutputPaths: []string{c.LogOutput},
	}.Build())
	if err != nil {
		Logger().Fatal("parse command error", log.Error(err))
	}
	ipAddr, err := net.ResolveIPAddr("ip", addr)
	if err != nil {
		Logger().Fatal("parse ip address error", log.Error(err))
	}
	c.Addr = ipAddr
	c.PortStart = def.SetDefault(c.PortStart, 5004)
	if c.PortEnd < c.PortStart+1 {
		c.PortEnd = c.PortStart + 1
	}
	if c.Help {
		os.Exit(0)
	}
	return c
}}

func GetConfig() *Config {
	return args.Get()
}

func main() {
	cfg := GetConfig()
	options := []server.Option{
		server.WithSocketBuffer(cfg.ReadBuffer, cfg.WriteBuffer),
		server.WithBufferPoolSize(cfg.BufferSize),
		server.WithBufferReverse(cfg.BufferReverse),
	}
	var servers []lifecycle.ChildLifecycle
	for port := cfg.PortStart; port < cfg.PortEnd; port++ {
		s := server.NewUDPServer(&net.UDPAddr{
			IP:   cfg.Addr.IP,
			Port: int(port),
			Zone: cfg.Addr.Zone,
		}, options...)
		s.SetLogger(Logger().WithOptions(log.WithName(s.Name())))
		s.Stream(nil, -1, frame.NewFrameRTPHandler(frame.FrameHandlerFunc{
			HandleFrameFn: func(stream server.Stream, frame *frame.Frame) {
				//stream.Logger().Info("receiver frame", log.Uint32("timestamp", frame.Timestamp))
				frame.Release()
			},
		})).SetOnLossPacket(func(stream server.Stream, loss int) {
			s.Logger().Warn("rtp loss packet", log.Int("loss packet", loss))
		})
		servers = append(servers, lifecycle.ChildLifecycle{
			Lifecycle: s,
			OnStarted: func(l lifecycle.Lifecycle) {
				s := l.(server.Server)
				s.Logger().Info("rtp server start success", log.String("addr", s.Addr().String()))
			},
			OnClosed: func(l lifecycle.Lifecycle) {
				s := l.(server.Server)
				s.Logger().Info("rtp server closed", log.String("addr", s.Addr().String()))
			},
		})
	}
	exit := svc.New("rtp udp receiver", lifecycle.NewGroup("rtp udp receiver", servers)).Run()
	Logger().Sync()
	os.Exit(exit)
}
