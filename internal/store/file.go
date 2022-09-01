package store

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type FDB struct {
	filename string

	sync.Mutex
	counters    map[string]int64
	gauges      map[string]float64
	updateCount int
	tstamp      time.Time
	close       func() error
}

type args struct {
	restoreOnStart bool
	storeInterval  time.Duration
}

type option func(*FDB, *args)

func WithRestoreOnStart(restoreOnStart bool) option {
	return func(db *FDB, a *args) {
		a.restoreOnStart = restoreOnStart
	}
}

func WithInterval(interval time.Duration) option {
	return func(db *FDB, a *args) {
		a.storeInterval = interval
	}
}

func WithFile(filename string) option {
	return func(db *FDB, a *args) {
		db.filename = filename
	}
}

func NewFDB(ctx context.Context, opts ...option) *FDB {
	db := &FDB{
		counters: make(map[string]int64),
		gauges:   make(map[string]float64),
	}

	args := &args{}
	for _, opt := range opts {
		opt(db, args)
	}

	if db.filename == "" {
		return db
	}

	if err := ensureDir(db.filename); err != nil {
		panic(err)
	}
	log.Println("storage: db filename:", db.filename)

	if args.restoreOnStart {
		if err := db.load(); err != nil {
			log.Println("storage: fail on loading:", err)
		}
		if !db.tstamp.IsZero() {
			log.Println("storage: db loaded with:", db.tstamp)
		}
	}

	ctx, cancel := context.WithCancel(ctx)

	// Ограничим минимальный интервал в 1 секунду.
	// Просто что бы показать, что можем.
	// Если примем меньше, то отключаем автосохранение.
	if args.storeInterval >= time.Second {
		go db.run(ctx, args.storeInterval)
	}

	// При завершении, сохраняем данные на диск.
	db.close = func() error {
		log.Println("storage: shutting down...")
		cancel()
		_, _ = db.save()
		log.Println("storage: done")
		return nil
	}
	return db
}

func ensureDir(fileName string) error {
	dirName := filepath.Dir(fileName)
	if err := os.MkdirAll(dirName, os.ModePerm); err != nil && !os.IsExist(err) {
		return err
	}
	f, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, fs.ModePerm)
	if err != nil {
		return err
	}
	f.Close()
	return nil
}

func (f *FDB) Close() error {
	if f.close == nil {
		return nil
	}
	return f.close()
}

func (f *FDB) UpdateCounter(ctx context.Context, id string, delta int64) int {
	f.Lock()
	defer f.Unlock()
	f.tstamp = time.Now()
	f.counters[id] = f.counters[id] + delta
	f.updateCount++
	return f.updateCount
}

func (f *FDB) UpdateGauge(ctx context.Context, id string, value float64) int {
	f.Lock()
	defer f.Unlock()
	f.tstamp = time.Now()
	f.gauges[id] = value
	f.updateCount++
	return f.updateCount
}

func (f *FDB) Counter(ctx context.Context, id string) (int64, bool) {
	f.Lock()
	v, ok := f.counters[id]
	f.Unlock()
	return v, ok
}

func (f *FDB) Gauge(ctx context.Context, id string) (float64, bool) {
	f.Lock()
	v, ok := f.gauges[id]
	f.Unlock()
	return v, ok
}

func (f *FDB) Timestamp(ctx context.Context, layout string) string {
	f.Lock()
	defer f.Unlock()
	return f.tstamp.Format(layout)
}

func (f *FDB) timestamp() time.Time {
	f.Lock()
	defer f.Unlock()
	return f.tstamp
}

func (f *FDB) UpdateCount(ctx context.Context) int {
	f.Lock()
	defer f.Unlock()
	return f.updateCount
}

func (f *FDB) MapOrderedCounter(ctx context.Context, fun func(k string, v int64)) {
	f.Lock()
	defer f.Unlock()
	// По индексу заполнять было бы чуть быстрее, но так выразительнее.
	keys := []string{}
	for k := range f.counters {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	for _, k := range keys {
		fun(k, f.counters[k])
	}
}

func (f *FDB) MapOrderedGauge(ctx context.Context, fun func(k string, v float64)) {
	f.Lock()
	defer f.Unlock()

	keys := []string{}
	for k := range f.gauges {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	for _, k := range keys {
		fun(k, f.gauges[k])
	}
}

func (f *FDB) save() (time.Time, error) {
	jsonBody, timestamp, err := f.marshal()
	if err != nil || len(jsonBody) == 0 {
		return timestamp, err
	}
	err = os.WriteFile(f.filename, jsonBody, os.ModePerm)
	if err != nil {
		return timestamp, err
	}
	log.Println("storage: db saved on:", timestamp)
	return timestamp, nil
}

func (f *FDB) marshal() ([]byte, time.Time, error) {
	f.Lock()
	defer f.Unlock()
	jsonBody, err := json.MarshalIndent(f, "", "  ")
	return jsonBody, f.tstamp, err
}

func (f *FDB) load() error {
	jsonBody, err := os.ReadFile(f.filename)
	if err != nil {
		return err
	}

	f.Lock()
	defer f.Unlock()
	if err := json.Unmarshal(jsonBody, f); err != nil {
		return err
	}
	return nil
}

// Создаем вспомогательную структуру. В первую очередь для того, что бы
// не открывать интерфес DB и не делать поля DB экспортируемыми
// для спокойствия линтера.
// Это делать не обязательно, но для примера почему бы и нет?
// Например можно кастомизировать формат кодирования для Timestamp (бонусное задание?)
type fileDB struct {
	Counters    map[string]int64   `json:"counters,omitempty"`
	Gauges      map[string]float64 `json:"gauges,omitempty"`
	UpdateCount int                `json:"update_count,omitempty"`
	Tstamp      time.Time          `json:"timestamp,omitempty"`
}

func (f *FDB) MarshalJSON() ([]byte, error) {
	return json.Marshal(&fileDB{
		Counters:    f.counters,
		Gauges:      f.gauges,
		UpdateCount: f.updateCount,
		Tstamp:      f.tstamp,
	})
}

func (f *FDB) UnmarshalJSON(data []byte) error {
	fileDB := fileDB{}
	if err := json.Unmarshal(data, &fileDB); err != nil {
		return err
	}

	f.counters = make(map[string]int64)
	if fileDB.Counters != nil {
		f.counters = fileDB.Counters
	}
	f.gauges = make(map[string]float64)
	if fileDB.Gauges != nil {
		f.gauges = fileDB.Gauges
	}
	f.updateCount = fileDB.UpdateCount
	f.tstamp = fileDB.Tstamp
	return nil
}

// Существует и другой подход к сохранению данных.
// Когда мы тактуемся не тикером с проверкой на изменение данных,
// а сторим по таймауту с момента когда данные впервые обновились
// после последнего сохранения на диск.
// У каждого их подходов свои особенности.
// Их можно обсудить сразу, а можно оставить на усмотрение ментора.
func (f *FDB) run(ctx context.Context, interval time.Duration) {
	log.Println("storage: apply safe interval:", interval)

	lastSaved := f.timestamp()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
		ts := f.timestamp()
		if ts.Equal(lastSaved) {
			continue
		}
		var err error
		lastSaved, err = f.save()
		if err != nil {
			log.Println(err)
		}
	}
}

func (f *FDB) Ping(context.Context) error {
	log.Println("file ping not impelemnted")
	return errors.New("not implemented")
}
