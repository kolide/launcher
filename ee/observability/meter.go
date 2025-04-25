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

	// Define our meter names and descriptions. All meter names should have "launcher." prepended.
	goMemoryUsageGaugeName            = "launcher.memory.golang"
	goMemoryUsageGaugeDescription     = "Go runtime memory usage"
	nonGoMemoryUsageGaugeName         = "launcher.memory.non_golang"
	nonGoMemoryUsageGaugeDescription  = "Non-Go memory usage"
	memoryPercentGaugeName            = "launcher.memory.percent"
	memoryPercentGaugeDescription     = "Process memory percent"
	cpuPercentGaugeName               = "launcher.cpu.percent"
	cpuPercentGaugeDescription        = "Process CPU percent"
	launcherRestartCounterName        = "launcher.restart"
	launcherRestartCounterDescription = "The number of launcher restarts"
	osqueryRestartCounterName         = "launcher.osquery.restart"
	osqueryRestartCounterDescription  = "The number of osquery instance restarts"
)

var (
	// Gauges
	GoMemoryUsageGauge    metric.Int64Gauge
	NonGoMemoryUsageGauge metric.Int64Gauge
	MemoryPercentGauge    metric.Int64Gauge
	CpuPercentGauge       metric.Int64Gauge

	// Counters
	LauncherRestartCounter metric.Int64Counter
	OsqueryRestartCounter  metric.Int64Counter
)

// Initialize all of our meters. All meter names should have "launcher." prepended,
// and use units defined in the consts above.
func init() {
	ReinitializeMetrics()
}

// ReinitializeMetrics creates or re-creates all our gauges and counters; it should be called
// on initialization and any time the meter provider is replaced thereafter. All meters should
// use names, descriptions, and units defined in consts atove.
func ReinitializeMetrics() {
	// Gauges
	GoMemoryUsageGauge = int64GaugeOrNoop(goMemoryUsageGaugeName,
		metric.WithDescription(goMemoryUsageGaugeDescription),
		metric.WithUnit(unitByte))
	NonGoMemoryUsageGauge = int64GaugeOrNoop(nonGoMemoryUsageGaugeName,
		metric.WithDescription(nonGoMemoryUsageGaugeDescription),
		metric.WithUnit(unitByte))
	MemoryPercentGauge = int64GaugeOrNoop(memoryPercentGaugeName,
		metric.WithDescription(memoryPercentGaugeDescription),
		metric.WithUnit(unitPercent))
	CpuPercentGauge = int64GaugeOrNoop(cpuPercentGaugeName,
		metric.WithDescription(cpuPercentGaugeDescription),
		metric.WithUnit(unitPercent))

	// Counters
	LauncherRestartCounter = int64CounterOrNoop(launcherRestartCounterName,
		metric.WithDescription(launcherRestartCounterDescription),
		metric.WithUnit(unitRestart))
	OsqueryRestartCounter = int64CounterOrNoop(osqueryRestartCounterName,
		metric.WithDescription(osqueryRestartCounterDescription),
		metric.WithUnit(unitRestart))
}

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
