package agent

// Scope контейнер с репортером замкнутный в своей области видимости.
type Scope interface {
	// Counter возвращает счетчик с соответствующим именем.
	Counter(name string) Counter

	// Gauge возвращает датчик с соответствющим именем.
	Gauge(name string) Gauge

	// Tagged возвращает дочерний scope с указанными тегами.
	Tagged(tags map[string]string) Scope
}

// ReportableScope это интерфейс Scoup с расширенный методом Report.
type ReportableScope interface {
	Scope

	// Report отправляет метрики в репортер.
	Report()
}

// StatsReporter интерфейс для репортера.
type StatsReporter interface {
	Flush()

	// ReportCounter отправляет значения счетчиков.
	ReportCounter(
		name string,
		tags map[string]string,
		value int64,
	)

	// ReportGauge отправляет значения датчиков.
	ReportGauge(
		name string,
		tags map[string]string,
		value float64,
	)
}

// Counter интерфейс для выдачи метрик типа Счетчик.
type Counter interface {
	// Inc увеличить счетчик на дельту.
	Inc(delta int64)
}

// Gauge интерфейс для выдачи метрик типа Датчик.
type Gauge interface {
	// Update обновить текущее значение датчика.
	Update(value float64)
}

// Snapshot создать снимок текущих значений.
type Snapshot interface {
	// Counters returns a snapshot of all counter summations since last report execution.
	Counters() map[string]CounterSnapshot

	// Gauges returns a snapshot of gauge last values since last report execution.
	Gauges() map[string]GaugeSnapshot
}

// CounterSnapshot создать снимок счетчика.
type CounterSnapshot interface {
	// Name returns the name.
	Name() string

	// Tags returns the tags.
	Tags() map[string]string

	// Value returns the value.
	Value() int64
}

// GaugeSnapshot создать снимок датчика.
type GaugeSnapshot interface {
	// Name returns the name.
	Name() string

	// Tags returns the tags.
	Tags() map[string]string

	// Value returns the value.
	Value() float64
}
