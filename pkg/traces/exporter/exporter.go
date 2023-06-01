package exporter

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/traces"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

var archAttributeMap = map[string]attribute.KeyValue{
	"amd64": semconv.HostArchAMD64,
	"386":   semconv.HostArchX86,
	"arm64": semconv.HostArchARM64,
	"arm":   semconv.HostArchARM32,
}

type TraceExporter struct {
	provider *trace.TracerProvider
}

// NewTraceExporter sets up our traces to be exported via OTLP over HTTP.
// On interrupt, the provider will be shut down.
func NewTraceExporter(ctx context.Context, serverProvidedDataStore types.Getter, client osquery.Querier) (*TraceExporter, error) {
	traceClient := otlptracehttp.NewClient(otlptracehttp.WithInsecure())
	exp, err := otlptrace.New(ctx, traceClient)
	if err != nil {
		return nil, fmt.Errorf("could not create exporter for traces: %w", err)
	}

	traceExporter := &TraceExporter{}

	// Set up the exporter in the background -- we need to wait for the osquery client to be available, but don't
	// want to block on it.
	go traceExporter.configureProvider(exp, serverProvidedDataStore, client)

	return traceExporter, nil
}

// configureProvider sets up the trace provider attributes describing this application and installation,
// uniquely identifying the source of these traces.
func (t *TraceExporter) configureProvider(exp *otlptrace.Exporter, serverProvidedDataStore types.Getter, client osquery.Querier) {
	// Set all the attributes that we know we can get first
	attrs := []attribute.KeyValue{
		semconv.ServiceName(traces.ApplicationName),
		semconv.ServiceVersion(version.Version().Version),
	}

	if archAttr, ok := archAttributeMap[runtime.GOARCH]; ok {
		attrs = append(attrs, archAttr)
	}

	// Device identifiers -- available in bucket data.
	if deviceId, err := serverProvidedDataStore.Get([]byte("device_id")); err == nil {
		attrs = append(attrs, semconv.ServiceInstanceID(string(deviceId)))
	}

	if munemo, err := serverProvidedDataStore.Get([]byte("munemo")); err == nil {
		attrs = append(attrs, attribute.String("launcher.munemo", string(munemo)))
	}

	if orgId, err := serverProvidedDataStore.Get([]byte("organization_id")); err == nil {
		attrs = append(attrs, attribute.String("launcher.organization_id", string(orgId)))
	}

	if serialNumber, err := serverProvidedDataStore.Get([]byte("serial_number")); err == nil {
		attrs = append(attrs, attribute.String("launcher.serial", string(serialNumber)))
	}

	// Device and OS details -- available via osquery.
	// The osqueryd client may not have initialized yet, so retry a few times on error.
	osquerydRetries := 5
	for i := 0; i < osquerydRetries; i += 1 {
		query := `
SELECT
	osquery_info.version as osquery_version,
	os_version.name as os_name,
	os_version.version as os_version,
	system_info.hostname
FROM
	os_version,
	system_info,
	osquery_info;
`

		resp, err := client.Query(query)
		if err != nil || len(resp) == 0 {
			time.Sleep(30 * time.Second)
			continue
		}

		attrs = append(attrs,
			attribute.String("launcher.osquery_version", resp[0]["osquery_version"]),
			semconv.OSName(resp[0]["os_name"]),
			semconv.OSVersion(resp[0]["os_version"]),
			semconv.HostName("hostname"),
		)
		break
	}

	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
	if err != nil {
		r = resource.Default()
	}

	t.provider = trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(r),
	)

	otel.SetTracerProvider(t.provider)
}

func (t *TraceExporter) Execute() error {
	// Does nothing, just waiting for launcher to exit
	<-context.Background().Done()
	return nil
}

func (t *TraceExporter) Interrupt(_ error) {
	if t.provider != nil {
		t.provider.Shutdown(context.Background())
	}
}
