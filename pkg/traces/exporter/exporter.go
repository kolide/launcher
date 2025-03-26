package exporter

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/kolide/launcher/pkg/traces/bufspanprocessor"
	osquerygotraces "github.com/osquery/osquery-go/traces"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
)

const applicationName = "launcher"

var archAttributeMap = map[string]attribute.KeyValue{
	"amd64": semconv.HostArchAMD64,
	"386":   semconv.HostArchX86,
	"arm64": semconv.HostArchARM64,
	"arm":   semconv.HostArchARM32,
}

var enrollmentDetailsRecheckInterval = 5 * time.Second

type TraceExporter struct {
	provider                  *sdktrace.TracerProvider
	providerLock              sync.Mutex
	bufSpanProcessor          *bufspanprocessor.BufSpanProcessor
	knapsack                  types.Knapsack
	slogger                   *slog.Logger
	attrs                     []attribute.KeyValue // resource attributes, identifying this device + installation
	attrLock                  sync.RWMutex
	ingestClientAuthenticator *clientAuthenticator
	ingestAuthToken           string
	ingestUrl                 string
	disableIngestTLS          bool
	enabled                   bool
	traceSamplingRate         float64
	batchTimeout              time.Duration
	ctx                       context.Context // nolint:containedctx
	cancel                    context.CancelFunc
	interrupted               atomic.Bool
}

// NewTraceExporter sets up our traces to be exported via OTLP over HTTP.
// On interrupt, the provider will be shut down.
func NewTraceExporter(ctx context.Context, k types.Knapsack, initialTraceBuffer *InitialTraceBuffer) (*TraceExporter, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	currentToken, _ := k.TokenStore().Get(storage.ObservabilityIngestAuthTokenKey)
	ctx, cancel := context.WithCancel(ctx)

	t := &TraceExporter{
		providerLock:              sync.Mutex{},
		knapsack:                  k,
		slogger:                   k.Slogger().With("component", "trace_exporter"),
		attrLock:                  sync.RWMutex{},
		ingestClientAuthenticator: newClientAuthenticator(string(currentToken), k.DisableTraceIngestTLS()),
		ingestAuthToken:           string(currentToken),
		ingestUrl:                 k.TraceIngestServerURL(),
		disableIngestTLS:          k.DisableTraceIngestTLS(),
		enabled:                   k.ExportTraces(),
		traceSamplingRate:         k.TraceSamplingRate(),
		batchTimeout:              k.TraceBatchTimeout(),
		ctx:                       ctx,
		cancel:                    cancel,
	}

	if initialTraceBuffer != nil {
		t.provider = initialTraceBuffer.provider
		t.bufSpanProcessor = initialTraceBuffer.bufSpanProcessor
		t.attrs = initialTraceBuffer.attrs
	} else {
		t.bufSpanProcessor = &bufspanprocessor.BufSpanProcessor{
			MaxBufferedSpans: 500,
		}
		t.attrs = initialAttrs()
	}

	// Observe changes to trace configuration to know when to start/stop exporting, and when
	// to adjust exporting behavior
	t.knapsack.RegisterChangeObserver(t, keys.ExportTraces, keys.TraceSamplingRate, keys.TraceIngestServerURL, keys.DisableTraceIngestTLS, keys.TraceBatchTimeout)

	if !t.enabled {
		return t, nil
	}

	// Set our own error handler to avoid otel printing errors
	otel.SetErrorHandler(newErrorHandler(
		k.Slogger().With("component", "trace_exporter"),
	))

	t.addDeviceIdentifyingAttributes()

	// Check if enrollment details are already available, add them immediately if so
	enrollmentDetails := t.knapsack.GetEnrollmentDetails()
	if hasRequiredEnrollmentDetails(enrollmentDetails) {
		t.addAttributesFromEnrollmentDetails(enrollmentDetails)
	} else {
		// Launch a goroutine to wait for enrollment details
		gowrapper.Go(context.TODO(), t.slogger, func() {
			t.addAttributesFromOsquery()
		})
	}

	return t, nil
}

// addDeviceIdentifyingAttributes gets device identifiers from the server-provided
// data and adds them to our resource attributes.
func (t *TraceExporter) addDeviceIdentifyingAttributes() {
	t.attrLock.Lock()
	defer t.attrLock.Unlock()

	if deviceId, err := t.knapsack.ServerProvidedDataStore().Get([]byte("device_id")); err != nil {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get device id",
			"err", err,
		)
	} else {
		t.attrs = append(t.attrs, semconv.ServiceInstanceID(string(deviceId)))
		t.attrs = append(t.attrs, attribute.String("k2.device_id", string(deviceId)))
	}

	if munemo, err := t.knapsack.ServerProvidedDataStore().Get([]byte("munemo")); err != nil {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get munemo",
			"err", err,
		)
	} else {
		t.attrs = append(t.attrs, attribute.String("k2.munemo", string(munemo)))
	}

	if orgId, err := t.knapsack.ServerProvidedDataStore().Get([]byte("organization_id")); err != nil {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get organization id",
			"err", err,
		)
	} else {
		t.attrs = append(t.attrs, attribute.String("k2.organization_id", string(orgId)))
	}

	if serialNumber, err := t.knapsack.ServerProvidedDataStore().Get([]byte("serial_number")); err != nil {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get serial number",
			"err", err,
		)
	} else {
		t.attrs = append(t.attrs, attribute.String("launcher.serial", string(serialNumber)))
	}
}

// hasRequiredEnrollmentDetails checks if the provided enrollment details contain
// all the required fields for adding trace attributes
func hasRequiredEnrollmentDetails(details types.EnrollmentDetails) bool {
	// Check that all required fields have values
	return details.OsqueryVersion != "" &&
		details.OSName != "" &&
		details.OSVersion != "" &&
		details.Hostname != ""
}

// addAttributesFromOsquery waits for enrollment details to be available
// and then adds the relevant attributes
func (t *TraceExporter) addAttributesFromOsquery() {
	// Wait until enrollment details are available
	retryTimeout := time.Now().Add(3 * time.Minute)
	for {
		if time.Now().After(retryTimeout) {
			t.slogger.Log(context.TODO(), slog.LevelWarn,
				"could not get enrollment details before timeout",
			)
			return
		}

		enrollmentDetails := t.knapsack.GetEnrollmentDetails()
		if hasRequiredEnrollmentDetails(enrollmentDetails) {
			t.addAttributesFromEnrollmentDetails(enrollmentDetails)
			return
		}

		select {
		case <-t.ctx.Done():
			t.slogger.Log(context.TODO(), slog.LevelDebug,
				"trace exporter interrupted while waiting for enrollment details",
			)
			return
		case <-time.After(enrollmentDetailsRecheckInterval):
			continue
		}
	}
}

func (t *TraceExporter) addAttributesFromEnrollmentDetails(details types.EnrollmentDetails) {
	t.attrLock.Lock()
	defer t.attrLock.Unlock()

	// Add OS and system attributes from enrollment details
	t.attrs = append(t.attrs,
		attribute.String("launcher.osquery_version", details.OsqueryVersion),
		semconv.OSName(details.OSName),
		semconv.OSVersion(details.OSVersion),
		semconv.HostName(details.Hostname),
	)

	t.slogger.Log(context.TODO(), slog.LevelDebug,
		"added attributes from enrollment details",
	)
}

// setNewGlobalProvider creates and sets a new global provider with the currently-available
// attributes. If a provider was previously set, it will be shut down.
func (t *TraceExporter) setNewGlobalProvider(rebuildExporter bool) {
	t.providerLock.Lock()
	defer t.providerLock.Unlock()

	t.attrLock.RLock()
	defer t.attrLock.RUnlock()

	defaultResource := resource.Default()
	r, err := resource.Merge(
		defaultResource,
		resource.NewWithAttributes(defaultResource.SchemaURL(), t.attrs...),
	)
	if err != nil {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not merge resource",
			"err", err,
		)
		r = resource.Default()
	}

	// Sample root spans based on t.traceSamplingRate, then sample child spans based on the
	// decision made for their parent: if parent is sampled, then children should be as well;
	// otherwise, do not sample child spans.
	parentBasedSampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(t.traceSamplingRate))

	newProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(t.bufSpanProcessor),
		sdktrace.WithResource(r),
		sdktrace.WithSampler(parentBasedSampler),
	)

	otel.SetTracerProvider(newProvider)
	osquerygotraces.SetTracerProvider(newProvider)

	if t.provider != nil {
		// shutdown still gets called even though the span processor is unregistered
		// leaving this in because it just feel correct
		t.provider.UnregisterSpanProcessor(t.bufSpanProcessor)
		t.provider.Shutdown(t.ctx)
	}

	t.provider = newProvider

	if !rebuildExporter {
		return
	}

	// create a trace client with the new ingest url
	traceClientOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(t.knapsack.TraceIngestServerURL()),
		otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(t.ingestClientAuthenticator)),
	}
	if t.disableIngestTLS {
		traceClientOpts = append(traceClientOpts, otlptracegrpc.WithInsecure())
	}

	traceClient := otlptracegrpc.NewClient(traceClientOpts...)

	// create exporter with new trace client
	exporter, err := otlptrace.New(t.ctx, traceClient)
	if err != nil {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not create new exporter",
			"err", err,
		)
		return
	}

	// create child processor with new exporter and set it on the bufspanprocessor
	batchSpanProcessor := sdktrace.NewBatchSpanProcessor(exporter, sdktrace.WithBatchTimeout(t.batchTimeout))
	t.bufSpanProcessor.SetChildProcessor(batchSpanProcessor)

	// set ingest url after successfully setting up new child processor
	t.ingestUrl = t.knapsack.TraceIngestServerURL()
}

// Execute begins exporting traces, if exporting is enabled.
func (t *TraceExporter) Execute() error {
	if t.enabled {
		t.setNewGlobalProvider(true)
		t.slogger.Log(context.TODO(), slog.LevelDebug,
			"successfully replaced global provider after adding more attributes",
		)
	}

	<-t.ctx.Done()
	return nil
}

func (t *TraceExporter) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if t.interrupted.Load() {
		return
	}

	t.interrupted.Store(true)

	if t.provider != nil {
		t.provider.Shutdown(t.ctx)
	}

	t.cancel()
}

// Update satisfies control.subscriber interface -- looks at changes to the `observability_ingest` subsystem,
// which amounts to a new bearer auth token being provided.
func (t *TraceExporter) Ping() {
	newToken, err := t.knapsack.TokenStore().Get(storage.ObservabilityIngestAuthTokenKey)
	if err != nil || len(newToken) == 0 {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get new token from token store",
			"err", err,
		)
		return
	}

	// No need to replace the entire global provider on token update -- we can swap
	// to the new token in place.
	t.ingestAuthToken = string(newToken)
	t.ingestClientAuthenticator.setToken(t.ingestAuthToken)
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface -- handles updates to flags
// that we care about, which are ingest_url and export_traces.
func (t *TraceExporter) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

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
			t.slogger.Log(ctx, slog.LevelDebug,
				"enabling trace export",
			)
		} else if t.enabled && !t.knapsack.ExportTraces() {
			// Newly disabled
			if t.provider != nil {
				t.provider.Shutdown(t.ctx)
			}
			t.enabled = false
			t.slogger.Log(ctx, slog.LevelDebug,
				"disabling trace export",
			)
		}
	}

	// Handle trace_sampling_rate updates
	if slices.Contains(flagKeys, keys.TraceSamplingRate) {
		if t.traceSamplingRate != t.knapsack.TraceSamplingRate() {
			t.traceSamplingRate = t.knapsack.TraceSamplingRate()
			needsNewProvider = true
			t.slogger.Log(ctx, slog.LevelDebug,
				"updating trace sampling rate",
				"new_sampling_rate", t.traceSamplingRate,
			)
		}
	}

	// Handle ingest_url updates
	if slices.Contains(flagKeys, keys.TraceIngestServerURL) {
		if t.ingestUrl != t.knapsack.TraceIngestServerURL() {
			t.ingestUrl = t.knapsack.TraceIngestServerURL()
			needsNewProvider = true
			t.slogger.Log(ctx, slog.LevelDebug,
				"updating ingest server url",
				"new_ingest_url", t.ingestUrl,
			)
		}
	}

	// Handle disable_trace_ingest_tls updates
	if slices.Contains(flagKeys, keys.DisableTraceIngestTLS) {
		if t.disableIngestTLS != t.knapsack.DisableTraceIngestTLS() {
			t.ingestClientAuthenticator.setDisableTLS(t.knapsack.DisableTraceIngestTLS())
			t.disableIngestTLS = t.knapsack.DisableTraceIngestTLS()
			needsNewProvider = true
			t.slogger.Log(ctx, slog.LevelDebug,
				"updating ingest server config",
				"new_disable_trace_ingest_tls", t.disableIngestTLS,
			)
		}
	}

	// Handle trace_batch_timeout updates
	if slices.Contains(flagKeys, keys.TraceBatchTimeout) {
		if t.batchTimeout != t.knapsack.TraceBatchTimeout() {
			t.batchTimeout = t.knapsack.TraceBatchTimeout()
			needsNewProvider = true
			t.slogger.Log(ctx, slog.LevelDebug,
				"updating trace batch timeout",
				"new_batch_timeout", t.batchTimeout,
			)
		}
	}

	if !t.enabled || !needsNewProvider {
		return
	}

	t.setNewGlobalProvider(true)
}
