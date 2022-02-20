package metrics

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/util/workqueue"
)

const (
	addonNamespace           = "addon"
	subsystem                = "manager"
	DefaultMetricsServerPort = 9090
	DefaultMetricsServerPath = "/metrics"
)

type ServerConfig struct {
	Enabled      bool
	Path         string
	Port         int
	TTL          time.Duration
	IgnoreErrors bool
	Secure       bool
}

func (s ServerConfig) SameServerAs(other ServerConfig) bool {
	return s.Port == other.Port && s.Path == other.Path && s.Enabled && other.Enabled && s.Secure == other.Secure
}

type metric struct {
	metric      prometheus.Metric
	lastUpdated time.Time
}

type Metrics struct {
	mutex         sync.RWMutex
	metricsConfig ServerConfig

	addonsProcessed    prometheus.Counter
	addons             map[string][]string
	operationDurations prometheus.Histogram
	errors             map[ErrorCause]prometheus.Counter

	customMetrics    map[string]metric
	workqueueMetrics map[string]prometheus.Metric
	workersBusy      map[string]prometheus.Gauge

	// Used to quickly check if a metric desc is already used by the system
	defaultMetricDescs map[string]bool
	metricNameHelps    map[string]string
	logMetric          *prometheus.CounterVec
}

func (m *Metrics) Levels() []log.Level {
	return []log.Level{log.InfoLevel, log.WarnLevel, log.ErrorLevel}
}

func (m *Metrics) Fire(entry *log.Entry) error {
	m.logMetric.WithLabelValues(entry.Level.String()).Inc()
	return nil
}

var _ prometheus.Collector = &Metrics{}

func New(metricsConfig ServerConfig) *Metrics {
	metrics := &Metrics{
		metricsConfig:   metricsConfig,
		addonsProcessed: newCounter("addons_processed_count", "Number of workflow updates processed", nil),

		addons:             make(map[string][]string),
		operationDurations: newHistogram("operation_duration_seconds", "Histogram of durations of operations", nil, []float64{0.1, 0.25, 0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0, 2.5, 3.0}),
		errors:             getErrorCounters(),
		customMetrics:      make(map[string]metric),
		workqueueMetrics:   make(map[string]prometheus.Metric),
		workersBusy:        make(map[string]prometheus.Gauge),
		defaultMetricDescs: make(map[string]bool),
		metricNameHelps:    make(map[string]string),
		logMetric: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "log_messages",
			Help: "Total number of log messages.",
		}, []string{"level"}),
	}

	for _, metric := range metrics.allMetrics() {
		metrics.defaultMetricDescs[metric.Desc().String()] = true
	}

	for _, level := range metrics.Levels() {
		metrics.logMetric.WithLabelValues(level.String())
	}

	log.AddHook(metrics)

	return metrics
}

func (m *Metrics) allMetrics() []prometheus.Metric {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	allMetrics := []prometheus.Metric{
		m.addonsProcessed,
		m.operationDurations,
	}
	for _, metric := range m.errors {
		allMetrics = append(allMetrics, metric)
	}
	for _, metric := range m.workqueueMetrics {
		allMetrics = append(allMetrics, metric)
	}
	for _, metric := range m.workersBusy {
		allMetrics = append(allMetrics, metric)
	}
	for _, metric := range m.customMetrics {
		allMetrics = append(allMetrics, metric.metric)
	}
	return allMetrics
}

func (m *Metrics) StopRealtimeMetricsForKey(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.addons[key]; !exists {
		return
	}

	realtimeMetrics := m.addons[key]
	for _, metric := range realtimeMetrics {
		delete(m.customMetrics, metric)
	}

	delete(m.addons, key)
}

func (m *Metrics) OperationCompleted(durationSeconds float64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.operationDurations.Observe(durationSeconds)
}

func (m *Metrics) GetCustomMetric(key string) prometheus.Metric {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// It's okay to return nil metrics in this function
	return m.customMetrics[key].metric
}

func (m *Metrics) UpsertCustomMetric(key string, ownerKey string, newMetric prometheus.Metric, realtime bool) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	metricDesc := newMetric.Desc().String()
	if _, inUse := m.defaultMetricDescs[metricDesc]; inUse {
		return fmt.Errorf("metric '%s' is already in use by the system, please use a different name", newMetric.Desc())
	}

	m.customMetrics[key] = metric{metric: newMetric, lastUpdated: time.Now()}

	// If this is a realtime metric, track it
	if realtime {
		m.addons[ownerKey] = append(m.addons[ownerKey], key)
	}

	return nil
}

type ErrorCause string

const (
	ErrorCauseOperationPanic ErrorCause = "OperationPanic"
)

func (m *Metrics) OperationPanic() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.errors[ErrorCauseOperationPanic].Inc()
}

// Act as a metrics provider for a workflow queue
var _ workqueue.MetricsProvider = &Metrics{}

func (m *Metrics) NewDepthMetric(name string) workqueue.GaugeMetric {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := fmt.Sprintf("%s-depth", name)
	if _, ok := m.workqueueMetrics[key]; !ok {
		m.workqueueMetrics[key] = newGauge("queue_depth_count", "Depth of the queue", map[string]string{"queue_name": name})
	}
	return m.workqueueMetrics[key].(prometheus.Gauge)
}

func (m *Metrics) NewAddsMetric(name string) workqueue.CounterMetric {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := fmt.Sprintf("%s-adds", name)
	if _, ok := m.workqueueMetrics[key]; !ok {
		m.workqueueMetrics[key] = newCounter("queue_adds_count", "Adds to the queue", map[string]string{"queue_name": name})
	}
	return m.workqueueMetrics[key].(prometheus.Counter)
}

func (m *Metrics) NewLatencyMetric(name string) workqueue.HistogramMetric {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := fmt.Sprintf("%s-latency", name)
	if _, ok := m.workqueueMetrics[key]; !ok {
		m.workqueueMetrics[key] = newHistogram("queue_latency", "Time objects spend waiting in the queue", map[string]string{"queue_name": name}, []float64{1.0, 5.0, 20.0, 60.0, 180.0})
	}
	return m.workqueueMetrics[key].(prometheus.Histogram)
}

// These metrics are not relevant to be exposed
type noopMetric struct{}

func (noopMetric) Inc()            {}
func (noopMetric) Dec()            {}
func (noopMetric) Set(float64)     {}
func (noopMetric) Observe(float64) {}

func (m *Metrics) NewRetriesMetric(name string) workqueue.CounterMetric        { return noopMetric{} }
func (m *Metrics) NewWorkDurationMetric(name string) workqueue.HistogramMetric { return noopMetric{} }
func (m *Metrics) NewUnfinishedWorkSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return noopMetric{}
}

func (m *Metrics) NewLongestRunningProcessorSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return noopMetric{}
}
