package agent

import (
	"io"
	"sync"
	"time"
)

// DefaultSeparator разделитель по умолчанию.
const DefaultSeparator = "."

type scope struct {
	prefix    string
	reporter  StatsReporter
	separator string
	tags      map[string]string

	registry *scopeRegistry
	status   scopeStatus

	cm sync.Mutex
	gm sync.Mutex

	counters map[string]*counter
	gauges   map[string]*gauge
}

type scopeStatus struct {
	sync.Mutex
	closed bool
	quit   chan struct{}
}

type scopeRegistry struct {
	sync.Mutex
	subscopes map[string]*scope
}

// ScopeOptions набор опций для создания области видимости.
type ScopeOptions struct {
	Tags      map[string]string
	Prefix    string
	Reporter  StatsReporter
	Separator string
}

// NewRootScope создать область видимости для сбора метрик.
func NewRootScope(opts ScopeOptions, interval time.Duration) (Scope, io.Closer) {
	s := newRootScope(opts, interval)
	return s, s
}

func newRootScope(opts ScopeOptions, reportInterval time.Duration) *scope {
	if opts.Tags == nil {
		opts.Tags = make(map[string]string)
	}
	if opts.Separator == "" {
		opts.Separator = DefaultSeparator
	}

	s := &scope{
		prefix:    opts.Prefix,
		reporter:  opts.Reporter,
		separator: opts.Separator,

		registry: &scopeRegistry{
			subscopes: make(map[string]*scope),
		},

		status: scopeStatus{
			closed: false,
			quit:   make(chan struct{}, 1),
		},

		counters: make(map[string]*counter),
		gauges:   make(map[string]*gauge),
	}

	s.tags = s.copyMap(opts.Tags)
	s.registry.subscopes[KeyMap(s.prefix, s.tags)] = s

	if reportInterval > 0 {
		go s.reportLoop(reportInterval)
	}
	return s
}

func (s *scope) reportLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-s.status.quit:
			return
		}
		s.reportLoopRun()
	}
}

func (s *scope) Report() {
	s.reportLoopRun()
}

func (s *scope) reportLoopRun() {
	s.status.Lock()
	defer s.status.Unlock()
	if s.status.closed {
		return
	}
	s.reportRegistryWithLock()
}

func (s *scope) reportRegistryWithLock() {
	s.registry.Lock()
	if s.reporter != nil {
		for _, ss := range s.registry.subscopes {
			ss.report(s.reporter)
		}
	}
	s.registry.Unlock()
}

func (s *scope) report(r StatsReporter) {
	s.cm.Lock()
	for name, counter := range s.counters {
		counter.report(s.fullyQualifiedName(name), s.tags, r)
	}
	s.cm.Unlock()

	s.gm.Lock()
	for name, gauge := range s.gauges {
		gauge.report(s.fullyQualifiedName(name), s.tags, r)
	}
	s.gm.Unlock()

	r.Flush()
}

func (s *scope) Counter(name string) Counter {
	s.cm.Lock()
	val, ok := s.counters[name]
	s.cm.Unlock()
	if !ok {
		return s.counter(name)
	}
	return val
}

func (s *scope) counter(name string) Counter {
	s.cm.Lock()
	defer s.cm.Unlock()
	val, ok := s.counters[name]
	if !ok {
		val = newCounter()
		s.counters[name] = val
	}
	return val
}

func (s *scope) Gauge(name string) Gauge {
	s.gm.Lock()
	val, ok := s.gauges[name]
	s.gm.Unlock()
	if !ok {
		return s.gauge(name)
	}
	return val
}

func (s *scope) gauge(name string) Gauge {
	s.gm.Lock()
	defer s.gm.Unlock()
	val, ok := s.gauges[name]
	if !ok {
		val = newGauge()
		s.gauges[name] = val
	}
	return val
}

func (s *scope) Tagged(tags map[string]string) Scope {
	tags = s.copyMap(tags)
	return s.subscope(s.prefix, tags)
}

func (s *scope) SubScope(prefix string) Scope {
	return s.subscope(s.fullyQualifiedName(prefix), nil)
}

func (s *scope) subscope(prefix string, immutableTags map[string]string) Scope {
	immutableTags = mergeRightTags(s.tags, immutableTags)
	key := KeyMap(prefix, immutableTags)

	s.registry.Lock()
	existing, ok := s.registry.subscopes[key]
	if !ok {
		s.registry.Unlock()
		return s.newSubscope(prefix, immutableTags, key)
	}
	s.registry.Unlock()
	return existing
}

func (s *scope) newSubscope(prefix string, immutableTags map[string]string, key string) Scope {
	s.registry.Lock()
	defer s.registry.Unlock()

	existing, ok := s.registry.subscopes[key]
	if ok {
		return existing
	}

	subscope := &scope{
		prefix:    prefix,
		registry:  s.registry,
		reporter:  s.reporter,
		separator: s.separator,
		tags:      immutableTags,

		counters: make(map[string]*counter),
		gauges:   make(map[string]*gauge),
	}

	s.registry.subscopes[key] = subscope
	return subscope
}

func (s *scope) Snapshot() Snapshot {
	snap := newSnapshot()

	s.registry.Lock()
	for _, ss := range s.registry.subscopes {
		tags := make(map[string]string, len(s.tags))
		for k, v := range ss.tags {
			tags[k] = v
		}

		ss.cm.Lock()
		for key, c := range ss.counters {
			name := ss.fullyQualifiedName(key)
			id := KeyMap(name, tags)
			snap.counters[id] = &counterSnapshot{
				name:  name,
				tags:  tags,
				value: c.snapshot(),
			}
		}
		ss.cm.Unlock()
		ss.gm.Lock()
		for key, g := range ss.gauges {
			name := ss.fullyQualifiedName(key)
			id := KeyMap(name, tags)
			snap.gauges[id] = &gaugeSnapshot{
				name:  name,
				tags:  tags,
				value: g.snapshot(),
			}
		}
		ss.gm.Unlock()
	}
	s.registry.Unlock()

	return snap
}

func (s *scope) Close() error {
	s.status.Lock()

	if s.status.closed {
		s.status.Unlock()
		return nil
	}

	s.status.closed = true
	close(s.status.quit)
	s.reportRegistryWithLock()

	s.status.Unlock()

	if closer, ok := s.reporter.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (s *scope) fullyQualifiedName(name string) string {
	if len(s.prefix) == 0 {
		return name
	}
	return s.prefix + s.separator + name
}

func (s *scope) copyMap(tags map[string]string) map[string]string {
	result := make(map[string]string, len(tags))
	for k, v := range tags {
		result[k] = v
	}
	return result
}

func mergeRightTags(tagsLeft, tagsRight map[string]string) map[string]string {
	if tagsLeft == nil && tagsRight == nil {
		return nil
	}
	if len(tagsRight) == 0 {
		return tagsLeft
	}
	if len(tagsLeft) == 0 {
		return tagsRight
	}

	result := make(map[string]string, len(tagsLeft)+len(tagsRight))
	for k, v := range tagsLeft {
		result[k] = v
	}
	for k, v := range tagsRight {
		result[k] = v
	}
	return result
}

type snapshot struct {
	counters map[string]CounterSnapshot
	gauges   map[string]GaugeSnapshot
}

func newSnapshot() *snapshot {
	return &snapshot{
		counters: make(map[string]CounterSnapshot),
		gauges:   make(map[string]GaugeSnapshot),
	}
}

func (s *snapshot) Counters() map[string]CounterSnapshot {
	return s.counters
}

func (s *snapshot) Gauges() map[string]GaugeSnapshot {
	return s.gauges
}

type counterSnapshot struct {
	name  string
	tags  map[string]string
	value int64
}

func (s *counterSnapshot) Name() string {
	return s.name
}

func (s *counterSnapshot) Tags() map[string]string {
	return s.tags
}

func (s *counterSnapshot) Value() int64 {
	return s.value
}

type gaugeSnapshot struct {
	name  string
	tags  map[string]string
	value float64
}

func (s *gaugeSnapshot) Name() string {
	return s.name
}

func (s *gaugeSnapshot) Tags() map[string]string {
	return s.tags
}

func (s *gaugeSnapshot) Value() float64 {
	return s.value
}
