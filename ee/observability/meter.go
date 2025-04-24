package observability

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

const (
	// Units as defined by https://ucum.org
	unitByte    = "B"
	unitPercent = "%"

	// Custom units
	unitRestart = "{restart}"
)

// Initialize all of our meters. All meter names should have "launcher." prepended,
// and use units defined in the consts above.
var (
	// Gauges
	GoMemoryUsageGauge = int64GaugeOrNoop("launcher.memory.golang",
		metric.WithDescription("Go runtime memory usage"),
		metric.WithUnit(unitByte))
	NonGoMemoryUsageGauge = int64GaugeOrNoop("launcher.memory.non_golang",
		metric.WithDescription("Non-Go memory usage"),
		metric.WithUnit(unitByte))
	MemoryPercentGauge = int64GaugeOrNoop("launcher.memory.percent",
		metric.WithDescription("Process memory percent"),
		metric.WithUnit(unitPercent))
	CpuPercentGauge = int64GaugeOrNoop("launcher.cpu.percent",
		metric.WithDescription("Process CPU percent"),
		metric.WithUnit(unitPercent))

	// Counters
	LauncherRestartCounter = int64CounterOrNoop("launcher.restart",
		metric.WithDescription("The number of launcher restarts"),
		metric.WithUnit(unitRestart))
	OsqueryRestartCounter = int64CounterOrNoop("launcher.osquery.restart",
		metric.WithDescription("The number of osquery instance restarts"),
		metric.WithUnit(unitRestart))
)

// int64GaugeOrNoop is guaranteed to return an Int64Gauge -- if we cannot create
// a real Int64Gauge, we return a noop version instead.
func int64GaugeOrNoop(name string, options ...metric.Int64GaugeOption) metric.Int64Gauge {
	gauge, err := otel.Meter(instrumentationPkg).Int64Gauge(name, options...)
	if err != nil {
		return noop.Int64Gauge{}
	}
	return gauge
}

// int64CounterOrNoop is guaranteed to return an Int64Counter -- if we cannot create
// a real Int64Counter, we return a noop version instead.
func int64CounterOrNoop(name string, options ...metric.Int64CounterOption) metric.Int64Counter {
	counter, err := otel.Meter(instrumentationPkg).Int64Counter(name, options...)
	if err != nil {
		return noop.Int64Counter{}
	}
	return counter
}
