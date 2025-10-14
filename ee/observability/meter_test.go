package observability

import (
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// TestReinitializeMetrics does not run in parallel to avoid setting a global meter provider
// too early during the test run.
func TestReinitializeMetrics(t *testing.T) { //nolint:paralleltest
	// On initialization, meters should be non-nil
	require.NotNil(t, GoMemoryUsageGauge)
	require.NotNil(t, RSSHistogram)
	require.NotNil(t, LauncherRestartCounter)

	// Set up a meter provider that writes to a buffer every 100 milliseconds
	writeInterval := 100 * time.Millisecond
	meterOutBytes := &threadsafebuffer.ThreadSafeBuffer{}
	testExporter, err := stdoutmetric.New(stdoutmetric.WithWriter(meterOutBytes))
	require.NoError(t, err)
	testProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(testExporter, sdkmetric.WithInterval(writeInterval))))
	otel.SetMeterProvider(testProvider)

	// We should still be able to use our gauge and counter -- write data and wait
	// for it to be written to our exporter.
	for i := 0; i < 3; i++ {
		time.Sleep(writeInterval)
		GoMemoryUsageGauge.Record(t.Context(), int64(i))
		RSSHistogram.Record(t.Context(), int64(i))
		LauncherRestartCounter.Add(t.Context(), int64(i))
	}
	time.Sleep(writeInterval)

	// Confirm we exported data
	require.Greater(t, len(meterOutBytes.String()), 0)

	// Now, shut down the provider
	require.NoError(t, testProvider.Shutdown(t.Context()))

	// Meters should still be non-nil
	require.NotNil(t, GoMemoryUsageGauge)
	require.NotNil(t, RSSHistogram)
	require.NotNil(t, LauncherRestartCounter)

	// Reinitialize the meters
	ReinitializeMetrics()

	// Set up a new meter provider that writes to a new buffer every 100 milliseconds
	secondMeterOutBytes := &threadsafebuffer.ThreadSafeBuffer{}
	secondTestExporter, err := stdoutmetric.New(stdoutmetric.WithWriter(secondMeterOutBytes))
	require.NoError(t, err)
	secondTestProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(secondTestExporter, sdkmetric.WithInterval(writeInterval))))
	otel.SetMeterProvider(secondTestProvider)

	// Meters should still be non-nil
	require.NotNil(t, GoMemoryUsageGauge)
	require.NotNil(t, RSSHistogram)
	require.NotNil(t, LauncherRestartCounter)

	// We should still be able to use our gauge and counter -- write data and wait
	// for it to be written to our new exporter.
	for i := 0; i < 3; i++ {
		time.Sleep(writeInterval)
		GoMemoryUsageGauge.Record(t.Context(), int64(i))
		RSSHistogram.Record(t.Context(), int64(i))
		LauncherRestartCounter.Add(t.Context(), int64(i))
	}
	time.Sleep(writeInterval)

	// Confirm we exported data via the new provider.
	require.Greater(t, len(secondMeterOutBytes.String()), 0)

	// Confirm we can shut down the new provider.
	require.NoError(t, secondTestProvider.Shutdown(t.Context()))
}

// Test_int64GaugeOrNoop does not run in parallel to avoid setting a global meter provider
// too early during the test run.
func Test_int64GaugeOrNoop(t *testing.T) { //nolint:paralleltest
	// Before we set up the meter provider, we should still get a usable int64 gauge
	testGauge := int64GaugeOrNoop("launcher.test.gauge", metric.WithUnit(unitByte))
	require.NotNil(t, testGauge)
	testGauge.Record(t.Context(), 5)

	// Set up a meter provider that writes to a buffer every 100 milliseconds
	writeInterval := 100 * time.Millisecond
	meterOutBytes := &threadsafebuffer.ThreadSafeBuffer{}
	testExporter, err := stdoutmetric.New(stdoutmetric.WithWriter(meterOutBytes))
	require.NoError(t, err)
	testProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(testExporter, sdkmetric.WithInterval(writeInterval))))
	otel.SetMeterProvider(testProvider)

	// We should still be able to use our test gauge -- write data and wait
	// for it to be written to our exporter.
	for i := 0; i < 3; i++ {
		time.Sleep(writeInterval)
		testGauge.Record(t.Context(), int64(i))
	}
	time.Sleep(writeInterval)

	// Confirm we exported data
	require.Greater(t, len(meterOutBytes.String()), 0)
}

// Test_float64GaugeOrNoop does not run in parallel to avoid setting a global meter provider
// too early during the test run.
func Test_float64GaugeOrNoop(t *testing.T) { //nolint:paralleltest
	// Before we set up the meter provider, we should still get a usable float64 gauge
	testGauge := float64GaugeOrNoop("launcher.test.gauge", metric.WithUnit(unitPercent))
	require.NotNil(t, testGauge)
	testGauge.Record(t.Context(), 5)

	// Set up a meter provider that writes to a buffer every 100 milliseconds
	writeInterval := 100 * time.Millisecond
	meterOutBytes := &threadsafebuffer.ThreadSafeBuffer{}
	testExporter, err := stdoutmetric.New(stdoutmetric.WithWriter(meterOutBytes))
	require.NoError(t, err)
	testProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(testExporter, sdkmetric.WithInterval(writeInterval))))
	otel.SetMeterProvider(testProvider)

	// We should still be able to use our test gauge -- write data and wait
	// for it to be written to our exporter.
	for i := range 3 {
		time.Sleep(writeInterval)
		testGauge.Record(t.Context(), float64(i))
	}
	time.Sleep(writeInterval)

	// Confirm we exported data
	require.Greater(t, len(meterOutBytes.String()), 0)
}

// Test_int64HistogramOrNoop does not run in parallel to avoid setting a global meter provider
// too early during the test run.
func Test_int64HistogramOrNoop(t *testing.T) { //nolint:paralleltest
	// Before we set up the meter provider, we should still get a usable int64 gauge
	testHist := int64HistogramOrNoop("launcher.test.histogram", metric.WithUnit(unitByte))
	require.NotNil(t, testHist)
	testHist.Record(t.Context(), 5)

	// Set up a meter provider that writes to a buffer every 100 milliseconds
	writeInterval := 100 * time.Millisecond
	meterOutBytes := &threadsafebuffer.ThreadSafeBuffer{}
	testExporter, err := stdoutmetric.New(stdoutmetric.WithWriter(meterOutBytes))
	require.NoError(t, err)
	testProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(testExporter, sdkmetric.WithInterval(writeInterval))))
	otel.SetMeterProvider(testProvider)

	// We should still be able to use our test gauge -- write data and wait
	// for it to be written to our exporter.
	for i := 0; i < 3; i++ {
		time.Sleep(writeInterval)
		testHist.Record(t.Context(), int64(i))
	}
	time.Sleep(writeInterval)

	// Confirm we exported data
	require.Greater(t, len(meterOutBytes.String()), 0)
}

// Test_float64HistogramOrNoop does not run in parallel to avoid setting a global meter provider
// too early during the test run.
func Test_float64HistogramOrNoop(t *testing.T) { //nolint:paralleltest
	// Before we set up the meter provider, we should still get a usable int64 gauge
	testHist := float64HistogramOrNoop("launcher.test.histogram", metric.WithUnit(unitByte))
	require.NotNil(t, testHist)
	testHist.Record(t.Context(), 5)

	// Set up a meter provider that writes to a buffer every 100 milliseconds
	writeInterval := 100 * time.Millisecond
	meterOutBytes := &threadsafebuffer.ThreadSafeBuffer{}
	testExporter, err := stdoutmetric.New(stdoutmetric.WithWriter(meterOutBytes))
	require.NoError(t, err)
	testProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(testExporter, sdkmetric.WithInterval(writeInterval))))
	otel.SetMeterProvider(testProvider)

	// We should still be able to use our test gauge -- write data and wait
	// for it to be written to our exporter.
	for i := 0; i < 3; i++ {
		time.Sleep(writeInterval)
		testHist.Record(t.Context(), float64(i))
	}
	time.Sleep(writeInterval)

	// Confirm we exported data
	require.Greater(t, len(meterOutBytes.String()), 0)
}

// Test_int64CounterOrNoop does not run in parallel to avoid setting a global meter provider
// too early during the test run.
func Test_int64CounterOrNoop(t *testing.T) { //nolint:paralleltest
	// Before we set up the meter provider, we should still get a usable int64 counter
	testCounter := int64CounterOrNoop("launcher.test.gauge", metric.WithUnit(unitByte))
	require.NotNil(t, testCounter)
	testCounter.Add(t.Context(), 1)

	// Set up a meter provider that writes to a buffer every 100 milliseconds
	writeInterval := 100 * time.Millisecond
	meterOutBytes := &threadsafebuffer.ThreadSafeBuffer{}
	testExporter, err := stdoutmetric.New(stdoutmetric.WithWriter(meterOutBytes))
	require.NoError(t, err)
	testProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(testExporter, sdkmetric.WithInterval(writeInterval))))
	otel.SetMeterProvider(testProvider)

	// We should still be able to use our test gauge -- write data and wait
	// for it to be written to our exporter.
	for i := 0; i < 3; i++ {
		time.Sleep(writeInterval)
		testCounter.Add(t.Context(), int64(i))
	}
	time.Sleep(writeInterval)

	// Confirm we exported data
	require.Greater(t, len(meterOutBytes.String()), 0)
}
