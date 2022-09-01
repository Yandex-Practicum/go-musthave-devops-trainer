package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"go-musthave-devops-trainer/internal/agent"
	"go-musthave-devops-trainer/internal/misc"
	"go-musthave-devops-trainer/models"
)

const (
	defaultAddress        = "localhost:8080"
	defaultReportInterval = 10 * time.Second
	defaultPollInterval   = 2 * time.Second
)

type config struct {
	address        string
	reportInterval time.Duration
	pollInterval   time.Duration
	key            string
}

func main() {
	c := config{}

	flag.StringVar(&c.address, "a", defaultAddress, "address <<HOST:PORT>>")
	flag.DurationVar(&c.reportInterval, "r", defaultReportInterval, "report interval")
	flag.DurationVar(&c.pollInterval, "p", defaultPollInterval, "poll interval")
	flag.StringVar(&c.key, "k", "", "key for sha256")

	flag.Parse()

	c = config{
		address:        misc.GetEnvStr("ADDRESS", c.address),
		reportInterval: misc.GetEnvSeconds("REPORT_INTERVAL", c.reportInterval),
		pollInterval:   misc.GetEnvSeconds("POLL_INTERVAL", c.pollInterval),
		key:            misc.GetEnvStr("KEY", c.key),
	}
	if err := c.Run(); err != nil {
		log.Fatalln("client:", err)
	}
	log.Println("client: done")
}

func (c *config) Run() error {
	log.Println("client: starting...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Обрабатывем сигналы от системы.
	termSignal := make(chan os.Signal, 1)
	signal.Notify(termSignal, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	// Регистируем простейший обработчик для выгрузки репортов.
	scopeOpt := agent.ScopeOptions{Reporter: NewReporter(c.address, c.key)}
	scope, closer := agent.NewRootScope(scopeOpt, c.reportInterval)
	defer closer.Close()

	// Запускаем процесс мониторинга с заданным интервалом.
	cancel = runMemMonitor(ctx, scope, c.pollInterval)
	defer cancel()

	// Ожидаем формирование условий, для завершения приложения.
	sig := <-termSignal
	log.Println("client: finished, reason:", sig.String())
	return nil
}

type simpleReporter struct {
	address      string
	client       *http.Client
	counterFlush int
	key          []byte
	metrics      []models.Metrics
}

func NewReporter(address, key string) agent.StatsReporter {
	return &simpleReporter{
		address: "http://" + address + "/updates/",
		client:  &http.Client{},
		key:     []byte(key),
	}
}

// simpleReporter реализация тривиального варианта репортера.
func (r *simpleReporter) ReportCounter(name string, tags map[string]string, delta int64) {
	data := fmt.Sprintf("%s:%s:%d", name, models.Counter, delta)
	// Накапливаем данные для последующей отправки пачкой
	r.metrics = append(r.metrics, models.Metrics{
		ID:    name,
		MType: models.Counter,
		Delta: &delta,
		Hash:  r.hash(data),
	})
}

func (r *simpleReporter) ReportGauge(name string, tags map[string]string, value float64) {
	data := fmt.Sprintf("%s:%s:%f", name, models.Gauge, value)
	// Накапливаем данные для последующей отправки пачкой
	r.metrics = append(r.metrics, models.Metrics{
		ID:    name,
		MType: models.Gauge,
		Value: &value,
		Hash:  r.hash(data),
	})
}

func (r *simpleReporter) Flush() {
	r.counterFlush++
	log.Printf("reporter: flush, count: %d\n", r.counterFlush)
	// Отправляем ранее накопление данные
	metrics := r.metrics
	r.metrics = r.metrics[:0] // в случае проблем, буфер все равно отчищаем.
	jsonBody, err := json.Marshal(metrics)
	if err != nil {
		panic(err)
	}
	resp, err := r.client.Post(r.address, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		log.Println("reporter: ", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("reporter: got response, status: %d, proto: %s, value: %s\n", resp.StatusCode, resp.Proto, jsonBody)
}

func (r *simpleReporter) hash(data string) string {
	if len(r.key) == 0 {
		return ""
	}

	h := hmac.New(sha256.New, r.key)
	h.Write([]byte(data))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// runMemMonitor запускаем горутину по сбору метрик экспартируемых пакетом runtime.
func runMemMonitor(ctx context.Context, scope agent.Scope, pollInterval time.Duration) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	go newMemMonitor(ctx, scope, pollInterval)
	return cancel
}

func newMemMonitor(ctx context.Context, scope agent.Scope, pollInterval time.Duration) {
	rPollCount := scope.Counter("PollCount")
	rRandomValue := scope.Gauge("RandomValue") // Немного энтропии в данных (для примера дробного значения)

	rAlloc := scope.Gauge("Alloc")
	rTotalAlloc := scope.Gauge("TotalAlloc")
	rSys := scope.Gauge("Sys")
	rLookups := scope.Gauge("Lookups")
	rMallocs := scope.Gauge("Mallocs")
	rFrees := scope.Gauge("Frees")
	rHeapAlloc := scope.Gauge("HeapAlloc")
	rHeapSys := scope.Gauge("HeapSys")
	rHeapIdle := scope.Gauge("HeapIdle")
	rHeapInuse := scope.Gauge("HeapInuse")
	rHeapReleased := scope.Gauge("HeapReleased")
	rHeapObjects := scope.Gauge("HeapObjects")
	rStackInuse := scope.Gauge("StackInuse")
	rStackSys := scope.Gauge("StackSys")
	rMSpanInuse := scope.Gauge("MSpanInuse")
	rMSpanSys := scope.Gauge("MSpanSys")
	rMCacheInuse := scope.Gauge("MCacheInuse")
	rMCacheSys := scope.Gauge("MCacheSys")
	rBuckHashSys := scope.Gauge("BuckHashSys")
	rGCSys := scope.Gauge("GCSys")
	rOtherSys := scope.Gauge("OtherSys")
	rNextGC := scope.Gauge("NextGC")
	rLastGC := scope.Gauge("LastGC")
	rPauseTotalNs := scope.Gauge("PauseTotalNs")
	rNumGC := scope.Gauge("NumGC")
	rNumForcedGC := scope.Gauge("NumForcedGC")
	rGCCPUFraction := scope.Gauge("GCCPUFraction")

	rand.Seed(time.Now().UnixNano())
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var rtm runtime.MemStats

	// Обновляем счетчики с заданной периодичностью.
	// Значения (и описание) получаем из пакета runtime.
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			log.Printf("monitor: teminate goroutine, reason: %s\n", ctx.Err())
			return
		}

		log.Printf("monitor: update metrics with interval: %s\n", pollInterval)
		rPollCount.Inc(1)
		rRandomValue.Update(rand.Float64() * 100)

		// Read full mem stats
		runtime.ReadMemStats(&rtm)

		// General statistics.
		// Alloc is bytes of allocated heap objects.
		rAlloc.Update(float64(rtm.Alloc))
		// TotalAlloc is cumulative bytes allocated for heap objects.
		rTotalAlloc.Update(float64(rtm.TotalAlloc))
		// Sys is the total bytes of memory obtained from the OS.
		rSys.Update(float64(rtm.Sys))
		// Lookups is the number of pointer lookups performed by the runtime.
		rLookups.Update(float64(rtm.Lookups))
		// Mallocs is the cumulative count of heap objects allocated.
		// The number of live objects is Mallocs - Frees.
		rMallocs.Update(float64(rtm.Mallocs))
		// Frees is the cumulative count of heap objects freed.
		rFrees.Update(float64(rtm.Frees))

		// Heap memory statistics.
		// HeapAlloc is bytes of allocated heap objects.
		rHeapAlloc.Update(float64(rtm.HeapAlloc))
		// HeapSys is bytes of heap memory obtained from the OS.
		rHeapSys.Update(float64(rtm.HeapSys))
		// HeapIdle is bytes in idle (unused) spans.
		rHeapIdle.Update(float64(rtm.HeapIdle))
		// HeapInuse is bytes in in-use spans.
		rHeapInuse.Update(float64(rtm.HeapInuse))
		// HeapReleased is bytes of physical memory returned to the OS.
		rHeapReleased.Update(float64(rtm.HeapReleased))
		// HeapObjects is the number of allocated heap objects.
		rHeapObjects.Update(float64(rtm.HeapObjects))

		// Stack memory statistics.
		// StackInuse is bytes in stack spans.
		rStackInuse.Update(float64(rtm.StackInuse))
		// StackSys is bytes of stack memory obtained from the OS.
		rStackSys.Update(float64(rtm.StackSys))

		// Off-heap memory statistics.
		// MSpanInuse is bytes of allocated mspan structures.
		rMSpanInuse.Update(float64(rtm.MSpanInuse))
		// MSpanSys is bytes of memory obtained from the OS for mspan
		// structures.
		rMSpanSys.Update(float64(rtm.MSpanSys))
		// MCacheInuse is bytes of allocated mcache structures.
		rMCacheInuse.Update(float64(rtm.MCacheInuse))
		// MCacheSys is bytes of memory obtained from the OS for mcache structures.
		rMCacheSys.Update(float64(rtm.MCacheSys))
		// BuckHashSys is bytes of memory in profiling bucket hash tables.
		rBuckHashSys.Update(float64(rtm.BuckHashSys))
		// GCSys is bytes of memory in garbage collection metadata.
		rGCSys.Update(float64(rtm.GCSys))
		// OtherSys is bytes of memory in miscellaneous off-heap runtime allocations.
		rOtherSys.Update(float64(rtm.OtherSys))

		// Garbage collector statistics.
		// NextGC is the target heap size of the next GC cycle.
		rNextGC.Update(float64(rtm.NextGC))
		// LastGC is the time the last garbage collection finished, as
		// nanoseconds since 1970 (the UNIX epoch).
		rLastGC.Update(float64(rtm.LastGC))
		// PauseTotalNs is the cumulative nanoseconds in GC
		// stop-the-world pauses since the program started.
		rPauseTotalNs.Update(float64(rtm.PauseTotalNs))
		// NumGC is the number of completed GC cycles.
		rNumGC.Update(float64(rtm.NumGC))
		// NumForcedGC is the number of GC cycles that were forced by
		// the application calling the GC function.
		rNumForcedGC.Update(float64(rtm.NumForcedGC))
		// GCCPUFraction is the fraction of this program's available
		// CPU time used by the GC since the program started.
		rGCCPUFraction.Update(float64(rtm.GCCPUFraction))
	}
}
