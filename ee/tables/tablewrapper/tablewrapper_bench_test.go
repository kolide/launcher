package tablewrapper

import (
	"context"
	"testing"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/flags/keys"
	typesmocks "github.com/kolide/launcher/v2/ee/agent/types/mocks"
	"github.com/kolide/launcher/v2/ee/observability"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/mock"
	"go.opentelemetry.io/otel"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// benchmarkGenerate measures the full wrapped generate path -- span, worker
// semaphore, goroutine handoff, and health counters -- around a trivial gen func.
func benchmarkGenerate(b *testing.B) {
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return()

	wt := newWrappedTable(mockFlags, multislogger.NewNopLogger(), "bench_table", func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{{"somekey": "somevalue"}}, nil
	})

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := wt.generate(ctx, table.QueryContext{}); err != nil {
			b.Fatal(err)
		}
	}
}

// Baseline: health counters are noops (no meter provider configured, e.g. telemetry disabled).
func BenchmarkGenerate_MetricsDisabled(b *testing.B) {
	otel.SetMeterProvider(metricnoop.NewMeterProvider())
	observability.ReinitializeMetrics()
	benchmarkGenerate(b)
}

// Health counters recording to a real SDK meter provider, as in production with telemetry enabled.
func BenchmarkGenerate_MetricsEnabled(b *testing.B) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	observability.ReinitializeMetrics()
	b.Cleanup(func() {
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		observability.ReinitializeMetrics()
	})
	benchmarkGenerate(b)
}
