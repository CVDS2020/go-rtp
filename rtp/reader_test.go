package rtp

import (
	"bufio"
	"fmt"
	"gitee.com/sy_183/common/unit"
	"io"
	"os"
	"testing"
)

func TestReader(t *testing.T) {
	fp, err := os.Open("C:\\Users\\suy\\Documents\\Language\\Go\\rtp\\tools\\tcp-dump\\192.168.1.108-9720.tcp")
	if err != nil {
		t.Fatal(err)
	}
	defer fp.Close()
	r := Reader{Reader: bufio.NewReaderSize(fp, unit.MeBiByte)}
	for {
		layer, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		fmt.Println(layer)
	}
}
