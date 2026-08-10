package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func benchCounter(b *testing.B) metric.Int64Counter {
	b.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	counter, err := mp.Meter("bench").Int64Counter("launcher.tablewrapper.error")
	if err != nil {
		b.Fatal(err)
	}
	return counter
}

// Building the attribute set on every Add call -- what NOT to do in a hot path.
func BenchmarkCounterAdd_PerCallAttrs(b *testing.B) {
	counter := benchCounter(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Add(ctx, 1, metric.WithAttributes(attribute.String("table_name", "kolide_macho_info")))
	}
}

// Attribute set precomputed once and reused, as tablewrapper does.
func BenchmarkCounterAdd_PrecomputedAttrs(b *testing.B) {
	counter := benchCounter(b)
	ctx := context.Background()
	attrs := metric.WithAttributeSet(attribute.NewSet(attribute.String("table_name", "kolide_macho_info")))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Add(ctx, 1, attrs)
	}
}
