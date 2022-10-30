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
	"github.com/spf13/cobra"
	"io"
	"net"
	"os"
	"strings"
	"sync"
)

func init() {
	cobra.MousetrapHelpText = ""
}

type Config struct {
	Help      bool
	Addr      *net.TCPAddr
	Prefix    string
	LogOutput string
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
	command.Flags().StringVarP(&c.Prefix, "prefix", "f", "", "specify dump file prefix")
	command.Flags().StringVarP(&c.LogOutput, "log-output", "o", "stdout", "log output path")
	err := command.Execute()
	logger = assert.Must(log.Config{
		Level: log.NewAtomicLevelAt(log.DebugLevel),
		Encoder: log.NewConsoleEncoder(log.ConsoleEncoderConfig{
			DisableCaller: true,
			//DisableFunction:   true,
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
	listener, err := net.ListenTCP("tcp", cfg.Addr)
	if err != nil {
		Logger().Fatal("listen tcp address error", log.Error(err), log.String("addr", cfg.Addr.String()))
	}

	_, s := lifecycle.New("tcp dump", lifecycle.RunFn(func() error {
		chs := sync.Map{}
		var futures []chan error
		defer func() {
			chs.Range(func(key, _ any) bool {
				ch := key.(lifecycle.OnceLifecycle)
				futures = append(futures, ch.AddClosedFuture(nil))
				ch.Close(nil)
				return true
			})
			for _, future := range futures {
				<-future
			}
		}()
		for {
			conn, err := listener.AcceptTCP()
			if err != nil {
				if opErr, is := err.(*net.OpError); is && opErr.Err.Error() == "use of closed network connection" {
					break
				}
				Logger().ErrorWith("tcp accept connection err", err)
			}
			_, ch := lifecycle.NewOnce("tcp connection", lifecycle.RunFn(func() error {
				file := cfg.Prefix
				if file != "" {
					file += "-"
				}
				file += strings.ReplaceAll(conn.RemoteAddr().String(), ":", "-") + ".tcp"
				fp, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
				if err != nil {
					Logger().ErrorWith("open output file error", err, log.String("file", file))
					return nil
				}
				Logger().Info("open output file success", log.String("file", file))
				defer func() {
					if err := fp.Close(); err != nil {
						Logger().ErrorWith("close file error", err, log.String("file", file))
					} else {
						Logger().Info("close file success", log.String("file", file))
					}
				}()
				fw := bufio.NewWriterSize(fp, unit.MeBiByte)
				var n int64
				defer func() {
					if err := fw.Flush(); err != nil {
						Logger().ErrorWith("dump file error", err)
					}
					Logger().Info("dump file completion", log.Int64("dumped", n))
				}()
				n, err = io.Copy(fw, conn)
				if err != nil {
					Logger().ErrorWith("dump file error", err)
				}
				return nil
			}), lifecycle.CloseFn(func() error {
				if err := conn.Close(); err != nil {
					Logger().ErrorWith("close tcp connection error", err, log.String("addr", conn.RemoteAddr().String()))
				}
				return nil
			}))
			chs.Store(ch, struct{}{})
			future := ch.AddClosedFuture(nil)
			go func() {
				<-future
				chs.Delete(ch)
			}()
		}
		return nil
	}), lifecycle.CloseFn(func() error {
		if err := listener.Close(); err != nil {
			Logger().ErrorWith("close listener error", err)
		}
		return nil
	}))

	exit := svc.New("tcp dump", s, svc.OnStarted(func(s svc.Service, l lifecycle.Lifecycle) {
		Logger().Info("tcp server listen success", log.String("addr", cfg.Addr.String()))
	}), svc.OnClosed(func(s svc.Service, l lifecycle.Lifecycle) {
		Logger().Info("tcp server close success", log.String("addr", cfg.Addr.String()))
	})).Run()
	Logger().Sync()
	os.Exit(exit)
}
