package main

import (
	"bufio"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/config"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/rtp/rtp"
	"github.com/spf13/cobra"
	"io"
	"os"
)

func init() {
	cobra.MousetrapHelpText = ""
}

type Config struct {
	Help         bool
	Input        string
	InputBuffer  int
	Output       string
	OutputBuffer int
	LogOutput    string
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
	command.Flags().StringVarP(&c.Input, "input", "i", "", "指定RTP文件路径")
	command.MarkFlagRequired("input")
	command.Flags().StringVarP(&c.Output, "output", "o", "", "指定输出文件路径")
	command.MarkFlagRequired("output")
	command.Flags().IntVar(&c.InputBuffer, "input-buffer", 4*unit.MeBiByte, "指定读取RTP文件的缓冲区大小")
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
		Logger().Fatal("解析命令行参数失败", log.Error(err))
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
		Logger().Fatal("打开文件失败", log.Error(err), log.String("文件名称", cfg.Input))
	}
	defer func() {
		if err := input.Close(); err != nil {
			Logger().ErrorWith("关闭文件失败", err, log.String("文件名称", cfg.Input))
		}
	}()

	output, err := os.OpenFile(cfg.Output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0644)
	if err != nil {
		Logger().Fatal("打开文件失败", log.Error(err), log.String("文件名称", cfg.Output))
	}
	defer func() {
		if err := output.Close(); err != nil {
			Logger().ErrorWith("关闭文件失败", err, log.String("文件名称", cfg.Output))
		}
	}()

	inputBuffer := make([]byte, cfg.InputBuffer)
	outputWriter := bufio.NewWriterSize(output, cfg.OutputBuffer)
	defer func() {
		if err := outputWriter.Flush(); err != nil {
			Logger().ErrorWith("写文件失败", err, log.String("文件名称", cfg.Output))
		}
	}()

	parser := rtp.Parser{Layer: rtp.NewLayer()}
	var offset int64
	for {
		n, err := input.Read(inputBuffer)
		if err != nil {
			if err == io.EOF {
				return
			}
			Logger().ErrorWith("读取文件失败", err, log.String("文件名称", cfg.Input))
			return
		}
		p := inputBuffer[:n]
		for len(p) > 0 {
			ok, remain, err := parser.Parse(p)
			if err != nil {
				Logger().ErrorWith("解析RTP文件失败", err, log.String("文件名称", cfg.Input), log.Int64("文件偏移", offset))
				return
			}
			offset += int64(len(p) - len(remain))
			p = remain
			if ok {
				_, err := parser.Layer.PayloadContent.WriteTo(outputWriter)
				if err != nil {
					Logger().ErrorWith("写文件失败", err, log.String("文件名称", cfg.Output))
					return
				}
				parser.Layer = rtp.NewLayer()
			}
		}
	}
}
