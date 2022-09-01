package store

import (
	"context"
	"io"
)

type Gauge interface {
	UpdateGauge(ctx context.Context, id string, value float64) int
	Gauge(ctx context.Context, id string) (float64, bool)
}

type Counter interface {
	UpdateCounter(ctx context.Context, id string, delta int64) int
	Counter(ctx context.Context, id string) (int64, bool)
}

type FileStore interface {
	Timestamp(ctx context.Context, layout string) string
	UpdateCount(ctx context.Context) int
	MapOrderedCounter(ctx context.Context, f func(k string, v int64))
	MapOrderedGauge(ctx context.Context, f func(k string, v float64))
}

type Store interface {
	io.Closer
	Gauge
	Counter
	FileStore

	Ping(ctx context.Context) error
}
