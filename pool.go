package bytebuffers

import (
	"sort"
	"sync"
	"sync/atomic"
)

const (
	minBitSize = 6
	steps      = 20

	minSize = 1 << minBitSize
	maxSize = 1 << (minBitSize + steps - 1)

	calibrateCallsThreshold = 42000
	maxPercentile           = 0.95
)

var defaultBufferPool BufferPool

// Acquire
// 请求一个 Buffer。
func Acquire() Buffer { return defaultBufferPool.Acquire() }

// Release
// 回收 Buffer，只有当 Buffer.Reset 成功才回收，否则关闭并丢弃。
// 即无可读或无未完成分配的情况下可回收。
func Release(b Buffer) { defaultBufferPool.Release(b) }

type BufferPool struct {
	calls       [steps]uint64
	calibrating uint64

	defaultSize uint64
	maxSize     uint64

	pool sync.Pool
}

func (p *BufferPool) Acquire() Buffer {
	v := p.pool.Get()
	if v != nil {
		return v.(Buffer)
	}
	return NewBufferWithSize(int(atomic.LoadUint64(&p.defaultSize)))
}

func (p *BufferPool) Release(b Buffer) {
	if ok := b.Reset(); ok {
		if b.Cap() > maxSize {
			_ = b.Close()
			return
		}
		idx := p.index(b.Len())

		if atomic.AddUint64(&p.calls[idx], 1) > calibrateCallsThreshold {
			p.calibrate()
		}
		size := int(atomic.LoadUint64(&p.maxSize))
		if size == 0 || b.Cap() <= size {
			p.pool.Put(b)
		} else {
			_ = b.Close()
		}
		return
	}
}

func (p *BufferPool) index(n int) int {
	n--
	n >>= minBitSize
	idx := 0
	for n > 0 {
		n >>= 1
		idx++
	}
	if idx >= steps {
		idx = steps - 1
	}
	return idx
}

func (p *BufferPool) calibrate() {
	if !atomic.CompareAndSwapUint64(&p.calibrating, 0, 1) {
		return
	}

	a := make(callSizes, 0, steps)
	var callsSum uint64
	for i := uint64(0); i < steps; i++ {
		calls := atomic.SwapUint64(&p.calls[i], 0)
		callsSum += calls
		a = append(a, callSize{
			calls: calls,
			size:  minSize << i,
		})
	}
	sort.Sort(a)

	defaultSize := a[0].size
	maxSizeOfCall := defaultSize

	maxSum := uint64(float64(callsSum) * maxPercentile)
	callsSum = 0
	for i := 0; i < steps; i++ {
		if callsSum > maxSum {
			break
		}
		callsSum += a[i].calls
		size := a[i].size
		if size > maxSizeOfCall {
			maxSizeOfCall = size
		}
	}

	atomic.StoreUint64(&p.defaultSize, defaultSize)
	atomic.StoreUint64(&p.maxSize, maxSizeOfCall)

	atomic.StoreUint64(&p.calibrating, 0)
}

type callSize struct {
	calls uint64
	size  uint64
}

type callSizes []callSize

func (ci callSizes) Len() int {
	return len(ci)
}

func (ci callSizes) Less(i, j int) bool {
	return ci[i].calls > ci[j].calls
}

func (ci callSizes) Swap(i, j int) {
	ci[i], ci[j] = ci[j], ci[i]
}
