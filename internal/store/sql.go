package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
)

type RDB struct {
	db *sql.DB
}

func NewRDB(db *sql.DB) *RDB {
	return &RDB{
		db: db,
	}
}

// Bootstrap creates all necessary tables and their structures
func (r *RDB) Bootstrap(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS metrics (
			id varchar PRIMARY KEY,
			type varchar,
			delta bigint,
			value double precision
		);
	`

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("cannot create `urls` table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit transaction: %w", err)
	}
	return nil
}

func (r *RDB) Close() error {
	return r.db.Close()
}

func (r *RDB) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func (r *RDB) Counter(ctx context.Context, id string) (int64, bool) {
	log.Printf("RDB Counter: %s\n", id)

	var delta int64
	query := `SELECT delta FROM metrics WHERE id = $1;`
	err := r.db.QueryRowContext(ctx, query, id).Scan(&delta)
	if err != nil {
		log.Printf("RDB Counter: %s, error: %v\n", id, err)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, true
		}
		return 0, false
	}
	log.Printf("RDB Counter: %s, result: %d\n", id, delta)
	return delta, true
}

func (r *RDB) Gauge(ctx context.Context, id string) (float64, bool) {
	log.Printf("RDB Gauge: %s\n", id)

	var value float64
	query := `SELECT value FROM metrics WHERE id = $1;`
	err := r.db.QueryRowContext(ctx, query, id).Scan(&value)
	if err != nil {
		log.Printf("RDB Gauge: %s, error: %v\n", id, err)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, true
		}
		log.Printf("RDB Gauge: %s\n", id)
		return 0, false
	}
	log.Printf("RDB Gauge: %s, result: %0.3f\n", id, value)
	return value, true
}

func (r *RDB) MapOrderedCounter(ctx context.Context, fun func(k string, v int64)) {
	log.Println("RDB MapOrderedCounter not implemented")
}

func (r *RDB) MapOrderedGauge(ctx context.Context, fun func(k string, v float64)) {
	log.Println("RDB MapOrderedGauge not implemented")
}

func (r *RDB) Timestamp(ctx context.Context, layout string) string {
	log.Println("RDB Timestamp not implemented")
	return ""
}

func (r *RDB) UpdateCount(ctx context.Context) int {
	log.Println("RDB UpdateCount not implemented")
	return 0
}

func (r *RDB) UpdateCounter(ctx context.Context, id string, delta int64) int {
	// DISCLAIMER: Код учебный !!!
	log.Printf("RDB UpdateCounter: %s=%d\n", id, delta)
	prevDelta, _ := r.Counter(ctx, id)

	query := `
		INSERT INTO metrics
		    (id, type, delta)
		VALUES
		    ($1, 'counter', $2)
		ON CONFLICT (id)
		DO UPDATE SET delta = $2
		RETURNING delta
		`

	var prevDelta2 int64
	err := r.db.QueryRowContext(ctx, query, id, prevDelta+delta).Scan(&prevDelta2)
	if err != nil {
		log.Printf("rdb error: %v\n", err)
	}

	log.Printf("RDB UpdateCounter: %s=%d|%d|%d\n", id, prevDelta+delta, prevDelta, delta)
	return int(prevDelta)
}

func (r *RDB) UpdateGauge(ctx context.Context, id string, value float64) int {
	// DISCLAIMER: Код учебный !!!
	log.Printf("RDB UpdateGauge: %s=%0.3f\n", id, value)
	prevValue, _ := r.Gauge(ctx, id)

	query := `
		INSERT INTO metrics
		    (id, type, value)
		VALUES
		    ($1, 'gauge', $2)
		ON CONFLICT (id)
		DO UPDATE SET value = $2
		RETURNING value
		`

	var prevValue2 float64
	err := r.db.QueryRowContext(ctx, query, id, value).Scan(&prevValue2)
	if err != nil {
		log.Printf("rdb error: %v\n", err)
	}
	log.Printf("RDB UpdateGauge: %s=%0.3f|%0.3f\n", id, prevValue, value)
	return int(prevValue)
}
