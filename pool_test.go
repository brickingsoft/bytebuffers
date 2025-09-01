package bytebuffers_test

import (
	"testing"

	"github.com/brickingsoft/bytebuffers"
)

func TestPool(t *testing.T) {
	pool := bytebuffers.Pool(512)
	b := pool.Acquire()
	defer pool.Release(b)
}
