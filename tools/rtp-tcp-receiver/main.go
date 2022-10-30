package main

import (
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/config"
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
	Help            bool
	Addr            *net.TCPAddr
	File            string
	Channel         uint8
	ReadBuffer      int
	WriteBuffer     int
	BufferSize      uint
	BufferReverse   uint
	WriteBufferSize uint
	LogOutput       string
}

var args = component.Pointer[Config]{Init: func() *Config {
	c := new(Config)
	config.Handle(c)
	command := cobra.Command{
		Use:   "rtp-dump",
		Short: "rtp dump to file application",
		Long:  "rtp dump to file application",
		Run: func(cmd *cobra.Command, args []string) {
			c.Help = false
		},
	}
	var addr string
	command.Flags().StringVarP(&addr, "listen-addr", "l", "0.0.0.0:5004", "specify listen tcp address")
	command.Flags().Uint8VarP(&c.Channel, "channel", "c", 0, "specify rtp channel")
	command.Flags().IntVar(&c.ReadBuffer, "read-buffer", unit.MeBiByte, "specify system socket read buffer")
	command.Flags().IntVar(&c.WriteBuffer, "write-buffer", unit.MeBiByte, "specify system socket write buffer")
	command.Flags().UintVar(&c.BufferSize, "buffer", unit.KiBiByte*256, "specify udp read buffer")
	command.Flags().UintVar(&c.BufferReverse, "buffer-reverse", 2048, "specify udp read buffer reverse")
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
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		Logger().Fatal("parse tcp address error", log.Error(err))
	}
	c.Addr = tcpAddr
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
		server.WithOnAccept(func(s *server.TCPServer, conn *net.TCPConn) []server.TCPChannelOption {
			remoteAddr := conn.RemoteAddr()
			s.Logger().Info("accept tcp connection success", log.String("remote addr", remoteAddr.String()))
			s.Stream(remoteAddr, -1, frame.NewFrameRTPHandler(frame.FrameHandlerFunc{
				HandleFrameFn: func(stream server.Stream, frame *frame.Frame) {
					//stream.Logger().Info("receiver frame", log.Uint32("timestamp", frame.Timestamp))
					frame.Release()
				},
				OnStreamClosedFn: func(stream server.Stream) {
					stream.Logger().Info("rtp stream closed",
						log.String("local addr", stream.LocalAddr().String()),
						log.String("remote addr", stream.RemoteAddr().String()),
					)
				},
			})).SetOnLossPacket(func(stream server.Stream, loss int) {
				s.Logger().Warn("rtp loss packet", log.Int("loss packet", loss))
			})
			return nil
		}),
	}
	s := server.NewTCPServer(cfg.Addr, options...)
	s.SetLogger(Logger().WithOptions(log.WithName(s.Name())))
	exit := svc.New("rtp tcp receiver", s, svc.OnStarted(func(s svc.Service, l lifecycle.Lifecycle) {
		us := l.(*server.TCPServer)
		us.Logger().Info("rtp server started", log.String("addr", us.Addr().String()))
	}), svc.OnClosed(func(s svc.Service, l lifecycle.Lifecycle) {
		us := l.(*server.TCPServer)
		us.Logger().Info("rtp server closed", log.String("addr", us.Addr().String()))
	})).Run()
	Logger().Sync()
	os.Exit(exit)
}
