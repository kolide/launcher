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
	unitByteGCP = "By" // Unfortunately, "B" isn't recognized by our metrics ingest -- we have to use "By" instead

	// Custom units
	unitRestart = "{restart}"
	unitFailure = "{failure}"
	unitReads   = "{reads}"
	unitWrites  = "{writes}"

	// Define our meter names and descriptions. All meter names should have "launcher." prepended.
	goMemoryUsageGaugeName                       = "launcher.memory.golang"
	goMemoryUsageGaugeDescription                = "Go runtime memory usage"
	nonGoMemoryUsageGaugeName                    = "launcher.memory.non_golang"
	nonGoMemoryUsageGaugeDescription             = "Non-Go memory usage"
	memoryPercentGaugeName                       = "launcher.memory.percent"
	memoryPercentGaugeDescription                = "Process memory percent"
	cpuPercentGaugeName                          = "launcher.cpu.percent"
	cpuPercentGaugeDescription                   = "Process CPU percent"
	checkupScoreGaugeName                        = "launcher.checkup.score"
	checkupScoreGaugeDescription                 = "Computed checkup score"
	rssHistogramName                             = "launcher.memory.rss"
	rssHistogramDescription                      = "launcher process RSS bytes"
	osqueryCpuHistogramName                      = "launcher.osquery.cpu.percent"
	osqueryCpuHistogramDescription               = "osquery process CPU percent"
	osqueryRssHistogramName                      = "launcher.osquery.memory.rss"
	osqueryRssHistogramDescription               = "osquery process RSS bytes"
	desktopCpuHistogramName                      = "launcher.desktop.cpu.percent"
	desktopCpuHistogramDescription               = "Desktop process CPU percent"
	desktopRssHistogramName                      = "launcher.desktop.memory.rss"
	desktopRssHistogramDescription               = "Desktop process RSS bytes"
	launcherRestartCounterName                   = "launcher.restart"
	launcherRestartCounterDescription            = "The number of launcher restarts"
	osqueryRestartCounterName                    = "launcher.osquery.restart"
	osqueryRestartCounterDescription             = "The number of osquery instance restarts"
	windowsUpdatesQueryFailureCounterName        = "launcher.windowsupdates.query.failed"
	windowsUpdatesQueryFailureCounterDescription = "The number of failures when querying the Windows Update Agent API"
	tablewrapperTimeoutCounterName               = "launcher.tablewrapper.timeout"
	tablewrapperTimeoutCounterDescription        = "The number of timeouts when querying a Kolide extension table"
	autoupdateFailureCounterName                 = "launcher.autoupdate.failed"
	autoupdateFailureCounterDescription          = "The number of TUF autoupdate failures"
	checkupErrorCounterName                      = "launcher.checkup.error"
	checkupErrorCounterDescription               = "The number of errors when running checkups"
	ioReadsCounterName                           = "launcher.io.reads"
	ioReadsCounterDescription                    = "IO read counter for launcher"
	ioWritesCounterName                          = "launcher.io.writes"
	ioWritesCounterDescription                   = "IO write counter for launcher"
)

var (
	// Gauges
	GoMemoryUsageGauge    metric.Int64Gauge
	NonGoMemoryUsageGauge metric.Int64Gauge
	MemoryPercentGauge    metric.Int64Gauge
	CpuPercentGauge       metric.Int64Gauge
	CheckupScoreGauge     metric.Float64Gauge

	// Histograms
	RSSHistogram               metric.Int64Histogram
	OsqueryCpuPercentHistogram metric.Float64Histogram
	OsqueryRssHistogram        metric.Int64Histogram
	DesktopCpuPercentHistogram metric.Float64Histogram
	DesktopRssHistogram        metric.Int64Histogram

	// Counters
	LauncherRestartCounter            metric.Int64Counter
	OsqueryRestartCounter             metric.Int64Counter
	WindowsUpdatesQueryFailureCounter metric.Int64Counter
	TablewrapperTimeoutCounter        metric.Int64Counter
	AutoupdateFailureCounter          metric.Int64Counter
	CheckupErrorCounter               metric.Int64Counter
	IOReadsCounter                    metric.Int64Counter
	IOWritesCounter                   metric.Int64Counter
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
	CheckupScoreGauge = float64GaugeOrNoop(checkupScoreGaugeName,
		metric.WithDescription(checkupScoreGaugeDescription),
		metric.WithUnit(unitPercent))

	// Histograms
	RSSHistogram = int64HistogramOrNoop(rssHistogramName,
		metric.WithDescription(rssHistogramDescription),
		metric.WithUnit(unitByte))
	OsqueryCpuPercentHistogram = float64HistogramOrNoop(osqueryCpuHistogramName,
		metric.WithDescription(osqueryCpuHistogramDescription),
		metric.WithUnit(unitPercent))
	OsqueryRssHistogram = int64HistogramOrNoop(osqueryRssHistogramName,
		metric.WithDescription(osqueryRssHistogramDescription),
		metric.WithUnit(unitByte))
	DesktopCpuPercentHistogram = float64HistogramOrNoop(desktopCpuHistogramName,
		metric.WithDescription(desktopCpuHistogramDescription),
		metric.WithUnit(unitPercent))
	DesktopRssHistogram = int64HistogramOrNoop(desktopRssHistogramName,
		metric.WithDescription(desktopRssHistogramDescription),
		metric.WithUnit(unitByteGCP))

	// Counters
	LauncherRestartCounter = int64CounterOrNoop(launcherRestartCounterName,
		metric.WithDescription(launcherRestartCounterDescription),
		metric.WithUnit(unitRestart))
	OsqueryRestartCounter = int64CounterOrNoop(osqueryRestartCounterName,
		metric.WithDescription(osqueryRestartCounterDescription),
		metric.WithUnit(unitRestart))
	WindowsUpdatesQueryFailureCounter = int64CounterOrNoop(windowsUpdatesQueryFailureCounterName,
		metric.WithDescription(windowsUpdatesQueryFailureCounterDescription),
		metric.WithUnit(unitFailure))
	TablewrapperTimeoutCounter = int64CounterOrNoop(tablewrapperTimeoutCounterName,
		metric.WithDescription(tablewrapperTimeoutCounterDescription),
		metric.WithUnit(unitFailure))
	AutoupdateFailureCounter = int64CounterOrNoop(autoupdateFailureCounterName,
		metric.WithDescription(autoupdateFailureCounterDescription),
		metric.WithUnit(unitFailure))
	CheckupErrorCounter = int64CounterOrNoop(checkupErrorCounterName,
		metric.WithDescription(checkupErrorCounterDescription),
		metric.WithUnit(unitFailure))
	IOReadsCounter = int64CounterOrNoop(ioReadsCounterName,
		metric.WithDescription(ioReadsCounterDescription),
		metric.WithUnit(unitReads))
	IOWritesCounter = int64CounterOrNoop(ioWritesCounterName,
		metric.WithDescription(ioWritesCounterDescription),
		metric.WithUnit(unitWrites))
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

// float64GaugeOrNoop is guaranteed to return an Float64Gauge -- if we cannot create
// a real Float64Gauge, we return a noop version instead.
func float64GaugeOrNoop(name string, options ...metric.Float64GaugeOption) metric.Float64Gauge {
	gauge, err := otel.Meter(instrumentationPkg).Float64Gauge(name, options...)
	if err != nil {
		return noop.Float64Gauge{}
	}
	return gauge
}

// int64HistogramOrNoop is guaranteed to return an Int64Histogram -- if we cannot create
// a real Int64Histogram, we return a noop version instead.
func int64HistogramOrNoop(name string, options ...metric.Int64HistogramOption) metric.Int64Histogram {
	hist, err := otel.Meter(instrumentationPkg).Int64Histogram(name, options...)
	if err != nil {
		return noop.Int64Histogram{}
	}
	return hist
}

// int64HistogramOrNoop is guaranteed to return an Float64Histogram -- if we cannot create
// a real Float64Histogram, we return a noop version instead.
func float64HistogramOrNoop(name string, options ...metric.Float64HistogramOption) metric.Float64Histogram {
	hist, err := otel.Meter(instrumentationPkg).Float64Histogram(name, options...)
	if err != nil {
		return noop.Float64Histogram{}
	}
	return hist
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
