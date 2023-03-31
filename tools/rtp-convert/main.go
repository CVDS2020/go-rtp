package main

import (
	"bufio"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/config"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/rtp/rtp"
	"github.com/spf13/cobra"
	"io"
	"math"
	"os"
)

func init() {
	cobra.MousetrapHelpText = ""
}

const DefaultTimeLayout = "2006-01-02 15:04:05.999999999"

var logger *log.Logger

type Config struct {
	Help         bool
	Input        string
	InputBuffer  uint
	Output       string
	OutputBuffer int
	LogOutput    string
}

var args = component.Pointer[Config]{Init: func() *Config {
	c := new(Config)
	config.Handle(c)
	command := cobra.Command{
		Use:   "rtp-covert",
		Short: "RTP转换器",
		Long:  "RTP转换器",
		Run: func(cmd *cobra.Command, args []string) {
		},
	}
	command.Flags().StringVarP(&c.Input, "input", "i", "", "指定RTP文件路径")
	command.MarkFlagRequired("input")
	command.Flags().StringVarP(&c.Output, "output", "o", "", "指定输出文件路径")
	command.MarkFlagRequired("output")
	command.Flags().UintVar(&c.InputBuffer, "input-buffer", 4*unit.MeBiByte, "指定读取RTP文件的缓冲区大小")
	command.Flags().IntVar(&c.OutputBuffer, "output-buffer", 4*unit.MeBiByte, "指定输出文件的缓冲区大小")
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
	if c.InputBuffer <= 0 {
		c.InputBuffer = 4 * unit.MeBiByte
	}
	if c.OutputBuffer <= 0 {
		c.OutputBuffer = 4 * unit.MeBiByte
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
		logger.Fatal("打开文件失败", log.Error(err), log.String("文件名称", cfg.Input))
	}
	defer func() {
		if err := input.Close(); err != nil {
			logger.ErrorWith("关闭文件失败", err, log.String("文件名称", cfg.Input))
		}
	}()

	output, err := os.OpenFile(cfg.Output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0644)
	if err != nil {
		logger.Fatal("打开文件失败", log.Error(err), log.String("文件名称", cfg.Output))
	}
	defer func() {
		if err := output.Close(); err != nil {
			logger.ErrorWith("关闭文件失败", err, log.String("文件名称", cfg.Output))
		}
	}()

	outputWriter := bufio.NewWriterSize(output, cfg.OutputBuffer)
	defer func() {
		if err := outputWriter.Flush(); err != nil {
			logger.ErrorWith("写文件失败", err, log.String("文件名称", cfg.Output))
		}
	}()

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
				_, err := packet.Payload().WriteTo(outputWriter)
				if err != nil {
					data.Release()
					logger.ErrorWith("写文件失败", err, log.String("文件名称", cfg.Output))
					return
				}
				packet.Release()
				packet = nil
			} else {
				packet.AddRelation(data.Use())
			}
		}

		data.Release()
	}
}
