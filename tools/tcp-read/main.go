package main

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/rtp/rtp"
	"gitee.com/sy_183/rtp/server"
	"io"
	"net"
	"os"
	"time"
)

type tcpReader struct {
	fp    *os.File
	cache []byte
}

func (r *tcpReader) load() (n int, err error) {
	lb := make([]byte, 2)
	n1, err := io.ReadFull(r.fp, lb)
	if err != nil {
		return 0, err
	}
	l := binary.BigEndian.Uint16(lb)
	r.cache = make([]byte, l)
	n2, err := io.ReadFull(r.fp, r.cache)
	if err != nil {
		return 0, err
	}
	return n1 + n2, nil
}

func (r *tcpReader) Read(p []byte) (n int, err error) {
	if len(r.cache) == 0 {
		n, err = r.load()
		if err != nil {
			return
		}
	}
	n = copy(p, r.cache)
	r.cache = r.cache[n:]
	fmt.Printf("%p\n", &p[0])
	return
}

func main() {
	fp, err := os.Open("test.tcp")
	if err != nil {
		return
	}
	conn := &tcpReader{fp: fp}
	readBufferPool := pool.NewDefaultBufferPool(256*unit.KiBiByte, 2048, pool.ProvideSlicePool[*pool.Buffer])
	packetPool := pool.NewSlicePool(func(p *pool.SlicePool[*rtp.IncomingPacket]) *rtp.IncomingPacket {
		return rtp.NewIncomingPacket(rtp.NewIncomingLayer(), rtp.PacketPool(p))
	})

	var packets []*rtp.IncomingPacket
	var md5s [][16]byte
	stream := &server.TCPStream{}
	stream.SetSelf(stream)
	stream.SetHandler(server.DefaultKeepChooserHandler(server.HandlerFunc{
		HandlePacketFn: func(stream server.Stream, packet *rtp.IncomingPacket) (dropped, keep bool) {
			for i, packet := range packets {
				if md5.Sum(packet.PayloadContent.Content()) != md5s[i] {
					panic("")
				}
			}
			packets = append(packets, packet)
			md5s = append(md5s, md5.Sum(packet.PayloadContent.Content()))
			return false, true
		},
	}, 5, 5))
	stream.SetSSRC(-1)

	var packet *rtp.IncomingPacket
	var parser rtp.Parser
	for {
		buf := readBufferPool.Get()
		if buf == nil {
			panic("")
		}
		n, err := conn.Read(buf)
		if err != nil {
			panic(err)
		}
		data := readBufferPool.Alloc(uint(n))

		for p := data.Data; len(p) > 0; {
			var ok bool
			if packet == nil {
				// 从RTP包的池中获取，并且添加至解析器
				packet = packetPool.Get().Use()
				parser.Layer = packet.IncomingLayer
			}
			if ok, p, err = parser.Parse(p); ok {
				// 解析RTP包成功
				packet.Chunks = append(packet.Chunks, data.Use())
				packet.Addr = &net.TCPAddr{IP: net.IP{192, 168, 1, 129}, Port: 5004}
				packet.Time = time.Now()
				// 将RTP包交给流处理，流处理器必须在使用完RTP包后将其释放
				stream.HandlePacket(stream, packet)
				packet = nil
			} else if err != nil {
				// 解析RTP包出错，释放RTP包中的数据，如果可以继续解析则使用此RTP包作为接下来解析器解析
				// 的载体
				panic(err)
			} else {
				// 解析RTP包未完成，需要将此块数据的引用添加到此RTP包中
				packet.Chunks = append(packet.Chunks, data.Use())
			}
		}

		data.Release()
	}

}
