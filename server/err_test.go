package server

import (
	"fmt"
	"gitee.com/sy_183/common/errors"
	"net"
	"testing"
	"time"
)

func TestErr(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IP{0, 0, 0, 0}, Port: 5004})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(time.Second)
		conn.Close()
	}()
	buf := make([]byte, 2048)
	_, err = conn.Read(buf)
	if err != nil {
		fmt.Println(errors.Is(err, net.ErrClosed))
	}
}
