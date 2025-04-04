package bytebuffers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"unsafe"
)

type Buffer interface {
	// Len
	// 长度
	Len() (n int)
	// Cap
	// 容量
	Cap() (n int)
	// Peek
	// 查看 n 个字节，但不会读掉。
	Peek(n int) (p []byte)
	// Next
	// 取后 n 个
	Next(n int) (p []byte, err error)
	// Discard
	// 丢弃
	Discard(n int)
	// Read
	// 读取
	Read(p []byte) (n int, err error)
	// ReadBytes
	// 以 delim 读
	ReadBytes(delim byte) (line []byte, err error)
	// Index
	// 标号
	Index(delim byte) (i int)
	// Write
	// 写入
	Write(p []byte) (n int, err error)
	// Borrow
	// 借出
	Borrow(size int) (p []byte, err error)
	// Return
	// 归还借出的实际使用量
	Return(n int)
	// Borrowing
	// 是否有借出
	Borrowing() bool
	// Reset
	// 重置
	Reset() bool
	// Close
	// 关闭
	Close() (err error)
}

const maxInt = int(^uint(0) >> 1)

var (
	pagesize = os.Getpagesize()
)

var (
	ErrTooLarge             = errors.New("bytebuffers.Buffer: too large")
	ErrWriteBeforeAllocated = errors.New("bytebuffers.Buffer: cannot write before Allocated(), cause prev Allocate() was not finished, please call Allocated() after the area was written")
	ErrAllocateZero         = errors.New("bytebuffers.Buffer: cannot allocate zero")
)

func adjustBufferSize(size int) int {
	return int(math.Ceil(float64(size)/float64(pagesize)) * float64(pagesize))
}

func NewBuffer() Buffer {
	return NewBufferWithSize(1)
}

func NewBufferWithSize(size int) Buffer {
	if size <= 0 {
		size = 1
	}
	b := &buffer{
		bufferFields: bufferFields{
			c: 0,
			r: 0,
			w: 0,
			a: 0,
		},
		b: nil,
	}
	err := b.grow(size)
	if err != nil {
		panic(fmt.Sprintf("bytebuffers.Buffer: new buffer with size failed, %v", err))
		return nil
	}
	runtime.SetFinalizer(b, func(buf *buffer) {
		_ = buf.Close()
		runtime.KeepAlive(buf)
	})
	return b
}

type bufferFields struct {
	c int
	r int
	w int
	a int
}

type buffer struct {
	bufferFields
	// We want to use a finalizer, so ensure that the size is large enough to not use the tiny allocator.
	_ [24 - unsafe.Sizeof(bufferFields{})%24]byte
	b []byte
}

func (buf *buffer) Len() int { return buf.w - buf.r }

func (buf *buffer) Cap() int { return buf.c }

func (buf *buffer) Peek(n int) (p []byte) {
	bLen := buf.Len()
	if n < 1 || bLen == 0 {
		return
	}
	if bLen > n {
		p = buf.b[buf.r : buf.r+n]
		return
	}
	p = buf.b[buf.r:buf.w]
	return
}

func (buf *buffer) Next(n int) (p []byte, err error) {
	if n < 1 {
		return
	}
	bLen := buf.Len()
	if bLen == 0 {
		err = io.EOF
		return
	}
	if n > bLen {
		n = bLen
	}
	p = make([]byte, n)
	data := buf.b[buf.r : buf.r+n]
	copy(p, data)
	buf.r += n

	buf.Reset()
	return
}

func (buf *buffer) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return
	}

	bLen := buf.Len()
	if bLen == 0 {
		err = io.EOF
		return
	}

	n = copy(p, buf.b[buf.r:buf.w])
	buf.r += n

	buf.Reset()
	return
}

func (buf *buffer) ReadBytes(delim byte) (line []byte, err error) {
	bLen := buf.Len()
	if bLen == 0 {
		err = io.EOF
		return
	}
	i := bytes.IndexByte(buf.b[buf.r:buf.w], delim)
	if i == -1 {
		line = make([]byte, buf.w)
		n := copy(line, buf.b[buf.r:buf.w])
		buf.r += n
	} else {
		end := buf.r + i + 1
		size := end - buf.r
		line = make([]byte, size)
		n := copy(line, buf.b[buf.r:end])
		buf.r += n
	}

	buf.Reset()
	return
}

func (buf *buffer) Index(delim byte) (i int) {
	bLen := buf.Len()
	if bLen == 0 {
		return
	}
	i = bytes.IndexByte(buf.b[buf.r:buf.w], delim)
	return
}

func (buf *buffer) Discard(n int) {
	if n < 1 {
		return
	}
	bLen := buf.Len()
	if bLen == 0 {
		return
	}
	if n > bLen {
		n = bLen
	}
	buf.r += n
	buf.Reset()
	return
}

func (buf *buffer) Write(p []byte) (n int, err error) {
	if buf.Borrowing() {
		err = ErrWriteBeforeAllocated
		return
	}
	pLen := len(p)
	if pLen == 0 {
		return
	}

	if m := buf.w + pLen - buf.Cap(); m > 0 {
		if err = buf.grow(m); err != nil {
			return
		}
	}

	n = copy(buf.b[buf.w:], p)
	buf.w += n
	buf.a = buf.w
	return
}

func (buf *buffer) Borrowing() bool {
	return buf.a != buf.w
}

func (buf *buffer) Borrow(size int) (p []byte, err error) {
	if buf.Borrowing() {
		err = ErrWriteBeforeAllocated
		return
	}
	if size < 1 {
		err = ErrAllocateZero
		return
	}
	if m := buf.w + size - buf.Cap(); m > 0 {
		if err = buf.grow(m); err != nil {
			return
		}
	}
	p = buf.b[buf.w : buf.w+size]
	buf.a += size
	return
}

func (buf *buffer) Return(n int) {
	if buf.a == buf.w {
		return
	}
	if n == 0 {
		buf.a = buf.w
	} else {
		buf.w += n
		buf.a = buf.w
	}
	return
}

func (buf *buffer) Reset() bool {
	ok := buf.r == buf.w && buf.a == buf.w
	if ok {
		buf.r = 0
		buf.w = 0
		buf.a = 0
	}
	return ok
}

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
	bCap := buf.Cap()
	nb := make([]byte, adjustedSize+bCap)
	copy(nb, buf.b[buf.r:buf.w])
	buf.b = nb
	buf.r = 0
	buf.w = buf.Len()
	buf.a = buf.w
	buf.c += adjustedSize
	return
}
