package main

import (
	"bufio"
	"bytes"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/config"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/unit"
	framePkg "gitee.com/sy_183/rtp/frame"
	"gitee.com/sy_183/rtp/rtp"
	"gitee.com/sy_183/rtp/server"
	"github.com/spf13/cobra"
	"io"
	"math"
	"net"
	"os"
	"strings"
	"time"
)

func init() {
	cobra.MousetrapHelpText = ""
}

const DefaultTimeLayout = "2006-01-02 15:04:05.999999999"

var logger *log.Logger

type Config struct {
	Help         bool
	LocalTCPAddr *net.TCPAddr
	LocalUDPAddr *net.UDPAddr
	TCPAddr      *net.TCPAddr
	UDPAddr      *net.UDPAddr
	Transport    string
	StartSeq     int
	Sampling     uint
	FrameRate    uint
	SSRC         int
	Input        string
	InputBuffer  uint
	LogOutput    string
}

var args = component.Pointer[Config]{Init: func() *Config {
	c := new(Config)
	config.Handle(c)
	command := cobra.Command{
		Use:   "rtp-pusher",
		Short: "RTP推流器",
		Long:  "RTP推流器",
		Run: func(cmd *cobra.Command, args []string) {
		},
	}
	var localAddr string
	var addr string
	command.Flags().StringVarP(&localAddr, "local-addr", "l", "", "指定推流本地地址")
	command.Flags().StringVarP(&addr, "server-addr", "s", "127.0.0.1:5004", "指定推流服务端的地址")
	command.Flags().StringVarP(&c.Transport, "transport", "t", "udp", "指定传输协议，tcp/udp")
	command.Flags().IntVar(&c.StartSeq, "start-seq", -1, "指定RTP起始序列号")
	command.Flags().UintVar(&c.Sampling, "sampling", 90000, "指定RTP时钟频率")
	command.Flags().UintVarP(&c.FrameRate, "frame-rate", "r", 25, "指定RTP推流帧率")
	command.Flags().IntVar(&c.SSRC, "ssrc", -1, "指定RTP SSRC")
	command.Flags().StringVarP(&c.Input, "input", "i", "", "指定RTP文件路径")
	command.MarkFlagRequired("input")
	command.Flags().UintVar(&c.InputBuffer, "input-buffer", 4*unit.MeBiByte, "指定读取RTP文件的缓冲区大小")
	command.Flags().StringVar(&c.LogOutput, "log-output", "stdout", "日志输出位置")
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
			logger.Fatal("解析TCP地址失败", log.Error(err))
		}
		c.TCPAddr = tcpAddr
		if localAddr != "" {
			localTCPAddr, err := net.ResolveTCPAddr("tcp", localAddr)
			if err != nil {
				logger.Fatal("解析TCP地址失败", log.Error(err))
			}
			c.LocalTCPAddr = localTCPAddr
		}
	case "UDP":
		c.Transport = "UDP"
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			logger.Fatal("解析UDP地址失败", log.Error(err))
		}
		c.UDPAddr = udpAddr
		if localAddr != "" {
			localUDPAddr, err := net.ResolveUDPAddr("udp", localAddr)
			if err != nil {
				logger.Fatal("解析UDP地址失败", log.Error(err))
			}
			c.LocalUDPAddr = localUDPAddr
		}
	default:
		logger.Fatal("未知的传输协议", log.String("传输协议", c.Transport))
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
	input, err := os.Open(cfg.Input)
	if err != nil {
		logger.ErrorWith("打开文件失败", err, log.String("文件名称", cfg.Input))
		return
	}
	defer func() {
		if err := input.Close(); err != nil {
			logger.ErrorWith("关闭文件失败", err, log.String("文件名称", cfg.Input))
		}
	}()

	var udpConn *net.UDPConn
	var tcpConn *net.TCPConn
	switch cfg.Transport {
	case "UDP":
		udpConn, err = net.DialUDP("udp", cfg.LocalUDPAddr, cfg.UDPAddr)
		if err != nil {
			logger.ErrorWith("打开UDP连接失败", err)
			return
		}
		defer func() {
			if err := udpConn.Close(); err != nil {
				logger.ErrorWith("关闭UDP连接失败", err)
			}
		}()
	case "TCP":
		tcpConn, err = net.DialTCP("tcp", cfg.LocalTCPAddr, cfg.TCPAddr)
		if err != nil {
			logger.ErrorWith("打开TCP连接失败", err)
			return
		}
		defer func() {
			if err := tcpConn.Close(); err != nil {
				logger.ErrorWith("关闭TCP连接失败", err)
			}
		}()
	}

	packetPool := pool.ProvideSyncPool(rtp.ProvideIncomingPacket, pool.WithLimit(math.MinInt64))
	dataPool := pool.NewStaticDataPool(cfg.InputBuffer, pool.ProvideSyncPool[*pool.Data])

	parser := rtp.Parser{}
	var packet *rtp.IncomingPacket
	defer func() {
		if packet != nil {
			packet.Release()
			packet = nil
		}
	}()

	var sendSignal <-chan time.Time
	var interval time.Duration
	if cfg.FrameRate > 0 {
		interval = time.Second / time.Duration(cfg.FrameRate)
		ticker := time.NewTicker(interval)
		sendSignal = ticker.C
		defer ticker.Stop()
	} else {
		signal := make(chan time.Time, 1)
		sendSignal = signal
		interval = time.Second / 25
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				signal <- time.Now()
			}
		}()
	}

	var timestamp time.Duration
	var initSeq = true
	var seq uint16
	writeBuffer := bytes.Buffer{}
	frameHandler := framePkg.NewFrameRTPHandler(framePkg.FrameHandlerFunc{
		HandleFrameFn: func(_ server.Stream, frame *framePkg.IncomingFrame) {
			defer frame.Release()
			<-sendSignal
			frame.SetTimestamp(uint32(timestamp * time.Duration(cfg.Sampling) / time.Second))
			timestamp += interval
			frame.Range(func(i int, packet rtp.Packet) bool {
				if cfg.SSRC >= 0 {
					packet.SetSSRC(uint32(cfg.SSRC))
				}
				if initSeq {
					initSeq = false
					if cfg.StartSeq >= 0 {
						seq = uint16(cfg.StartSeq)
					} else {
						seq = packet.SequenceNumber()
					}
				}
				packet.SetSequenceNumber(seq)
				seq++
				return true
			})
			if tcpConn != nil {
				framePkg.NewFullFrameWriter(frame).WriteTo(&writeBuffer)
				_, err = tcpConn.Write(writeBuffer.Bytes())
				writeBuffer.Reset()
			} else {
				frame.Range(func(i int, packet rtp.Packet) bool {
					defer writeBuffer.Reset()
					packet.WriteTo(&writeBuffer)
					_, err = udpConn.Write(writeBuffer.Bytes())
					if err != nil {
						return false
					}
					return true
				})
			}
		},
	})

	var offset int64
	for {
		data := dataPool.Alloc(cfg.InputBuffer)
		n, err := input.Read(data.Data)
		if err != nil {
			data.Release()
			if err == io.EOF {
				return
			}
			logger.ErrorWith("读取文件失败", err, log.String("文件名称", cfg.Input))
			return
		}
		data.CutTo(uint(n))

		p := data.Data
		for len(p) > 0 {
			if packet == nil {
				// 从RTP包的池中获取，并且添加至解析器
				packet = packetPool.Get().Use()
				parser.Layer = packet.IncomingLayer
			}
			ok, remain, err := parser.Parse(p)
			if err != nil {
				data.Release()
				logger.ErrorWith("解析RTP文件失败", err, log.String("文件名称", cfg.Input), log.Int64("文件偏移", offset))
				return
			}
			offset += int64(len(p) - len(remain))
			p = remain

			if ok {
				// 解析RTP包成功
				packet.AddRelation(data.Use())
				if _, keep := frameHandler.HandlePacket(nil, packet); !keep {
					data.Release()
					return
				}
				packet = nil
			} else {
				packet.AddRelation(data.Use())
			}
		}

		data.Release()
	}
}
