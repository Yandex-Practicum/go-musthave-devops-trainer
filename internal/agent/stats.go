package agent

import (
	"math"
	"sync/atomic"
)

type counter struct {
	prev int64
	curr int64
}

func newCounter() *counter {
	return &counter{}
}

func (c *counter) Inc(v int64) {
	atomic.AddInt64(&c.curr, v)
}

func (c *counter) report(name string, tags map[string]string, r StatsReporter) {
	delta := c.value()
	if delta == 0 {
		return
	}
	r.ReportCounter(name, tags, delta)
}

func (c *counter) value() int64 {
	curr := atomic.LoadInt64(&c.curr)
	prev := atomic.LoadInt64(&c.prev)
	if prev == curr {
		return 0
	}
	atomic.StoreInt64(&c.prev, curr)
	return curr - prev
}

func (c *counter) snapshot() int64 {
	return atomic.LoadInt64(&c.curr) - atomic.LoadInt64(&c.prev)
}

type gauge struct {
	updated uint64
	curr    uint64
}

func newGauge() *gauge {
	return &gauge{}
}

func (g *gauge) Update(v float64) {
	atomic.StoreUint64(&g.curr, math.Float64bits(v))
	atomic.StoreUint64(&g.updated, 1)
}

func (g *gauge) report(name string, tags map[string]string, r StatsReporter) {
	if atomic.SwapUint64(&g.updated, 0) == 1 {
		r.ReportGauge(name, tags, g.value())
	}
}

func (g *gauge) value() float64 {
	return math.Float64frombits(atomic.LoadUint64(&g.curr))
}

func (g *gauge) snapshot() float64 {
	return math.Float64frombits(atomic.LoadUint64(&g.curr))
}
