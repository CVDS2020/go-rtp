package main

import (
	"bufio"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/config"
	"gitee.com/sy_183/common/log"
	poolPkg "gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/rtp/frame"
	"gitee.com/sy_183/rtp/rtp"
	"gitee.com/sy_183/rtp/server"
	"gitee.com/sy_183/rtp/utils"
	"github.com/spf13/cobra"
	"net"
	"os"
	"time"
)

func init() {
	cobra.MousetrapHelpText = ""
}

type Config struct {
	Help      bool
	Addr      *net.TCPAddr
	StartSeq  int
	Sampling  uint
	SSRC      int
	File      string
	LogOutput string
}

var args = component.Pointer[Config]{Init: func() *Config {
	c := new(Config)
	config.Handle(c)
	command := cobra.Command{
		Use:   "rtp-tcp-pusher",
		Short: "rtp tcp push stream application",
		Long:  "rtp tcp push stream application",
		Run: func(cmd *cobra.Command, args []string) {
			c.Help = false
		},
	}
	var addr string
	command.Flags().StringVarP(&addr, "server-addr", "s", "127.0.0.1:5004", "specify server address")
	command.Flags().IntVar(&c.StartSeq, "start-seq", -1, "specify rtp start sequence number")
	command.Flags().UintVar(&c.Sampling, "sampling", 90000, "specify rtp sampling rate")
	command.Flags().IntVar(&c.SSRC, "ssrc", -1, "specify rtp SSRC")
	command.Flags().StringVarP(&c.File, "file", "f", "", "specify dump file path")
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
	if c.File == "" {
		Logger().Fatal("rtp file must be specified")
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
	fp, err := os.Open(cfg.File)
	if err != nil {
		Logger().Fatal("open rtp file error", log.Error(err), log.String("file", cfg.File))
	}
	defer func() {
		if err := fp.Close(); err != nil {
			Logger().ErrorWith("close file error", err)
		}
	}()
	conn, err := net.DialTCP("tcp", nil, cfg.Addr)
	if err != nil {
		Logger().Fatal("connect to server error", log.Error(err), log.String("addr", cfg.Addr.String()))
	}
	defer func() {
		if err := conn.Close(); err != nil {
			Logger().ErrorWith("close tcp connection error", err)
		}
	}()

	pool := poolPkg.NewDynamicDataPoolExp(64*unit.KiBiByte, unit.MeBiByte, "sync")

	var seq uint16
	var initSeq bool

	var initTime bool
	var timeDiff = uint32(0)
	var timeDiffAvg = uint32(0)
	var timeDiffCount = uint32(0)
	var lastTime = uint32(0)
	var timestamp = uint32(0)

	var lastWrite time.Time

	frameHandler := frame.NewFrameRTPHandler(frame.FrameHandlerFunc{
		HandleFrameFn: func(_ server.Stream, frame *frame.frame) {
			if !initTime {
				initTime = true
			} else {
				if frame.Timestamp < lastTime {
					timeDiff = timeDiffAvg
				} else {
					timeDiff = frame.Timestamp - lastTime
				}
				timeDiffAvg = (timeDiffAvg*timeDiffCount + timeDiff) / (timeDiffCount + 1)
				timeDiffCount++
				timestamp += timeDiff
			}
			lastTime = frame.Timestamp

			size := uint(len(frame.RTPLayers) * 4)
			for _, layer := range frame.RTPLayers {
				size += layer.Size()
			}
			var data = pool.Alloc(size)
			w := utils.Writer{Buf: data.Data}
			rw := rtp.Writer{Writer: &w}
			for _, layer := range frame.RTPLayers {
				if cfg.SSRC >= 0 {
					layer.SetSSRC(uint32(cfg.SSRC))
				}
				layer.SetSequenceNumber(seq)
				layer.SetTimestamp(timestamp)
				seq++
				rw.Write(layer)
			}

			if lastWrite != (time.Time{}) {
				now := time.Now()
				expect := lastWrite.Add(time.Second * time.Duration(timeDiff) / time.Duration(cfg.Sampling))
				if now.After(expect) {
					Logger().Warn("delay write frame", log.Duration("delay", now.Sub(expect)))
				} else {
					time.Sleep(expect.Sub(now))
				}
			}
			lastWrite = time.Now()
			_, err := conn.Write(data.Data)
			if err != nil {
				Logger().Fatal("tcp connection write error", log.Error(err))
			}
		},
	})

	fr := bufio.NewReaderSize(fp, unit.MeBiByte)
	rr := rtp.Reader{Reader: fr}
	for {
		layer, err := rr.Read()
		if !initSeq {
			if cfg.StartSeq < 0 {
				seq = layer.SequenceNumber()
			} else {
				seq = uint16(cfg.StartSeq)
			}
			initSeq = true
		}
		if err != nil {
			Logger().Fatal("parse rtp file error", log.Error(err))
		}
		frameHandler.HandlePacket(nil, rtp.NewIncomingPacket(layer))
	}
}
