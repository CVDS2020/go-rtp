package utils

import "io"

type Writer struct {
	Buf []byte
	off int
}

func (w *Writer) Reset(buf []byte) {
	w.Buf = buf
	w.off = 0
}

func (w *Writer) Write(p []byte) (n int, err error) {
	n = copy(w.Buf[w.off:], p)
	w.off += n
	return
}

func Write(w io.Writer, p []byte, np *int64) error {
	n, err := w.Write(p)
	*np += int64(n)
	return err
}

func WriteTo(wt io.WriterTo, w io.Writer, np *int64) error {
	n, err := wt.WriteTo(w)
	*np += n
	return err
}
