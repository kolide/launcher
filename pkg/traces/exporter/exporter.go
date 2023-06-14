package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent/flags/keys"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/osquery"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
)

const (
	applicationName     = "launcher"
	IngestSubsystem     = "ingest"
	configStoreTokenKey = "ingest_server_token"
)

type traceSubsystemConfig struct {
	IngestToken string `json:"ingest_token"`
}

var archAttributeMap = map[string]attribute.KeyValue{
	"amd64": semconv.HostArchAMD64,
	"386":   semconv.HostArchX86,
	"arm64": semconv.HostArchARM64,
	"arm":   semconv.HostArchARM32,
}

var osqueryClientRecheckInterval = 30 * time.Second

type TraceExporter struct {
	provider         *sdktrace.TracerProvider
	knapsack         types.Knapsack
	osqueryClient    osquery.Querier
	logger           log.Logger
	attrs            []attribute.KeyValue // resource attributes, identifying this device + installation
	attrLock         sync.RWMutex
	ingestAuthToken  string
	ingestUrl        string
	disableIngestTLS bool
	enabled          bool
}

// NewTraceExporter sets up our traces to be exported via OTLP over HTTP.
// On interrupt, the provider will be shut down.
func NewTraceExporter(ctx context.Context, k types.Knapsack, client osquery.Querier, logger log.Logger) (*TraceExporter, error) {
	// Set all the attributes that we know we can get first
	attrs := []attribute.KeyValue{
		semconv.ServiceName(applicationName),
		semconv.ServiceVersion(version.Version().Version),
	}

	if archAttr, ok := archAttributeMap[runtime.GOARCH]; ok {
		attrs = append(attrs, archAttr)
	}

	currentToken, _ := k.ConfigStore().Get([]byte(configStoreTokenKey))

	t := &TraceExporter{
		knapsack:         k,
		osqueryClient:    client,
		logger:           log.With(logger, "component", "trace_exporter"),
		attrs:            attrs,
		attrLock:         sync.RWMutex{},
		ingestAuthToken:  string(currentToken),
		ingestUrl:        k.ObservabilityIngestServerURL(),
		disableIngestTLS: k.DisableObservabilityIngestTLS(),
		enabled:          k.ExportTraces(),
	}

	// Observe ExportTraces and IngestServerURL changes to know when to start/stop exporting, and where
	// to export to
	t.knapsack.RegisterChangeObserver(t, keys.ExportTraces, keys.ObservabilityIngestServerURL, keys.DisableObservabilityIngestTLS)

	if !t.enabled {
		return t, nil
	}

	t.addDeviceIdentifyingAttributes()

	// Set the provider with as many resource attributes as we can get immediately
	t.setNewGlobalProvider()

	// In the background, wait for osquery to be ready so that we can fetch more resource
	// attributes for our traces, then replace the provider with a new one.
	go func() {
		t.addAttributesFromOsquery()
		t.setNewGlobalProvider()
		level.Debug(t.logger).Log("msg", "successfully replaced global provider after adding osquery attributes")
	}()

	return t, nil
}

// newExporter returns an exporter that will send traces with OTLP over gRPC.
func newExporter(ctx context.Context, token string, url string, insecure bool) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(url),
		otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(newClientAuthenticator(token))),
	}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	traceClient := otlptracegrpc.NewClient(opts...)
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

	if deviceId, err := t.knapsack.ServerProvidedDataStore().Get([]byte("device_id")); err != nil {
		level.Debug(t.logger).Log("msg", "could not get device id", "err", err)
	} else {
		t.attrs = append(t.attrs, semconv.ServiceInstanceID(string(deviceId)))
	}

	if munemo, err := t.knapsack.ServerProvidedDataStore().Get([]byte("munemo")); err != nil {
		level.Debug(t.logger).Log("msg", "could not get munemo", "err", err)
	} else {
		t.attrs = append(t.attrs, attribute.String("launcher.munemo", string(munemo)))
	}

	if orgId, err := t.knapsack.ServerProvidedDataStore().Get([]byte("organization_id")); err != nil {
		level.Debug(t.logger).Log("msg", "could not get organization id", "err", err)
	} else {
		t.attrs = append(t.attrs, attribute.String("launcher.organization_id", string(orgId)))
	}

	if serialNumber, err := t.knapsack.ServerProvidedDataStore().Get([]byte("serial_number")); err != nil {
		level.Debug(t.logger).Log("msg", "could not get serial number", "err", err)
	} else {
		t.attrs = append(t.attrs, attribute.String("launcher.serial", string(serialNumber)))
	}
}

// addAttributesFromOsquery retrieves device and OS details from osquery and adds them
// to our resource attributes. Since this is called on startup when the osquery client
// may not be ready yet, we perform a few retries.
func (t *TraceExporter) addAttributesFromOsquery() {
	t.attrLock.Lock()
	defer t.attrLock.Unlock()

	osqueryInfoQuery := `
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

	// The osqueryd client may not have initialized yet, so retry for up to three minutes on error.
	var resp []map[string]string
	if err := backoff.WaitFor(func() error {
		var err error
		resp, err = t.osqueryClient.Query(osqueryInfoQuery)
		if err != nil {
			return err
		}
		if len(resp) == 0 {
			return errors.New("no results returned")
		}
		return nil
	}, 3*time.Minute, osqueryClientRecheckInterval); err != nil {
		return
	}

	t.attrs = append(t.attrs,
		attribute.String("launcher.osquery_version", resp[0]["osquery_version"]),
		semconv.OSName(resp[0]["os_name"]),
		semconv.OSVersion(resp[0]["os_version"]),
		semconv.HostName("hostname"),
	)
}

// setNewGlobalProvider creates and sets a new global provider with the currently-available
// attributes. If a provider was previously set, it will be shut down.
func (t *TraceExporter) setNewGlobalProvider() {
	exp, err := newExporter(context.Background(), t.ingestAuthToken, t.ingestUrl, t.disableIngestTLS)
	if err != nil {
		level.Debug(t.logger).Log("msg", "could not create new exporter", "err", err)
		return
	}

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

// Execute is a no-op -- the exporter is already running in the background. The TraceExporter
// otherwise only responds to control server events.
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

// Update satisfies control.consumer interface -- handles updates to `traces` subsystem,
// which amounts to a new bearer auth token being provided
func (t *TraceExporter) Update(data io.Reader) error {
	var updatedCfg traceSubsystemConfig
	if err := json.NewDecoder(data).Decode(&updatedCfg); err != nil {
		return fmt.Errorf("failed to decode trace subsystem data: %w", err)
	}

	t.ingestAuthToken = updatedCfg.IngestToken

	if err := t.knapsack.ConfigStore().Set([]byte(configStoreTokenKey), []byte(updatedCfg.IngestToken)); err != nil {
		level.Debug(t.logger).Log("msg", "could not store trace ingest token after update", "err", err)
	}

	t.setNewGlobalProvider()

	return nil
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface -- handles updates to flags
// that we care about, which are ingest_url and export_traces.
func (t *TraceExporter) FlagsChanged(flagKeys ...keys.FlagKey) {
	needsNewProvider := false

	// Handle export_traces toggle
	if slices.Contains(flagKeys, keys.ExportTraces) {
		if !t.enabled && t.knapsack.ExportTraces() {
			// Newly enabled
			// Get any identifying attributes we may not have stored yet
			t.addDeviceIdentifyingAttributes()
			t.addAttributesFromOsquery()
			t.enabled = true
			needsNewProvider = true
			level.Debug(t.logger).Log("msg", "enabling trace export")
		} else if t.enabled && !t.knapsack.ExportTraces() {
			// Newly disabled
			if t.provider != nil {
				t.provider.Shutdown(context.Background())
			}
			t.enabled = false
			level.Debug(t.logger).Log("msg", "disabling trace export")
		}
	}

	// Handle ingest_url updates
	if slices.Contains(flagKeys, keys.ObservabilityIngestServerURL) {
		if t.ingestUrl != t.knapsack.ObservabilityIngestServerURL() {
			t.ingestUrl = t.knapsack.ObservabilityIngestServerURL()
			needsNewProvider = true
			level.Debug(t.logger).Log("msg", "updating ingest server url", "new_ingest_url", t.ingestUrl)
		}
	}

	// Handle disable_ingest_tls updates
	if slices.Contains(flagKeys, keys.DisableObservabilityIngestTLS) {
		if t.disableIngestTLS != t.knapsack.DisableObservabilityIngestTLS() {
			t.disableIngestTLS = t.knapsack.DisableObservabilityIngestTLS()
			needsNewProvider = true
			level.Debug(t.logger).Log("msg", "updating ingest server config", "new_disable_ingest_tls", t.disableIngestTLS)
		}
	}

	if !t.enabled || !needsNewProvider {
		return
	}

	t.setNewGlobalProvider()
}
