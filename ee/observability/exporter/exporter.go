package exporter

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/observability/bufspanprocessor"
	osquerygotraces "github.com/osquery/osquery-go/traces"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
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

type TelemetryExporter struct {
	tracerProvider            *sdktrace.TracerProvider
	meterProvider             *sdkmetric.MeterProvider
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

// NewTelemetryExporter sets up our telemetry (traces and soon metrics) to be exported via OTLP over HTTP.
// On interrupt, the provider will be shut down.
func NewTelemetryExporter(ctx context.Context, k types.Knapsack, initialTraceBuffer *InitialTraceBuffer) (*TelemetryExporter, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	currentToken, _ := k.TokenStore().Get(storage.ObservabilityIngestAuthTokenKey)
	ctx, cancel := context.WithCancel(ctx)

	t := &TelemetryExporter{
		providerLock:              sync.Mutex{},
		knapsack:                  k,
		slogger:                   k.Slogger().With("component", "telemetry_exporter"),
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
		t.tracerProvider = initialTraceBuffer.provider
		t.bufSpanProcessor = initialTraceBuffer.bufSpanProcessor
		t.attrs = initialTraceBuffer.attrs
	} else {
		t.bufSpanProcessor = bufspanprocessor.NewBufSpanProcessor(500)
		t.bufSpanProcessor.SetSlogger(k.Slogger())
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
		k.Slogger().With("component", "telemetry_exporter"),
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
func (t *TelemetryExporter) addDeviceIdentifyingAttributes() {
	t.attrLock.Lock()
	defer t.attrLock.Unlock()

	if deviceId, err := t.knapsack.ServerProvidedDataStore().Get([]byte("device_id")); err != nil {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get device id for attributes",
			"err", err,
		)
	} else {
		t.attrs = append(t.attrs, semconv.ServiceInstanceID(string(deviceId)))
		t.attrs = append(t.attrs, attribute.String("k2.device_id", string(deviceId)))
	}

	if munemo, err := t.knapsack.ServerProvidedDataStore().Get([]byte("munemo")); err != nil {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get munemo for attributes",
			"err", err,
		)
	} else {
		t.attrs = append(t.attrs, attribute.String("k2.munemo", string(munemo)))
	}

	if orgId, err := t.knapsack.ServerProvidedDataStore().Get([]byte("organization_id")); err != nil {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get organization id for attributes",
			"err", err,
		)
	} else {
		t.attrs = append(t.attrs, attribute.String("k2.organization_id", string(orgId)))
	}

	// Get serial number from enrollment details
	enrollmentDetails := t.knapsack.GetEnrollmentDetails()
	if enrollmentDetails.HardwareSerial != "" {
		t.attrs = append(t.attrs, attribute.String("launcher.serial", enrollmentDetails.HardwareSerial))
	} else {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get serial number from enrollment details",
		)
	}

	t.attrs = append(t.attrs, attribute.String("launcher.update_channel", t.knapsack.UpdateChannel()))

	// Add some attributes about the currently-running process, too
	t.attrs = append(t.attrs, attribute.String("launcher.run_id", t.knapsack.GetRunID()))
	t.attrs = append(t.attrs, semconv.ProcessPID(os.Getpid()))
	if execPath, err := os.Executable(); err != nil {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get executable path for attributes",
			"err", err,
		)
	} else {
		t.attrs = append(t.attrs, semconv.ProcessExecutablePath(execPath))
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
func (t *TelemetryExporter) addAttributesFromOsquery() {
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

func (t *TelemetryExporter) addAttributesFromEnrollmentDetails(details types.EnrollmentDetails) {
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

// setNewGlobalProvider creates and sets new global providers with the currently-available
// attributes. If providers were previously set, they will be shut down.
func (t *TelemetryExporter) setNewGlobalProvider(rebuildExporter bool) {
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

	t.setNewGlobalTracerProvider(r, rebuildExporter)
	t.setNewGlobalMeterProvider(r)

	// set ingest url after successfully setting up new child processor
	t.ingestUrl = t.knapsack.TraceIngestServerURL()
}

// setNewGlobalTracerProvider updates the global tracer provider:
// * It sets the latest launcher attributes on the resource, so that those details will be exported with the traces
// * If `rebuildExporter` is true, it re-creates the trace exporter with the latest ingest server URL and authentication
func (t *TelemetryExporter) setNewGlobalTracerProvider(launcherResource *resource.Resource, rebuildExporter bool) {
	t.providerLock.Lock()
	defer t.providerLock.Unlock()

	// Sample root spans based on t.traceSamplingRate, then sample child spans based on the
	// decision made for their parent: if parent is sampled, then children should be as well;
	// otherwise, do not sample child spans.
	parentBasedSampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(t.traceSamplingRate))

	newProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(t.bufSpanProcessor),
		sdktrace.WithResource(launcherResource),
		sdktrace.WithSampler(parentBasedSampler),
	)

	otel.SetTracerProvider(newProvider)
	osquerygotraces.SetTracerProvider(newProvider)

	if t.tracerProvider != nil {
		// shutdown still gets called even though the span processor is unregistered
		// leaving this in because it just feel correct
		t.tracerProvider.UnregisterSpanProcessor(t.bufSpanProcessor)
		if err := t.tracerProvider.Shutdown(t.ctx); err != nil {
			t.slogger.Log(t.ctx, slog.LevelWarn,
				"could not shut down old tracer provider to replace it",
				"err", err,
			)
		}
	}

	t.tracerProvider = newProvider

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
}

// setNewGlobalMeterProvider updates the global meter provider:
// * It creates a metrics exporter to ship the metrics with the latest ingest server URL and authentication
// * It sets the latest launcher attributes on the resource, so that those details will be exported with the metrics
// (Unlike with setNewGlobalTracerProvider, we always have to create a new exporter here.)
func (t *TelemetryExporter) setNewGlobalMeterProvider(launcherResource *resource.Resource) {
	t.providerLock.Lock()
	defer t.providerLock.Unlock()

	traceClientOpts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(t.knapsack.TraceIngestServerURL()),
		otlpmetricgrpc.WithDialOption(grpc.WithPerRPCCredentials(t.ingestClientAuthenticator)),
	}
	if t.disableIngestTLS {
		traceClientOpts = append(traceClientOpts, otlpmetricgrpc.WithInsecure())
	}

	metricsExporter, err := otlpmetricgrpc.New(context.TODO(), traceClientOpts...)
	if err != nil {
		t.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not create metrics gRPC exporter",
			"err", err,
		)
		return
	}

	// Create new meter provider and let otel set it globally
	newMeterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(launcherResource),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricsExporter, sdkmetric.WithInterval(15*time.Minute))),
	)
	otel.SetMeterProvider(newMeterProvider)

	// Shut down and replace old meter provider with new one
	if t.meterProvider != nil {
		if err := t.meterProvider.Shutdown(context.TODO()); err != nil {
			t.slogger.Log(context.TODO(), slog.LevelWarn,
				"could not shut down old meter provider to replace it",
				"err", err,
			)
		}
	}
	t.meterProvider = newMeterProvider

	observability.ReinitializeMetrics()
}

// Execute begins exporting telemetry if exporting is enabled.
func (t *TelemetryExporter) Execute() error {
	if t.enabled {
		t.setNewGlobalProvider(true)
		t.slogger.Log(context.TODO(), slog.LevelDebug,
			"successfully replaced global provider after adding more attributes",
		)
	}

	<-t.ctx.Done()
	return nil
}

func (t *TelemetryExporter) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if t.interrupted.Swap(true) {
		return
	}

	// We must use context.Background here, not t.ctx -- if we use t.ctx, the restart metric won't ship,
	// and calls to Shutdown will time out.
	interruptCtx, interruptCancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer interruptCancel()

	// Record launcher shutdown -- we do this here (rather than in e.g. rungroup shutdown) to ensure
	// we will write this metric before we complete `Interrupt`
	observability.LauncherRestartCounter.Add(interruptCtx, 1)
	time.Sleep(1 * time.Second)

	if t.tracerProvider != nil {
		if err := t.tracerProvider.Shutdown(interruptCtx); err != nil {
			t.slogger.Log(interruptCtx, slog.LevelWarn,
				"could not shut down tracer provider on interrupt",
				"err", err,
			)
		}
	}

	if t.meterProvider != nil {
		if err := t.meterProvider.Shutdown(interruptCtx); err != nil {
			t.slogger.Log(interruptCtx, slog.LevelWarn,
				"could not shut down meter provider on interrupt",
				"err", err,
			)
		}
	}

	t.cancel()
}

// Update satisfies control.subscriber interface -- looks at changes to the `observability_ingest` subsystem,
// which amounts to a new bearer auth token being provided.
func (t *TelemetryExporter) Ping() {
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
func (t *TelemetryExporter) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	ctx, span := observability.StartSpan(ctx)
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
			if t.tracerProvider != nil {
				if err := t.tracerProvider.Shutdown(context.TODO()); err != nil {
					t.slogger.Log(ctx, slog.LevelWarn,
						"could not shut down tracer provider on trace disable",
						"err", err,
					)
				}
			}
			if t.meterProvider != nil {
				if err := t.meterProvider.Shutdown(context.TODO()); err != nil {
					t.slogger.Log(ctx, slog.LevelWarn,
						"could not shut down meter provider on trace disable",
						"err", err,
					)
				}
			}
			t.enabled = false
			t.slogger.Log(ctx, slog.LevelDebug,
				"disabling telemetry export",
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
