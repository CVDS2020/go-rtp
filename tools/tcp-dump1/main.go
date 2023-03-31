package main

import (
	"bufio"
	"encoding/binary"
	"net"
	"os"
)

func main() {
	fp, err := os.OpenFile("test.tcp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	writer := bufio.NewWriter(fp)

	l, err := net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.IP{0, 0, 0, 0},
		Port: 5004,
	})
	if err != nil {
		return
	}
	conn, err := l.AcceptTCP()
	if err != nil {
		return
	}
	buf := make([]byte, 65535)
	lb := make([]byte, 2)
	for i := 0; i < 1000; i++ {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		binary.BigEndian.PutUint16(lb, uint16(n))
		writer.Write(lb)
		writer.Write(buf[:n])
	}
	writer.Flush()
}
