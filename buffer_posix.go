//go:build !linux

package bytebuffers

import (
	"runtime"
)

func (buf *buffer) Close() (err error) {
	runtime.SetFinalizer(buf, nil)
	buf.b = nil
	return
}

func (buf *buffer) grow(n int) (err error) {
	if n < 1 {
		return
	}

	if buf.b != nil {
		if buf.r > 0 {
			n -= buf.r
			// left shift
			copy(buf.b, buf.b[buf.r:buf.w])
			buf.w -= buf.r
			buf.a = buf.w
			buf.r = 0
			if n < 1 {
				return
			}
		}
	}

	if buf.c > maxInt-buf.c-n {
		err = ErrTooLarge
		return
	}

	// has no more place
	adjustedSize := adjustBufferSize(n)
	bLen := buf.Len()
	nb := make([]byte, adjustedSize+bLen)
	copy(nb, buf.b[buf.r:buf.w])
	buf.b = nb
	buf.r = 0
	buf.w = bLen
	buf.a = buf.w
	buf.c += adjustedSize
	return
}
