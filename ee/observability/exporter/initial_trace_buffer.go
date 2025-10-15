package exporter

import (
	"log/slog"
	"runtime"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/observability/bufspanprocessor"
	osquerygotraces "github.com/osquery/osquery-go/traces"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

type InitialTraceBuffer struct {
	provider         *sdktrace.TracerProvider
	bufSpanProcessor *bufspanprocessor.BufSpanProcessor
	attrs            []attribute.KeyValue // resource attributes, identifying this device + installation
}

func NewInitialTraceBuffer() *InitialTraceBuffer {
	attrs := initialAttrs()

	defaultResource := resource.Default()
	r, err := resource.Merge(
		defaultResource,
		resource.NewWithAttributes(defaultResource.SchemaURL(), attrs...),
	)
	if err != nil {
		r = resource.Default()
	}

	bufferedSpanProcessor := bufspanprocessor.NewBufSpanProcessor(500)

	newProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(bufferedSpanProcessor),
		sdktrace.WithResource(r),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))),
	)

	otel.SetTracerProvider(newProvider)
	osquerygotraces.SetTracerProvider(newProvider)

	return &InitialTraceBuffer{
		provider:         newProvider,
		bufSpanProcessor: bufferedSpanProcessor,
		attrs:            attrs,
	}
}

func (i *InitialTraceBuffer) SetSlogger(slogger *slog.Logger) {
	i.bufSpanProcessor.SetSlogger(slogger)
}

func initialAttrs() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(applicationName),
		semconv.ServiceVersion(version.Version().Version),
		attribute.String("launcher.goos", runtime.GOOS),
		// Calling GOMAXPROCS with value `0` does not change the value of this variable;
		// it just returns the current value.
		attribute.Int("launcher.gomaxprocs", runtime.GOMAXPROCS(0)),
	}

	if archAttr, ok := archAttributeMap[runtime.GOARCH]; ok {
		attrs = append(attrs, archAttr)
	}

	return attrs
}
