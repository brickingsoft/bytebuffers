package bytebuffers

import (
	"bytes"
	"errors"
	"io"
	"math"
	"math/bits"
	"unsafe"
)

type Buffer interface {
	// Len
	// 长度
	Len() (n int)
	// Capacity
	// 容量
	Capacity() (n int)
	// CapacityHint
	// 容量提示
	CapacityHint() (hint int)
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
	// WriteByte
	// 写入字节
	WriteByte(c byte) (err error)
	// WriteString
	// 写入字符串
	WriteString(s string) (n int, err error)
	// Set
	// 重写入可读字节
	Set(p []byte) (err error)
	// SetString
	// 重写入可读字符串
	SetString(s string) (err error)
	// ReadFrom
	// 从一流里读取全部
	ReadFrom(r io.Reader) (n int64, err error)
	// ReadFromWithHint
	// 以 hint 大小从一流里读取全部
	ReadFromWithHint(r io.Reader, hint int) (n int64, err error)
	// ReadFromLimited
	// 从一流里读取 n 个字节
	ReadFromLimited(r io.Reader, n int) (nn int, err error)
	// WriteTo
	// 全部写入一个流
	WriteTo(w io.Writer) (n int64, err error)
	// WriteToLimited
	// 把 n 个字节写入一个流
	WriteToLimited(w io.Writer, n int) (nn int, err error)
	// CloneBytes
	// 复制字节，非读操作。
	CloneBytes() []byte
	// Borrow
	// 借出
	Borrow(size int) (p []byte, err error)
	// Return
	// 归还借出的实际使用量
	Return(used int)
	// Borrowing
	// 是否有借出
	Borrowing() bool
	// Reset
	// 重置，当 Borrowing 时，无法重置。
	Reset() bool
}

const maxInt = int(^uint(0) >> 1)

var (
	ErrTooLarge             = errors.New("bytebuffers.Buffer: too large")
	ErrWriteBeforeAllocated = errors.New("bytebuffers.Buffer: cannot write before Allocated(), cause prev Allocate() was not finished, please call Allocated() after the area was written")
	ErrAllocateZero         = errors.New("bytebuffers.Buffer: cannot allocate zero")
)

func adjustBufferSize(size int, base int) int {
	return int(math.Ceil(float64(size)/float64(base)) * float64(base))
}

func NewBuffer() Buffer {
	return NewBufferWithCapacityHint(minHint)
}

func NewBufferWithCapacityHint(hint int) Buffer {
	if hint <= 0 {
		hint = minHint
	}
	b := &buffer{
		bufferFields: bufferFields{
			h: hint,
			c: 0,
			r: 0,
			w: 0,
			a: 0,
		},
		b: nil,
	}
	return b
}

type bufferFields struct {
	h int
	c int
	r int
	w int
	a int
}

type buffer struct {
	bufferFields
	b []byte
}

func (buf *buffer) Len() int { return buf.w - buf.r }

func (buf *buffer) Capacity() int { return buf.c }

func (buf *buffer) CapacityHint() int {
	return buf.h
}

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

func (buf *buffer) CloneBytes() []byte {
	p := buf.Peek(buf.Len())
	if len(p) == 0 {
		return nil
	}
	c := make([]byte, len(p))
	copy(c, p)
	return c
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
	copy(p, buf.b[buf.r:buf.w])
	buf.r += n

	buf.shrink()
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

	buf.shrink()
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
	buf.shrink()
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
		line = make([]byte, bLen)
		n := copy(line, buf.b[buf.r:buf.w])
		buf.r += n
	} else {
		end := buf.r + i + 1
		size := end - buf.r
		line = make([]byte, size)
		n := copy(line, buf.b[buf.r:end])
		buf.r += n
	}

	buf.shrink()
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
	buf.shrink()
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

func (buf *buffer) WriteString(s string) (n int, err error) {
	if s == "" {
		return
	}
	p := unsafe.Slice(unsafe.StringData(s), len(s))
	return buf.Write(p)
}

func (buf *buffer) WriteByte(c byte) (err error) {
	if buf.Borrowing() {
		err = ErrWriteBeforeAllocated
		return
	}
	if buf.c-buf.w < 1 {
		if err = buf.grow(1); err != nil {
			return
		}
	}
	buf.b[buf.w] = c
	buf.w++
	buf.a = buf.w
	return
}

func (buf *buffer) Set(p []byte) (err error) {
	if buf.Borrowing() {
		err = ErrWriteBeforeAllocated
		return
	}
	pLen := len(p)
	if pLen == 0 {
		if buf.c == 0 {
			return
		}
		buf.w = buf.r
		buf.a = buf.w
		return
	}
	if buf.c-buf.w < pLen {
		if err = buf.grow(pLen); err != nil {
			return
		}
	}
	buf.w = buf.r
	n := copy(buf.b[buf.w:], p)
	buf.w += n
	buf.a = buf.w
	return
}

func (buf *buffer) SetString(s string) (err error) {
	if buf.Borrowing() {
		err = ErrWriteBeforeAllocated
		return
	}
	if s == "" {
		if buf.c == 0 {
			return
		}
		buf.w = buf.r
		buf.a = buf.w
		return
	}
	p := unsafe.Slice(unsafe.StringData(s), len(s))
	return buf.Set(p)
}

func (buf *buffer) ReadFrom(r io.Reader) (n int64, err error) {
	n, err = buf.ReadFromWithHint(r, 1)
	return
}

func (buf *buffer) ReadFromWithHint(r io.Reader, hint int) (n int64, err error) {
	if buf.Borrowing() {
		err = ErrWriteBeforeAllocated
		return
	}
	if hint < 1 {
		hint = 1
	}
	if hint > buf.h {
		shift := bits.Len(uint(hint) - 1)
		hint = 1 << shift
	}
	for {
		if buf.w == buf.c {
			if err = buf.grow(hint); err != nil {
				return
			}
		}
		rn, rErr := r.Read(buf.b[buf.w:])
		buf.w += rn
		buf.a = buf.w
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

func (buf *buffer) ReadFromLimited(r io.Reader, n int) (nn int, err error) {
	if buf.Borrowing() {
		err = ErrWriteBeforeAllocated
		return
	}
	if n < 1 {
		return
	}
	if err = buf.grow(n); err != nil {
		return
	}
	for n > 0 {
		rn, rErr := r.Read(buf.b[buf.w : buf.w+n])
		buf.w += rn
		buf.a = buf.w
		nn += rn
		n -= rn
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

func (buf *buffer) WriteToLimited(w io.Writer, n int) (nn int, err error) {
	if bLen := buf.Len(); bLen < n {
		n = bLen
	}
	for buf.r < buf.w && n > 0 {
		wn, wErr := w.Write(buf.b[buf.r : buf.r+n])
		buf.r += wn
		n -= wn
		nn += wn
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

func (buf *buffer) Return(used int) {
	if buf.a == buf.w {
		return
	}
	if used < 0 {
		panic(errors.New("negative used"))
	}
	if used == 0 {
		buf.a = buf.w
	} else {
		buf.w += used
		buf.a = buf.w
	}
	return
}

func (buf *buffer) Reset() bool {
	ok := !buf.Borrowing()
	if ok {
		buf.r = 0
		buf.w = 0
		buf.a = 0
	}
	return ok
}

func (buf *buffer) shrink() bool {
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
	if buf.Borrowing() {
		err = ErrWriteBeforeAllocated
		return
	}

	if buf.b == nil { // init buffer
		adjustedSize := adjustBufferSize(n, buf.h)
		buf.r = 0
		buf.w = 0
		buf.a = 0
		buf.c += adjustedSize
		buf.b = make([]byte, adjustedSize)
		return
	}

	buf.shrink()
	bLen := buf.Len()
	bCap := buf.Capacity()

	if remains := bCap - bLen; n <= remains { // n <= remains then try to left shift
		if buf.r == 0 { // when read index is 0 then do not left shift
			return
		}
		// has data then left shift
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
	adjustedSize := adjustBufferSize(n, buf.h)
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
