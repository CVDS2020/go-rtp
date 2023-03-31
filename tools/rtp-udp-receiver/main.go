package main

import (
	"fmt"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/config"
	"gitee.com/sy_183/common/def"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/option"
	svc "gitee.com/sy_183/common/system/service"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/rtp/frame"
	"gitee.com/sy_183/rtp/server"
	"gitee.com/sy_183/rtp/tools/common"
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
		Short: "基于UDP的RTP接收器",
		Long:  "基于UDP的RTP接收器",
		Run: func(cmd *cobra.Command, args []string) {
		},
	}
	var addr string
	command.Flags().StringVarP(&addr, "listen-addr", "l", "0.0.0.0", "指定UDP监听IP地址")
	command.Flags().Uint16VarP(&c.PortStart, "port-start", "s", 5004, "指定UDP监听起始端口")
	command.Flags().Uint16VarP(&c.PortEnd, "port-end", "e", 5024, "指定UDP监听结束端口")
	command.Flags().IntVar(&c.ReadBuffer, "read-buffer", unit.MeBiByte, "指定UDP SOCKET读缓冲区大小，单位为字节")
	command.Flags().IntVar(&c.WriteBuffer, "write-buffer", unit.MeBiByte, "指定UDP SOCKET写缓冲区大小，单位为字节")
	command.Flags().UintVar(&c.BufferSize, "buffer", unit.KiBiByte*256, "指定UDP缓冲区大小，单位为字节")
	command.Flags().UintVar(&c.BufferReverse, "buffer-reverse", 2048, "指定UDP缓冲区保留大小，如果此大小小于接收到的UDP包大小，则可能出现接收到的UDP包数据不完整")
	command.Flags().BoolVar(&c.DebugFrame, "debug-frame", false, "是否打印接收到的帧数据")
	command.Flags().StringVarP(&c.LogOutput, "log-output", "o", "stdout", "日志输出位置")
	command.Flags().BoolVarP(&c.Help, "help", "h", false, "显示帮助信息")
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
		Logger().Fatal("解析命令行参数失败", log.Error(err))
	}
	ipAddr, err := net.ResolveIPAddr("ip", addr)
	if err != nil {
		Logger().Fatal("解析UDP监听IP地址失败", log.Error(err))
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
	options := []option.AnyOption{
		server.WithSocketBuffer(cfg.ReadBuffer, cfg.WriteBuffer),
		server.WithBufferPoolConfig(cfg.BufferSize, "slice"),
		server.WithBufferReverse(cfg.BufferReverse),
	}
	servers := lifecycle.NewGroup()
	for port := cfg.PortStart; port < cfg.PortEnd; port++ {
		s := server.NewUDPServer(&net.UDPAddr{
			IP:   cfg.Addr.IP,
			Port: int(port),
			Zone: cfg.Addr.Zone,
		}, options...)
		name := fmt.Sprintf("基于UDP的RTP服务(%s)", s.Addr().String())
		s.SetLogger(Logger().Named(name))
		s.OnStarting(common.OnStarting("UDP服务", s.Logger())).
			OnStarted(common.OnStarted("UDP服务", s.Logger())).
			OnClose(common.OnClose("UDP服务", s.Logger())).
			OnClosed(common.OnClosed("UDP服务", s.Logger()))
		s.Stream(nil, -1, server.DefaultKeepChooserHandler(frame.NewFrameRTPHandler(frame.FrameHandlerFunc{
			HandleFrameFn: func(stream server.Stream, frame *frame.frame) {
				if cfg.DebugFrame {
					stream.Logger().Debug("接收到数据帧", log.Uint32("时间戳", frame.Timestamp))
				}
				frame.Release()
			},
			OnParseRTPErrorFn: func(stream server.Stream, err error) (keep bool) {
				stream.Logger().ErrorWith("解析RTP包错误", err)
				return true
			},
			OnStreamClosedFn: func(stream server.Stream) {
				stream.Logger().Info("RTP流已关闭")
			},
		}), 5, 5)).SetOnLossPacket(func(stream server.Stream, loss int) {
			stream.Logger().Warn("检测到RTP丢包", log.Int("丢弃数量", loss))
		})
		servers.Add(name, s)
	}
	exit := svc.New("基于UDP的RTP接收器", servers).Run()
	Logger().Sync()
	os.Exit(exit)
}
