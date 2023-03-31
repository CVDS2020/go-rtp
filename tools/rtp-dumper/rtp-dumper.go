package main

import (
	"bufio"
	"fmt"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/config"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/option"
	"gitee.com/sy_183/common/pool"
	svc "gitee.com/sy_183/common/system/service"
	"gitee.com/sy_183/common/unit"
	framePkg "gitee.com/sy_183/rtp/frame"
	"gitee.com/sy_183/rtp/rtp"
	"gitee.com/sy_183/rtp/server"
	"gitee.com/sy_183/rtp/tools/common"
	"github.com/spf13/cobra"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

func init() {
	cobra.MousetrapHelpText = ""
}

const DefaultTimeLayout = "2006-01-02 15:04:05.999999999"

var logger *log.Logger

type Config struct {
	Help           bool
	TCPAddr        *net.TCPAddr
	UDPAddr        *net.UDPAddr
	Transport      string
	Timeout        time.Duration
	ReadBuffer     int
	Buffer         uint
	BufferReverse  uint
	BufferPoolType string
	BufferCount    uint
	DebugFrame     bool
	Dump           string
	DumpMode       string
	DumpBuffer     int
	LogOutput      string
}

var args = component.Pointer[Config]{Init: func() *Config {
	c := new(Config)
	config.Handle(c)
	command := cobra.Command{
		Use:   "rtp-dumper",
		Short: "RTP接收器",
		Long:  "RTP接收器",
		Run: func(cmd *cobra.Command, args []string) {
		},
	}
	var addr string
	command.Flags().StringVarP(&addr, "listen-addr", "l", "0.0.0.0:5004", "指定TCP服务监听地址")
	command.Flags().StringVarP(&c.Transport, "transport", "t", "udp", "指定传输协议，tcp/udp")
	command.Flags().DurationVar(&c.Timeout, "timeout", 0, "指定TCP通道超时时间，为0永不超时")
	command.Flags().IntVar(&c.ReadBuffer, "read-buffer", unit.MeBiByte, "指定SOCKET读缓冲区大小，单位为字节")
	command.Flags().UintVarP(&c.Buffer, "buffer", "b", unit.KiBiByte*256, "指定缓冲区大小，单位为字节")
	command.Flags().UintVar(&c.BufferReverse, "buffer-reverse", 2048, "指定缓冲区保留大小")
	command.Flags().StringVar(&c.BufferPoolType, "buffer-pool-type", "slice", "指定缓冲区类型，sync/slice/stack")
	command.Flags().UintVar(&c.BufferCount, "buffer-count", 8, "指定缓冲区个数，只在缓冲区类型为stack时生效")
	command.Flags().BoolVar(&c.DebugFrame, "debug-frame", false, "是否打印接收到的帧数据")
	command.Flags().StringVarP(&c.Dump, "dump", "d", "", "指定接收到的RTP数据保存路径，为空不保存文件")
	command.Flags().StringVar(&c.DumpMode, "dump-mode", "raw", "指定保存模式，raw模式只保存RTP负载数据，rtp模式则保存RTP头部和数据")
	command.Flags().IntVar(&c.DumpBuffer, "dump-buffer", 4*unit.MeBiByte, "指定保存RTP数据的写缓冲区大小")
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
		logger.Fatal("解析命令行参数失败", log.Error(err))
	}

	switch strings.ToUpper(c.Transport) {
	case "TCP":
		c.Transport = "TCP"
		tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
		if err != nil {
			logger.Fatal("解析TCP监听地址失败", log.Error(err))
		}
		c.TCPAddr = tcpAddr
	case "UDP":
		c.Transport = "UDP"
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			logger.Fatal("解析UDP监听地址失败", log.Error(err))
		}
		c.UDPAddr = udpAddr
	default:
		logger.Fatal("未知的传输协议", log.String("传输协议", c.Transport))
	}

	switch strings.ToLower(c.DumpMode) {
	case "raw":
		c.DumpMode = "raw"
	case "rtp":
		c.DumpMode = "rtp"
	default:
		logger.Fatal("未知的保存模式", log.String("保存模式", c.DumpMode))
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
	var writer *bufio.Writer
	if cfg.Dump != "" {
		fp, err := os.OpenFile(cfg.Dump, os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0644)
		if err != nil {
			logger.ErrorWith("打开文件失败", err, log.String("文件名称", cfg.Dump))
			return
		}
		defer func() {
			if err := fp.Close(); err != nil {
				logger.ErrorWith("关闭文件失败", err, log.String("文件名称", cfg.Dump))
			}
		}()
		writer = bufio.NewWriterSize(fp, cfg.DumpBuffer)
	}

	createStream := func(s server.Server, remoteAddr net.Addr) {
		s.Stream(remoteAddr, -1, server.DefaultKeepChooserHandler(framePkg.NewFrameRTPHandler(framePkg.FrameHandlerFunc{
			HandleFrameFn: func(stream server.Stream, frame *framePkg.IncomingFrame) {
				if cfg.DebugFrame {
					stream.Logger().Debug("接收到数据帧", log.Uint32("时间戳", frame.Timestamp()))
				}
				if writer != nil {
					var err error
					switch cfg.DumpMode {
					case "rtp":
						_, err = framePkg.NewFullFrameWriter(frame).WriteTo(writer)
					case "raw":
						_, err = framePkg.NewPayloadFrameWriter(frame).WriteTo(writer)
					}
					if err != nil {
						stream.Logger().ErrorWith("写文件失败", err)
					}
				}
				frame.Release()
			},
			OnParseRTPErrorFn: func(stream server.Stream, err error) (keep bool) {
				stream.Logger().ErrorWith("解析RTP流错误", err)
				return true
			},
			OnStreamClosedFn: func(stream server.Stream) {
				if writer != nil {
					err := writer.Flush()
					if err != nil {
						stream.Logger().ErrorWith("写文件失败", err)
					}
				}
				stream.Logger().Info("RTP流已关闭")
			},
		}), 5, 5), server.WithTimeout(cfg.Timeout), server.WithOnLossPacket(func(stream server.Stream, loss int) {
			stream.Logger().Warn("检测到RTP丢包", log.Int("丢弃数量", loss))
		}), server.WithOnStreamTimeout(func(stream server.Stream) {
			stream.Logger().Warn("RTP流超时")
		}))
	}

	streamOnce := sync.Once{}
	options := []option.AnyOption{
		server.WithOnAccept(func(s *server.TCPServer, conn *net.TCPConn) []option.AnyOption {
			remoteAddr := conn.RemoteAddr()
			s.Logger().Info("接受到新的TCP连接",
				log.String("本端地址", conn.LocalAddr().String()),
				log.String("远端地址", remoteAddr.String()),
			)
			return nil
		}),
		server.WithOnChannelCreated(func(s *server.TCPServer, channel *server.TCPChannel) {
			conn := channel.Conn()
			if cfg.ReadBuffer != 0 {
				if err := conn.SetReadBuffer(cfg.ReadBuffer); err != nil {
					channel.Logger().ErrorWith("设置 SOCKET 读缓冲区失败", err, log.Int("缓冲区大小", cfg.ReadBuffer))
				}
			}
			channel.OnStarted(func(lifecycle lifecycle.Lifecycle, err error) {
				streamOnce.Do(func() { createStream(s, conn.RemoteAddr()) })
			}).OnClose(common.OnClose("TCP通道", channel.Logger())).
				OnClosed(common.OnClosed("TCP通道", channel.Logger()))
		}),
	}
	if buffer := cfg.Buffer; buffer != 0 {
		reversed := cfg.BufferReverse
		if reversed == 0 {
			reversed = rtp.DefaultBufferReverse
		}
		if reversed < 1024 {
			reversed = 1024
		}
		if buffer < reversed {
			buffer = reversed
		}
		var poolProvider pool.PoolProvider[*pool.Buffer]
		switch cfg.BufferPoolType {
		case "slice":
			poolProvider = pool.ProvideSlicePool[*pool.Buffer]
		case "sync":
			poolProvider = pool.ProvideSyncPool[*pool.Buffer]
		case "stack":
			bufferCount := cfg.BufferCount
			if bufferCount == 0 {
				bufferCount = 8
			}
			if bufferCount < 2 {
				bufferCount = 2
			}
			poolProvider = pool.StackPoolProvider[*pool.Buffer](bufferCount)
		default:
			poolProvider = pool.ProvideSlicePool[*pool.Buffer]
		}
		options = append(options, server.WithReadBufferPoolProvider(func() pool.BufferPool {
			return pool.NewDefaultBufferPool(buffer, reversed, poolProvider)
		}))
	}

	var s server.Server
	switch cfg.Transport {
	case "UDP":
		us := server.NewUDPServer(cfg.UDPAddr, options...)
		us.OnStarted(func(l lifecycle.Lifecycle, err error) {
			if err == nil && cfg.ReadBuffer != 0 {
				if err := us.Listener().SetReadBuffer(cfg.ReadBuffer); err != nil {
					us.Logger().ErrorWith("设置 SOCKET 读缓冲区失败", err, log.Int("缓冲区大小", cfg.ReadBuffer))
				}
			}
			createStream(us, nil)
		})
		s = us
	case "TCP":
		s = server.NewTCPServer(cfg.TCPAddr, options...)
	}

	s.SetLogger(logger.Named(fmt.Sprintf("基于%s的RTP服务(%s)", cfg.Transport, s.Addr().String())))
	s.OnStarting(common.OnStarting(fmt.Sprintf("%s服务", cfg.Transport), s.Logger())).
		OnStarted(common.OnStarted(fmt.Sprintf("%s服务", cfg.Transport), s.Logger())).
		OnClose(common.OnClose(fmt.Sprintf("%s服务", cfg.Transport), s.Logger())).
		OnClosed(common.OnClosed(fmt.Sprintf("%s服务", cfg.Transport), s.Logger()))
	exit := svc.New("RTP接收器", s).Run()
	logger.Sync()
	os.Exit(exit)
}
