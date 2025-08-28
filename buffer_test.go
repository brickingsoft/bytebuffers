package bytebuffers_test

import (
	"bytes"
	"crypto/rand"
	"os"
	"strings"
	"testing"

	"github.com/brickingsoft/bytebuffers"
)

func TestBuffer(t *testing.T) {
	buf := bytebuffers.NewBuffer()
	t.Log(buf.Cap(), buf.Len())
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
	t.Log(buf.Len(), buf.Cap())
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
	t.Log(buf.Cap(), buf.Len()) //  4096 0
	pagesize := os.Getpagesize()
	firstData := []byte(strings.Repeat("a", pagesize/8))
	secondData := []byte(strings.Repeat("1", pagesize))
	t.Log("f", len(firstData), "s", len(secondData)) // f 512 s 4096
	wn, wErr := buf.Write(firstData)
	if wErr != nil {
		t.Fatal(wErr)
	}
	t.Log("w1", wn, buf.Len(), buf.Cap(), len(firstData)) // w1 512 512 4096 512
	wn, wErr = buf.Write(secondData)
	if wErr != nil {
		t.Fatal(wErr)
	}
	t.Log("w2", wn, buf.Len(), buf.Cap(), len(secondData)) // w2 4096 4608 8192 4096
	p := make([]byte, pagesize/8)
	rn, rErr := buf.Read(p)
	if rErr != nil {
		t.Fatal(rErr)
	}
	t.Log("r1", rn, buf.Len(), buf.Cap(), bytes.Equal(p, firstData)) // r1 512 4096 8192 true
	p = make([]byte, pagesize)
	rn, rErr = buf.Read(p)
	if rErr != nil {
		t.Fatal(rErr)
	}
	t.Log("r2", rn, buf.Len(), buf.Cap(), bytes.Equal(p, secondData)) // r2 4096 0 8192 true

	wn, wErr = buf.Write(secondData)
	if wErr != nil {
		t.Fatal(wErr)
	}
	t.Log("w3", wn, buf.Len(), buf.Cap(), len(secondData)) // w3 4096 4096 8192 4096
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
