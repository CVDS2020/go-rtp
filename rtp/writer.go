package rtp

import (
	"encoding/binary"
	"io"
	"math"
)

type Writer struct {
	Writer io.Writer
}

func (w *Writer) Write(layer *Layer) error {
	size := layer.Size()
	if size > math.MaxUint16 {
		return nil
	}
	var header = [2]byte{}
	binary.BigEndian.PutUint16(header[:], uint16(size))
	if _, err := w.Writer.Write(header[:]); err != nil {
		return err
	}
	if _, err := layer.WriteTo(w.Writer); err != nil {
		return err
	}
	return nil
}
