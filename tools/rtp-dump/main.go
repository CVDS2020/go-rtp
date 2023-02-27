package main

import (
	"bufio"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/config"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	svc "gitee.com/sy_183/common/system/service"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/rtp/rtp"
	"gitee.com/sy_183/rtp/server"
	"github.com/spf13/cobra"
	"net"
	"os"
	"strings"
)

func init() {
	cobra.MousetrapHelpText = ""
}

type Config struct {
	Help            bool
	Addr            *net.UDPAddr
	File            string
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
	command.Flags().StringVarP(&addr, "listen-addr", "l", "0.0.0.0:5004", "specify listen ip address")
	command.Flags().StringVarP(&c.File, "file", "f", "", "specify dump file path")
	command.Flags().IntVar(&c.ReadBuffer, "read-buffer", unit.MeBiByte, "specify system socket read buffer")
	command.Flags().IntVar(&c.WriteBuffer, "write-buffer", unit.MeBiByte, "specify system socket write buffer")
	command.Flags().UintVar(&c.BufferSize, "buffer", unit.KiBiByte*256, "specify udp read buffer")
	command.Flags().UintVar(&c.BufferReverse, "buffer-reverse", 2048, "specify udp read buffer reverse")
	command.Flags().UintVar(&c.WriteBufferSize, "write-buffer-size", unit.MeBiByte, "specify file write buffer size")
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
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		Logger().Fatal("parse udp address error", log.Error(err))
	}
	c.Addr = udpAddr
	if c.File == "" {
		c.File = strings.ReplaceAll(udpAddr.String(), ":", "-") + ".rtp"
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
	fp, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		Logger().Fatal("open output file error", log.Error(err), log.String("file", cfg.File))
	}
	Logger().Info("open output file success", log.String("file", cfg.File))
	fw := bufio.NewWriterSize(fp, int(cfg.WriteBufferSize))
	rw := rtp.Writer{Writer: fw}
	options := []server.Option{
		server.WithSocketBuffer(cfg.ReadBuffer, cfg.WriteBuffer),
		server.WithBufferPoolConfig(cfg.BufferSize, "sync"),
		server.WithBufferReverse(cfg.BufferReverse),
	}
	s := server.NewUDPServer(cfg.Addr, options...)
	s.SetLogger(Logger().WithOptions(log.WithName(s.Addr().String())))
	s.Stream(nil, -1, server.HandlerFunc{HandlePacketFn: func(stream server.Stream, packet *rtp.Packet) {
		defer packet.Release()
		if err := rw.Write(packet.Layer); err != nil {
			stream.Logger().ErrorWith("write rtp packet error", err)
		}
	}, OnStreamClosedFn: func(stream server.Stream) {
		if err := fw.Flush(); err != nil {
			stream.Logger().ErrorWith("flush rtp packet error", err)
		} else {
			stream.Logger().Info("output file write completion")
		}
	}}).SetOnLossPacket(func(stream server.Stream, loss int) {
		s.Logger().Warn("rtp loss packet", log.Int("loss packet", loss))
	})
	exit := svc.New("rtp dump", s, svc.OnStarted(func(s svc.Service, l lifecycle.Lifecycle) {
		us := l.(*server.UDPServer)
		us.Logger().Info("rtp server started", log.String("addr", us.Addr().String()))
	}), svc.OnClosed(func(s svc.Service, l lifecycle.Lifecycle) {
		us := l.(*server.UDPServer)
		us.Logger().Info("rtp server closed", log.String("addr", us.Addr().String()))
		if err := fp.Close(); err != nil {
			Logger().ErrorWith("output file close error", err)
		} else {
			Logger().Info("output file close success")
		}
	})).Run()
	Logger().Sync()
	os.Exit(exit)
}
