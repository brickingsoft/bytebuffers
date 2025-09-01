package bytebuffers_test

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/brickingsoft/bytebuffers"
)

func TestBuffer(t *testing.T) {
	buf := bytebuffers.NewBuffer()
	t.Log(buf.Capacity(), buf.Len())
	t.Log(buf.Write([]byte("0123456789")))
	t.Log(buf.Len())
	p5 := buf.Peek(5)
	t.Log(string(p5))
	buf.Discard(5)

	nexted, nextErr := buf.Next(5)
	if nextErr != nil {
		t.Fatal(nextErr)
	}
	t.Log(string(nexted))
	t.Log(buf.Len(), buf.Capacity())
}

func TestBuffer_Borrow(t *testing.T) {
	buf := bytebuffers.NewBuffer()
	_, _ = buf.Write([]byte("0123456789"))
	p, allocateErr := buf.Borrow(5)
	if allocateErr != nil {
		t.Fatal(allocateErr)
	}
	copy(p, "abc")
	buf.Return(3)
	_, _ = buf.Write([]byte("012"))
	t.Log(string(buf.Peek(100)))
}

func TestBuffer_Read(t *testing.T) {
	buf := bytebuffers.Acquire()
	defer bytebuffers.Release(buf)
	_, _ = buf.Write([]byte("0123456789"))
	p := make([]byte, 5)
	n, err := buf.Read(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(n, string(p), string(buf.Peek(5)))
	buf.Discard(5)
}

func TestBuffer_Write(t *testing.T) {
	buf := bytebuffers.NewBuffer()
	t.Log(buf.Capacity(), buf.Len()) //  64 0
	pagesize := buf.Capacity()
	firstData := []byte(strings.Repeat("a", pagesize/8))
	secondData := []byte(strings.Repeat("1", pagesize))
	t.Log("first", len(firstData), "second", len(secondData)) // first 8 second 64
	wn, wErr := buf.Write(firstData)
	if wErr != nil {
		t.Fatal(wErr)
	}
	t.Log("w1", wn, buf.Len(), buf.Capacity(), len(firstData)) // w1 8 8 64 8
	wn, wErr = buf.Write(secondData)
	if wErr != nil {
		t.Fatal(wErr)
	}
	t.Log("w2", wn, buf.Len(), buf.Capacity(), len(secondData)) // w2 64 72 128 64
	p := make([]byte, pagesize/8)
	rn, rErr := buf.Read(p)
	if rErr != nil {
		t.Fatal(rErr)
	}
	t.Log("r1", rn, buf.Len(), buf.Capacity(), bytes.Equal(p, firstData)) // r1 8 64 128 true
	p = make([]byte, pagesize)
	rn, rErr = buf.Read(p)
	if rErr != nil {
		t.Fatal(rErr)
	}
	t.Log("r2", rn, buf.Len(), buf.Capacity(), bytes.Equal(p, secondData)) // r2 64 0 128 true

	wn, wErr = buf.Write(bytes.Repeat(secondData, 3))
	if wErr != nil {
		t.Fatal(wErr)
	}
	t.Log("w3", wn, buf.Len(), buf.Capacity(), len(secondData)) // w3 192 192 192 64
}

func TestBuffer_ReadFrom(t *testing.T) {
	buf := bytebuffers.Acquire()
	defer bytebuffers.Release(buf)
	n := buf.Capacity()
	src := bytes.NewBuffer([]byte("0123456789"))
	src.Write(bytes.Repeat([]byte("a"), n))

	rn, rErr := buf.ReadFrom(src)
	if rErr != nil {
		t.Fatal(rErr)
	}
	t.Log(rn, buf.Len(), rn == int64(10+n))

}

func TestBuffer_WriteTo(t *testing.T) {
	buf := bytebuffers.Acquire()
	defer bytebuffers.Release(buf)
	n := buf.Capacity()
	buf.Write([]byte("0123456789"))
	buf.Write(bytes.Repeat([]byte("a"), n))

	dst := bytes.NewBuffer(nil)

	wn, wErr := buf.WriteTo(dst)
	if wErr != nil {
		t.Fatal(wErr)
	}
	t.Log(wn, buf.Len(), dst.Len() == int(wn))
}

func TestBuffer_Set(t *testing.T) {
	b := bytebuffers.NewBuffer()
	defer bytebuffers.Release(b)
	b.WriteString("0123456789")
	b.SetString("abdce")

	p := b.CloneBytes()
	t.Log(string(p), string(p) == "abdce")
}

// BenchmarkBuffer
// BenchmarkBuffer-20    	13220983	        86.01 ns/op	       0 B/op	       0 allocs/op
func BenchmarkBuffer(b *testing.B) {
	b.ReportAllocs()

	buf := bytebuffers.Acquire()
	defer bytebuffers.Release(buf)

	rb := make([]byte, 4096)
	wb := make([]byte, 4096)

	buf.Write(wb)
	buf.Read(wb)

	buf.Reset()

	rb = make([]byte, 4096*2)
	wb = make([]byte, 4096*2)

	rand.Read(wb)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Write(wb)
		buf.Read(rb)
		buf.Reset()
	}
}

// BenchmarkStandByteBuffer
// BenchmarkStandByteBuffer-20    	12543076	        98.56 ns/op	       0 B/op	       0 allocs/op
func BenchmarkStandByteBuffer(b *testing.B) {
	b.ReportAllocs()

	buf := bytes.NewBuffer(nil)

	rb := make([]byte, 4096)
	wb := make([]byte, 4096)

	buf.Write(wb)
	buf.Read(wb)

	buf.Reset()

	rb = make([]byte, 4096*2)
	wb = make([]byte, 4096*2)

	rand.Read(wb)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Write(wb)
		buf.Read(rb)
		buf.Reset()
	}
}
