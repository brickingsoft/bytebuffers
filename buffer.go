package bytebuffers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
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
	// ReadByte
	// 读取一个字节
	ReadByte() (b byte, err error)
	// ReadBytes
	// 以 delim 读
	ReadBytes(delim byte) (line []byte, err error)
	// Index
	// 标号
	Index(delim byte) (i int)
	// Write
	// 写入
	Write(p []byte) (n int, err error)
	// ReadFrom
	// 从一流里读取
	ReadFrom(r io.Reader) (n int64, err error)
	// WriteTo
	// 写入一个流
	WriteTo(w io.Writer) (n int64, err error)
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

func (buf *buffer) ReadByte() (b byte, err error) {
	bLen := buf.Len()
	if bLen == 0 {
		err = io.EOF
		return
	}
	b = buf.b[buf.r]
	buf.r++
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

	if buf.c-buf.w < pLen {
		if err = buf.grow(pLen); err != nil {
			return
		}
	}

	n = copy(buf.b[buf.w:], p)
	buf.w += n
	buf.a = buf.w
	return
}

func (buf *buffer) ReadFrom(r io.Reader) (n int64, err error) {
	if buf.Borrowing() {
		err = ErrWriteBeforeAllocated
		return
	}
	for {
		if buf.w == buf.c {
			if err = buf.grow(1); err != nil {
				return
			}
		}
		rn, rErr := r.Read(buf.b[buf.w:])
		buf.w += rn
		n += int64(rn)
		if rErr != nil {
			if errors.Is(rErr, io.EOF) {
				break
			}
			err = rErr
			return
		}
	}
	return
}

func (buf *buffer) WriteTo(w io.Writer) (n int64, err error) {
	for buf.r < buf.w {
		wn, wErr := w.Write(buf.b[buf.r:buf.w])
		buf.r += wn
		n += int64(wn)
		if wErr != nil {
			err = wErr
			return
		}
	}
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

	if buf.c-buf.w < size {
		if err = buf.grow(size); err != nil {
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

func (buf *buffer) grow(n int) (err error) {
	if n < 1 {
		return
	}

	if buf.b == nil { // init buffer
		adjustedSize := adjustBufferSize(n)
		buf.r = 0
		buf.w = 0
		buf.a = 0
		buf.c += adjustedSize
		buf.b = make([]byte, adjustedSize)
		return
	}

	bLen := buf.Len()
	bCap := buf.Cap()

	if remains := bCap - bLen; n <= remains { // n <= remains then left shift
		copy(buf.b, buf.b[buf.r:buf.w])
		buf.r = 0
		buf.w = bLen
		buf.a = buf.w
		return
	} else { // sub n
		n = n - remains
	}

	if buf.c > maxInt-buf.c-n { // check too large
		err = ErrTooLarge
		return
	}

	// grow
	adjustedSize := adjustBufferSize(n)
	nb := make([]byte, adjustedSize+bCap)
	if bLen > 0 { // has data then copy
		copy(nb, buf.b[buf.r:buf.w])
	}
	buf.r = 0
	buf.w = bLen
	buf.a = buf.w
	buf.c += adjustedSize
	buf.b = nb
	return
}
