package exporter

import (
	"runtime"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/traces/bufspanprocessor"
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

	bufferedSpanProcessor := &bufspanprocessor.BufSpanProcessor{
		MaxBufferedSpans: 500,
	}

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

func initialAttrs() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(applicationName),
		semconv.ServiceVersion(version.Version().Version),
	}

	if archAttr, ok := archAttributeMap[runtime.GOARCH]; ok {
		attrs = append(attrs, archAttr)
	}

	return attrs
}
