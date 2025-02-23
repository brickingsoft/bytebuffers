package bytebuffers_test

import (
	"bytes"
	"github.com/brickingsoft/bytebuffers"
	"os"
	"strings"
	"testing"
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
	buf := bytebuffers.NewBuffer()
	_, _ = buf.Write([]byte("0123456789"))
	p := make([]byte, 5)
	n, err := buf.Read(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(n, string(p), string(buf.Peek(5)))
}

func TestBuffer_Write(t *testing.T) {
	buf := bytebuffers.NewBuffer()
	defer func() {
		err := buf.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
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
	t.Log("r1", rn, buf.Len(), buf.Cap(), bytes.Equal(p, firstData)) // r1 4096 512 8192
	p = make([]byte, pagesize)
	rn, rErr = buf.Read(p)
	if rErr != nil {
		t.Fatal(rErr)
	}
	t.Log("r2", rn, buf.Len(), buf.Cap(), bytes.Equal(p, secondData)) // r2 512 0 8192

	wn, wErr = buf.Write(secondData)
	if wErr != nil {
		t.Fatal(wErr)
	}
	t.Log("w3", wn, buf.Len(), buf.Cap(), len(secondData)) // w3 4096 4096 8192 4096
}

// BenchmarkBuffer-20    	20256279	        58.55 ns/op	         0 failed	       0 B/op	       0 allocs/op
func BenchmarkBuffer(b *testing.B) {
	//failed := new(atomic.Int64)
	var err error
	buf := bytebuffers.NewBuffer()
	defer buf.Close()
	pagesize := os.Getpagesize()
	firstData := []byte(strings.Repeat("abcd", pagesize/8))
	secondData := []byte(strings.Repeat("defg", pagesize/4))

	_, _ = buf.Write(firstData)

	p := make([]byte, pagesize)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = buf.Write(secondData)
		if err != nil {
			//failed.Add(1)
		}
		_, err = buf.Read(p)
		if err != nil {
			//failed.Add(1)
		}
	}
	//b.ReportMetric(float64(failed.Load()), "failed")
}
