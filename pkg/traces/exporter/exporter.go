package exporter

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/osquery"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

const applicationName = "launcher"

var archAttributeMap = map[string]attribute.KeyValue{
	"amd64": semconv.HostArchAMD64,
	"386":   semconv.HostArchX86,
	"arm64": semconv.HostArchARM64,
	"arm":   semconv.HostArchARM32,
}

type TraceExporter struct {
	provider                *sdktrace.TracerProvider
	serverProvidedDataStore types.Getter
	osqueryClient           osquery.Querier
	attrs                   []attribute.KeyValue // resource attributes, identifying this device + installation
	attrLock                sync.RWMutex
}

// NewTraceExporter sets up our traces to be exported via OTLP over HTTP.
// On interrupt, the provider will be shut down.
func NewTraceExporter(ctx context.Context, serverProvidedDataStore types.Getter, client osquery.Querier) (*TraceExporter, error) {
	// Set all the attributes that we know we can get first
	attrs := []attribute.KeyValue{
		semconv.ServiceName(applicationName),
		semconv.ServiceVersion(version.Version().Version),
	}

	if archAttr, ok := archAttributeMap[runtime.GOARCH]; ok {
		attrs = append(attrs, archAttr)
	}

	t := &TraceExporter{
		serverProvidedDataStore: serverProvidedDataStore,
		osqueryClient:           client,
		attrs:                   attrs,
		attrLock:                sync.RWMutex{},
	}

	t.addDeviceIdentifyingAttributes()

	// Set the provider with as many resource attributes as we could get immediately
	exp, err := newExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not create new exporter: %w", err)
	}
	t.setNewGlobalProvider(exp)

	// In the background, wait for osquery to be ready so that we can fetch more resource
	// attributes for our traces, then replace the provider with a new one.
	go func() {
		t.addAttributesFromOsquery()
		if exp, err := newExporter(ctx); err == nil {
			t.setNewGlobalProvider(exp)
		}
	}()

	return t, nil
}

// newExporter returns an exporter that will send traces with OTLP over HTTP.
func newExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	traceClient := otlptracegrpc.NewClient(otlptracegrpc.WithInsecure())
	exp, err := otlptrace.New(ctx, traceClient)
	if err != nil {
		return nil, fmt.Errorf("could not create exporter for traces: %w", err)
	}

	return exp, nil
}

// addDeviceIdentifyingAttributes gets device identifiers from the server-provided
// data and adds them to our resource attributes.
func (t *TraceExporter) addDeviceIdentifyingAttributes() {
	t.attrLock.Lock()
	defer t.attrLock.Unlock()

	if deviceId, err := t.serverProvidedDataStore.Get([]byte("device_id")); err == nil {
		t.attrs = append(t.attrs, semconv.ServiceInstanceID(string(deviceId)))
	}

	if munemo, err := t.serverProvidedDataStore.Get([]byte("munemo")); err == nil {
		t.attrs = append(t.attrs, attribute.String("launcher.munemo", string(munemo)))
	}

	if orgId, err := t.serverProvidedDataStore.Get([]byte("organization_id")); err == nil {
		t.attrs = append(t.attrs, attribute.String("launcher.organization_id", string(orgId)))
	}

	if serialNumber, err := t.serverProvidedDataStore.Get([]byte("serial_number")); err == nil {
		t.attrs = append(t.attrs, attribute.String("launcher.serial", string(serialNumber)))
	}
}

// addAttributesFromOsquery retrieves device and OS details from osquery and adds them
// to our resource attributes. Since this is called on startup when the osquery client
// may not be ready yet, we perform a few retries.
func (t *TraceExporter) addAttributesFromOsquery() {
	t.attrLock.Lock()
	defer t.attrLock.Unlock()

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

		resp, err := t.osqueryClient.Query(query)
		if err != nil || len(resp) == 0 {
			time.Sleep(30 * time.Second)
			continue
		}

		t.attrs = append(t.attrs,
			attribute.String("launcher.osquery_version", resp[0]["osquery_version"]),
			semconv.OSName(resp[0]["os_name"]),
			semconv.OSVersion(resp[0]["os_version"]),
			semconv.HostName("hostname"),
		)
		return
	}
}

// setNewGlobalProvider creates and sets a new global provider with the currently-available
// attributes. If a provider was previously set, it will be shut down.
func (t *TraceExporter) setNewGlobalProvider(exp sdktrace.SpanExporter) {
	t.attrLock.RLock()
	defer t.attrLock.RUnlock()

	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, t.attrs...),
	)
	if err != nil {
		r = resource.Default()
	}

	newProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(r),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(newProvider)

	if t.provider != nil {
		t.provider.Shutdown(context.Background())
	}

	t.provider = newProvider
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
