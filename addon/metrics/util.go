package metrics

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	descRegex = regexp.MustCompile(fmt.Sprintf(`Desc{fqName: "%s_%s_(.+?)", help: "(.+?)", constLabels: {`, addonNamespace, subsystem))
)

func newCounter(name, help string, labels map[string]string) prometheus.Counter {
	counterOpts := prometheus.CounterOpts{
		Namespace:   addonNamespace,
		Subsystem:   subsystem,
		Name:        name,
		Help:        help,
		ConstLabels: labels,
	}
	m := prometheus.NewCounter(counterOpts)
	mustBeRecoverable(name, help, m)
	return m
}

func newGauge(name, help string, labels map[string]string) prometheus.Gauge {
	gaugeOpts := prometheus.GaugeOpts{
		Namespace:   addonNamespace,
		Subsystem:   subsystem,
		Name:        name,
		Help:        help,
		ConstLabels: labels,
	}
	m := prometheus.NewGauge(gaugeOpts)
	mustBeRecoverable(name, help, m)
	return m
}

func newHistogram(name, help string, labels map[string]string, buckets []float64) prometheus.Histogram {
	histOpts := prometheus.HistogramOpts{
		Namespace:   addonNamespace,
		Subsystem:   subsystem,
		Name:        name,
		Help:        help,
		ConstLabels: labels,
		Buckets:     buckets,
	}
	m := prometheus.NewHistogram(histOpts)
	mustBeRecoverable(name, help, m)
	return m
}

func getErrorCounters() map[ErrorCause]prometheus.Counter {
	getOptsByPahse := func(phase ErrorCause) prometheus.CounterOpts {
		return prometheus.CounterOpts{
			Namespace:   addonNamespace,
			Subsystem:   subsystem,
			Name:        "error_count",
			Help:        "Number of errors encountered by the controller by cause",
			ConstLabels: map[string]string{"cause": string(phase)},
		}
	}
	return map[ErrorCause]prometheus.Counter{
		ErrorCauseOperationPanic: prometheus.NewCounter(getOptsByPahse(ErrorCauseOperationPanic)),
	}
}

func getWorkersBusy(name string) prometheus.Gauge {
	return prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   addonNamespace,
		Name:        "workers_busy_count",
		Help:        "Number of workers currently busy",
		ConstLabels: map[string]string{"worker_type": name},
	})
}

func mustBeRecoverable(name, help string, metric prometheus.Metric) {
	recoveredName, recoveredHelp := recoverMetricNameAndHelpFromDesc(metric.Desc().String())
	if name != recoveredName {
		panic(fmt.Sprintf("unable to recover metric name from desc provided by prometheus: expected '%s' got '%s'", name, recoveredName))
	}
	if help != recoveredHelp {
		panic(fmt.Sprintf("unable to recover metric help from desc provided by prometheus: expected '%s' got '%s'", help, recoveredHelp))
	}
}

func recoverMetricNameAndHelpFromDesc(desc string) (string, string) {
	finds := descRegex.FindStringSubmatch(desc)
	if len(finds) != 3 {
		panic(fmt.Sprintf("malformed desc provided by prometheus: '%s' parsed to %v", desc, finds))
	}
	return finds[1], strings.ReplaceAll(finds[2], `\"`, `"`)
}
