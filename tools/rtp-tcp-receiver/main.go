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
	"time"
)

func init() {
	cobra.MousetrapHelpText = ""
}

type Config struct {
	Help           bool
	Addr           *net.TCPAddr
	Timeout        time.Duration
	ReadBuffer     int
	WriteBuffer    int
	Buffer         uint
	BufferReverse  uint
	BufferPoolType string
	BufferCount    uint
	DebugFrame     bool
	Dump           string
	DumpBuffer     int
	LogOutput      string
}

var args = component.Pointer[Config]{Init: func() *Config {
	c := new(Config)
	config.Handle(c)
	command := cobra.Command{
		Use:   "rtp-tcp-receiver",
		Short: "基于TCP的RTP接收器",
		Long:  "基于TCP的RTP接收器",
		Run: func(cmd *cobra.Command, args []string) {
		},
	}
	var addr string
	command.Flags().StringVarP(&addr, "listen-addr", "l", "0.0.0.0:5004", "指定TCP服务监听地址")
	command.Flags().DurationVarP(&c.Timeout, "timeout", "t", 0, "指定TCP通道超时时间，为0永不超时")
	command.Flags().IntVar(&c.ReadBuffer, "read-buffer", unit.MeBiByte, "指定TCP SOCKET读缓冲区大小，单位为字节")
	command.Flags().IntVar(&c.WriteBuffer, "write-buffer", unit.MeBiByte, "指定TCP SOCKET写缓冲区大小，单位为字节")
	command.Flags().UintVar(&c.Buffer, "buffer", unit.KiBiByte*256, "指定TCP缓冲区大小，单位为字节")
	command.Flags().UintVar(&c.BufferReverse, "buffer-reverse", 2048, "指定TCP缓冲区保留大小")
	command.Flags().StringVar(&c.BufferPoolType, "buffer-pool-type", "slice", "指定TCP缓冲区类型")
	command.Flags().UintVar(&c.BufferCount, "buffer-count", 8, "指定TCP缓冲区个数，只在缓冲区类型为stack时生效")
	command.Flags().BoolVar(&c.DebugFrame, "debug-frame", false, "是否打印接收到的帧数据")
	command.Flags().StringVarP(&c.Dump, "dump", "d", "", "指定接收到的RTP数据保存路径，为空不保存文件")
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
		Logger().Fatal("解析命令行参数失败", log.Error(err))
	}
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		Logger().Fatal("解析TCP监听地址失败", log.Error(err))
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
	var writer *bufio.Writer
	if cfg.Dump != "" {
		fp, err := os.OpenFile(cfg.Dump, os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0644)
		if err != nil {
			Logger().Fatal("打开文件失败", log.Error(err))
		}
		writer = bufio.NewWriterSize(fp, cfg.DumpBuffer)
	}

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
			if cfg.WriteBuffer != 0 {
				if err := conn.SetWriteBuffer(cfg.WriteBuffer); err != nil {
					channel.Logger().ErrorWith("设置 SOCKET 写缓冲区失败", err, log.Int("缓冲区大小", cfg.WriteBuffer))
				}
			}
			//var packets []*rtp.Packet
			//var md5s [][16]byte
			channel.OnStarted(func(lifecycle lifecycle.Lifecycle, err error) {
				s.Stream(conn.RemoteAddr(), -1, server.DefaultKeepChooserHandler(framePkg.NewFrameRTPHandler(framePkg.FrameHandlerFunc{
					HandleFrameFn: func(stream server.Stream, frame *framePkg.IncomingFrame) {
						if cfg.DebugFrame {
							stream.Logger().Debug("接收到数据帧", log.Uint32("时间戳", frame.Timestamp()))
						}
						if writer != nil {
							_, err := framePkg.NewPayloadFrameWriter(frame).WriteTo(writer)
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
				//s.Stream(conn.RemoteAddr(), -1, server.DefaultKeepChooserHandler(server.HandlerFunc{
				//	HandlePacketFn: func(stream server.Stream, packet *rtp.Packet) (dropped, keep bool) {
				//		for i, packet := range packets {
				//			if md5.Sum(packet.PayloadContent.Content()) != md5s[i] {
				//				panic("")
				//			}
				//		}
				//		//if len(packets) > 512 {
				//		//	for _, packet := range packets {
				//		//		if writer != nil {
				//		//			_, err := packet.PayloadContent.WriteTo(writer)
				//		//			if err != nil {
				//		//				stream.Logger().ErrorWith("写文件失败", err)
				//		//			}
				//		//		}
				//		//		packet.Release()
				//		//	}
				//		//	packets = packets[:0]
				//		//	md5s = md5s[:0]
				//		//}
				//		packets = append(packets, packet)
				//		md5s = append(md5s, md5.Sum(packet.PayloadContent.Content()))
				//		//if writer != nil {
				//		//	_, err := packet.PayloadContent.WriteTo(writer)
				//		//	if err != nil {
				//		//		stream.Logger().ErrorWith("写文件失败", err)
				//		//	}
				//		//}
				//		//packet.Release()
				//		return false, true
				//	},
				//	OnParseErrorFn: func(stream server.Stream, err error) (keep bool) {
				//		stream.Logger().ErrorWith("解析RTP流错误", err)
				//		return true
				//	},
				//	OnStreamClosedFn: func(stream server.Stream) {
				//		for _, packet := range packets {
				//			if writer != nil {
				//				_, err := packet.PayloadContent.WriteTo(writer)
				//				if err != nil {
				//					stream.Logger().ErrorWith("写文件失败", err)
				//				}
				//			}
				//			packet.Release()
				//		}
				//		packets = packets[:0]
				//		md5s = md5s[:0]
				//		if writer != nil {
				//			err := writer.Flush()
				//			if err != nil {
				//				stream.Logger().ErrorWith("写文件失败", err)
				//			}
				//		}
				//		stream.Logger().Info("RTP流已关闭")
				//	},
				//}, 5, 5))
			}).OnClose(common.OnClose("TCP通道", channel.Logger())).OnClosed(common.OnClosed("TCP通道", channel.Logger()))
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

	s := server.NewTCPServer(cfg.Addr, options...)
	s.SetLogger(Logger().Named(fmt.Sprintf("基于TCP的RTP服务(%s)", s.Addr().String())))
	s.OnStarting(common.OnStarting("TCP服务", s.Logger())).
		OnStarted(common.OnStarted("TCP服务", s.Logger())).
		OnClose(common.OnClose("TCP服务", s.Logger())).
		OnClosed(common.OnClosed("TCP服务", s.Logger()))
	exit := svc.New("基于TCP的RTP接收器", s).Run()
	Logger().Sync()
	os.Exit(exit)
}
