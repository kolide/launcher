package exporter

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/observability/bufspanprocessor"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

// NB - Tests that result in calls to `setNewGlobalProvider` should not be run in parallel
// to avoid race condition complaints.

func TestNewTelemetryExporter(t *testing.T) { //nolint:paralleltest
	mockKnapsack := typesmocks.NewKnapsack(t)

	tokenStore := testTokenStore(t)
	mockKnapsack.On("TokenStore").Return(tokenStore)
	tokenStore.Set(storage.ObservabilityIngestAuthTokenKey, []byte("test token"))

	serverProvidedDataStore := testServerProvidedDataStore(t)
	mockKnapsack.On("ServerProvidedDataStore").Return(serverProvidedDataStore)
	serverProvidedDataStore.Set([]byte("device_id"), []byte("500"))
	serverProvidedDataStore.Set([]byte("munemo"), []byte("nababe"))
	serverProvidedDataStore.Set([]byte("organization_id"), []byte("101"))

	mockKnapsack.On("TraceIngestServerURL").Return("localhost:3417")
	mockKnapsack.On("DisableTraceIngestTLS").Return(false)
	mockKnapsack.On("ExportTraces").Return(true)
	mockKnapsack.On("TraceSamplingRate").Return(1.0)
	mockKnapsack.On("TraceBatchTimeout").Return(1 * time.Minute)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ExportTraces, keys.TraceSamplingRate, keys.TraceIngestServerURL, keys.DisableTraceIngestTLS, keys.TraceBatchTimeout).Return(nil)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("UpdateChannel").Return("nightly").Maybe()
	mockKnapsack.On("GetRunID").Return(ulid.New()).Maybe()
	mockKnapsack.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{
		OsqueryVersion: "5.8.0",
		OSName:         runtime.GOOS,
		OSVersion:      "3.4.5",
		Hostname:       "Test-Hostname2",
		HardwareSerial: "abcdabcd",
	})

	telemetryExporter, err := NewTelemetryExporter(t.Context(), mockKnapsack, NewInitialTraceBuffer())
	require.NoError(t, err)

	// Wait a few seconds to allow the osquery queries to go through
	time.Sleep(500 * time.Millisecond)

	// We expect a total of 13 attributes: 4 initial attributes, 5 from the ServerProvidedDataStore, and 4 from osquery
	telemetryExporter.attrLock.RLock()
	defer telemetryExporter.attrLock.RUnlock()
	require.Equal(t, 17, len(telemetryExporter.attrs))

	// Confirm we set a provider
	telemetryExporter.providerLock.Lock()
	defer telemetryExporter.providerLock.Unlock()
	require.NotNil(t, telemetryExporter.tracerProvider, "expected tracer provider to be created")

	mockKnapsack.AssertExpectations(t)
}

func TestNewTelemetryExporter_exportNotEnabled(t *testing.T) {
	t.Parallel()

	tokenStore := testTokenStore(t)
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("TokenStore").Return(tokenStore)
	tokenStore.Set(storage.ObservabilityIngestAuthTokenKey, []byte("test token"))
	mockKnapsack.On("TraceIngestServerURL").Return("localhost:3417")
	mockKnapsack.On("DisableTraceIngestTLS").Return(false)
	mockKnapsack.On("ExportTraces").Return(false)
	mockKnapsack.On("TraceSamplingRate").Return(0.0)
	mockKnapsack.On("TraceBatchTimeout").Return(1 * time.Minute)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ExportTraces, keys.TraceSamplingRate, keys.TraceIngestServerURL, keys.DisableTraceIngestTLS, keys.TraceBatchTimeout).Return(nil)
	mockKnapsack.On("UpdateChannel").Return("alpha").Maybe()
	mockKnapsack.On("GetRunID").Return(ulid.New()).Maybe()
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

	telemetryExporter, err := NewTelemetryExporter(t.Context(), mockKnapsack, nil)
	require.NoError(t, err)

	// Confirm we didn't set a provider
	require.Nil(t, telemetryExporter.tracerProvider, "expected disabled exporter to not create a tracer provider but one was created")
	require.Nil(t, telemetryExporter.meterProvider, "expected disabled exporter to not create a meter provider but one was created")

	// Confirm we added basic attributes
	require.Equal(t, 4, len(telemetryExporter.attrs))
	for _, actualAttr := range telemetryExporter.attrs {
		switch actualAttr.Key {
		case "service.name":
			require.Equal(t, applicationName, actualAttr.Value.AsString())
		case "service.version":
			require.Equal(t, version.Version().Version, actualAttr.Value.AsString())
		case "host.arch":
			require.Equal(t, runtime.GOARCH, actualAttr.Value.AsString())
		case "launcher.goos":
			require.Equal(t, runtime.GOOS, actualAttr.Value.AsString())
		default:
			t.Fatalf("unexpected attr %s", actualAttr.Key)
		}
	}

	mockKnapsack.AssertExpectations(t)
}

func TestInterrupt_Multiple(t *testing.T) { //nolint:paralleltest
	tokenStore := testTokenStore(t)
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("TokenStore").Return(tokenStore)
	tokenStore.Set(storage.ObservabilityIngestAuthTokenKey, []byte("test token"))
	mockKnapsack.On("TraceIngestServerURL").Return("localhost:3417")
	mockKnapsack.On("DisableTraceIngestTLS").Return(false)
	mockKnapsack.On("ExportTraces").Return(false)
	mockKnapsack.On("TraceSamplingRate").Return(0.0)
	mockKnapsack.On("TraceBatchTimeout").Return(1 * time.Minute)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ExportTraces, keys.TraceSamplingRate, keys.TraceIngestServerURL, keys.DisableTraceIngestTLS, keys.TraceBatchTimeout).Return(nil)
	mockKnapsack.On("UpdateChannel").Return("beta").Maybe()
	mockKnapsack.On("GetRunID").Return(ulid.New()).Maybe()
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack.On("Slogger").Return(slogger)

	telemetryExporter, err := NewTelemetryExporter(t.Context(), mockKnapsack, NewInitialTraceBuffer())
	require.NoError(t, err)
	mockKnapsack.AssertExpectations(t)

	go telemetryExporter.Execute()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
	telemetryExporter.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			telemetryExporter.Interrupt(nil)
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for {
		if receivedInterrupts >= expectedInterrupts {
			break
		}

		select {
		case <-interruptComplete:
			receivedInterrupts += 1
			continue
		case <-time.After(5 * time.Second):
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- interrupted at %s, received %d interrupts before timeout; logs: \n%s\n", interruptStart.String(), receivedInterrupts, logBytes.String())
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}

func Test_addDeviceIdentifyingAttributes(t *testing.T) {
	t.Parallel()

	s := testServerProvidedDataStore(t)
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("ServerProvidedDataStore").Return(s)

	// Set expected data
	expectedDeviceId := "123"
	s.Set([]byte("device_id"), []byte(expectedDeviceId))

	expectedMunemo := "nababe"
	s.Set([]byte("munemo"), []byte(expectedMunemo))

	expectedOrganizationId := "100"
	s.Set([]byte("organization_id"), []byte(expectedOrganizationId))

	expectedSerialNumber := "abcd"
	mockKnapsack.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{
		HardwareSerial: expectedSerialNumber,
	})

	expectedUpdateChannel := "stable"
	mockKnapsack.On("UpdateChannel").Return(expectedUpdateChannel)

	expectedRunId := ulid.New()
	mockKnapsack.On("GetRunID").Return(expectedRunId)

	traceExporter := &TelemetryExporter{
		knapsack:                  mockKnapsack,
		slogger:                   multislogger.NewNopLogger(),
		attrs:                     make([]attribute.KeyValue, 0),
		attrLock:                  sync.RWMutex{},
		ingestClientAuthenticator: newClientAuthenticator("test token", false),
		ingestAuthToken:           "test token",
		ingestUrl:                 "localhost:4317",
		disableIngestTLS:          false,
		enabled:                   true,
		traceSamplingRate:         1.0,
	}

	traceExporter.addDeviceIdentifyingAttributes()

	// Confirm all expected attributes were added
	require.Equal(t, 9, len(traceExporter.attrs))
	for _, actualAttr := range traceExporter.attrs {
		switch actualAttr.Key {
		case "service.instance.id", "k2.device_id":
			require.Equal(t, expectedDeviceId, actualAttr.Value.AsString())
		case "k2.munemo":
			require.Equal(t, expectedMunemo, actualAttr.Value.AsString())
		case "k2.organization_id":
			require.Equal(t, expectedOrganizationId, actualAttr.Value.AsString())
		case "launcher.serial":
			require.Equal(t, expectedSerialNumber, actualAttr.Value.AsString())
		case "launcher.update_channel":
			require.Equal(t, expectedUpdateChannel, actualAttr.Value.AsString())
		case "launcher.run_id":
			require.Equal(t, expectedRunId, actualAttr.Value.AsString())
		case "process.pid", "process.executable.path":
			// Not going to validate attr values, but we do expect to have them
		default:
			t.Fatalf("unexpected attr %s", actualAttr.Key)
		}
	}

	mockKnapsack.AssertExpectations(t)
}

func Test_addAttributesFromOsquery(t *testing.T) {
	t.Parallel()

	expectedOsqueryVersion := "5.7.1"
	expectedOsName := runtime.GOOS
	expectedOsVersion := "1.2.3"
	expectedHostname := "Test-Hostname"

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{
		OsqueryVersion: expectedOsqueryVersion,
		OSName:         expectedOsName,
		OSVersion:      expectedOsVersion,
		Hostname:       expectedHostname,
	})

	traceExporter := &TelemetryExporter{
		knapsack:                  mockKnapsack,
		slogger:                   multislogger.NewNopLogger(),
		attrs:                     make([]attribute.KeyValue, 0),
		attrLock:                  sync.RWMutex{},
		ingestClientAuthenticator: newClientAuthenticator("test token", false),
		ingestAuthToken:           "test token",
		ingestUrl:                 "localhost:4317",
		disableIngestTLS:          false,
		enabled:                   true,
		traceSamplingRate:         1.0,
	}

	traceExporter.addAttributesFromOsquery()

	// Confirm all expected attributes were added
	require.Equal(t, 4, len(traceExporter.attrs))
	for _, actualAttr := range traceExporter.attrs {
		switch actualAttr.Key {
		case "launcher.osquery_version":
			require.Equal(t, expectedOsqueryVersion, actualAttr.Value.AsString())
		case "os.name":
			require.Equal(t, expectedOsName, actualAttr.Value.AsString())
		case "os.version":
			require.Equal(t, expectedOsVersion, actualAttr.Value.AsString())
		case "host.name":
			require.Equal(t, expectedHostname, actualAttr.Value.AsString())
		default:
			t.Fatalf("unexpected attr %s", actualAttr.Key)
		}
	}

}

func TestPing(t *testing.T) {
	t.Parallel()

	// Set up the client authenticator + exporter with an initial token
	initialTestToken := "test token A"
	clientAuthenticator := newClientAuthenticator(initialTestToken, false)

	s := testTokenStore(t)
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("TokenStore").Return(s)
	mockKnapsack.On("UpdateChannel").Return("nightly").Maybe()
	mockKnapsack.On("GetRunID").Return(ulid.New()).Maybe()

	traceExporter := &TelemetryExporter{
		knapsack:                  mockKnapsack,
		bufSpanProcessor:          bufspanprocessor.NewBufSpanProcessor(500),
		slogger:                   multislogger.NewNopLogger(),
		attrs:                     make([]attribute.KeyValue, 0),
		attrLock:                  sync.RWMutex{},
		ingestClientAuthenticator: clientAuthenticator,
		ingestAuthToken:           "test token",
		ingestUrl:                 "localhost:4317",
		disableIngestTLS:          false,
		traceSamplingRate:         1.0,
		ctx:                       t.Context(),
	}

	// Simulate a new token being set by updating the data store
	newToken := "test token B"
	require.NoError(t, s.Set(storage.ObservabilityIngestAuthTokenKey, []byte(newToken)))

	// Alert the exporter that the token has changed
	traceExporter.Ping()

	// Confirm that token has changed for exporter
	require.Equal(t, newToken, traceExporter.ingestAuthToken)

	// Confirm that the token was replaced in the client authenticator
	require.Equal(t, newToken, clientAuthenticator.token)

	mockKnapsack.AssertExpectations(t)
}

func TestFlagsChanged_ExportTraces(t *testing.T) { //nolint:paralleltest
	tests := []struct {
		testName              string
		currentEnableValue    bool
		newEnableValue        bool
		shouldReplaceProvider bool
	}{
		{
			testName:              "disable => enable",
			currentEnableValue:    false,
			newEnableValue:        true,
			shouldReplaceProvider: true,
		},
		{
			testName:              "enable => disable",
			currentEnableValue:    true,
			newEnableValue:        false,
			shouldReplaceProvider: false,
		},
		{
			testName:              "disable => disable",
			currentEnableValue:    false,
			newEnableValue:        false,
			shouldReplaceProvider: false,
		},
		{
			testName:              "enable => enable",
			currentEnableValue:    true,
			newEnableValue:        true,
			shouldReplaceProvider: false,
		},
	}

	for _, tt := range tests { //nolint:paralleltest
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			s := testServerProvidedDataStore(t)
			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("ExportTraces").Return(tt.newEnableValue)
			mockKnapsack.On("UpdateChannel").Return("nightly").Maybe()
			mockKnapsack.On("GetRunID").Return(ulid.New()).Maybe()

			if tt.shouldReplaceProvider {
				mockKnapsack.On("TraceIngestServerURL").Return("https://example.com")
			}

			if tt.shouldReplaceProvider {
				mockKnapsack.On("ServerProvidedDataStore").Return(s)
				mockKnapsack.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{
					OsqueryVersion: "5.8.0",
					OSName:         "Windows",
					OSVersion:      "11",
					Hostname:       "Test device",
				})
			}

			ctx, cancel := context.WithCancel(t.Context())
			traceExporter := &TelemetryExporter{
				knapsack:                  mockKnapsack,
				bufSpanProcessor:          bufspanprocessor.NewBufSpanProcessor(500),
				slogger:                   multislogger.NewNopLogger(),
				attrs:                     make([]attribute.KeyValue, 0),
				attrLock:                  sync.RWMutex{},
				ingestClientAuthenticator: newClientAuthenticator("test token", false),
				ingestAuthToken:           "test token",
				ingestUrl:                 "localhost:4317",
				disableIngestTLS:          false,
				enabled:                   tt.currentEnableValue,
				traceSamplingRate:         1.0,
				ctx:                       ctx,
				cancel:                    cancel,
			}

			traceExporter.FlagsChanged(ctx, keys.ExportTraces)

			require.Equal(t, tt.newEnableValue, traceExporter.enabled, "enabled value not updated")

			if tt.shouldReplaceProvider {
				mockKnapsack.AssertExpectations(t)
				require.Greater(t, len(traceExporter.attrs), 0)
				require.NotNil(t, traceExporter.tracerProvider)
				require.NotNil(t, traceExporter.meterProvider)
			}
		})
	}
}

func TestFlagsChanged_TraceSamplingRate(t *testing.T) { //nolint:paralleltest
	tests := []struct {
		testName                 string
		currentTraceSamplingRate float64
		newTraceSamplingRate     float64
		tracingEnabled           bool
		shouldReplaceProvider    bool
	}{
		{
			testName:                 "update",
			currentTraceSamplingRate: 1.0,
			newTraceSamplingRate:     0.5,
			tracingEnabled:           true,
			shouldReplaceProvider:    true,
		},
		{
			testName:                 "update but tracing not enabled",
			currentTraceSamplingRate: 1.0,
			newTraceSamplingRate:     0.5,
			tracingEnabled:           false,
			shouldReplaceProvider:    false,
		},
		{
			testName:                 "no update",
			currentTraceSamplingRate: 0.0,
			newTraceSamplingRate:     0.0,
			tracingEnabled:           true,
			shouldReplaceProvider:    false,
		},
	}

	for _, tt := range tests { //nolint:paralleltest
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("TraceSamplingRate").Return(tt.newTraceSamplingRate)
			mockKnapsack.On("UpdateChannel").Return("nightly").Maybe()
			mockKnapsack.On("GetRunID").Return(ulid.New()).Maybe()

			if tt.shouldReplaceProvider {
				mockKnapsack.On("TraceIngestServerURL").Return("https://example.com")
			}

			ctx, cancel := context.WithCancel(t.Context())
			traceExporter := &TelemetryExporter{
				knapsack:                  mockKnapsack,
				bufSpanProcessor:          bufspanprocessor.NewBufSpanProcessor(500),
				slogger:                   multislogger.NewNopLogger(),
				attrs:                     make([]attribute.KeyValue, 0),
				attrLock:                  sync.RWMutex{},
				ingestClientAuthenticator: newClientAuthenticator("test token", false),
				ingestAuthToken:           "test token",
				ingestUrl:                 "localhost:4317",
				disableIngestTLS:          false,
				enabled:                   tt.tracingEnabled,
				traceSamplingRate:         tt.currentTraceSamplingRate,
				ctx:                       ctx,
				cancel:                    cancel,
			}

			traceExporter.FlagsChanged(ctx, keys.TraceSamplingRate)

			require.Equal(t, tt.newTraceSamplingRate, traceExporter.traceSamplingRate, "trace sampling rate value not updated")

			if tt.shouldReplaceProvider {
				require.NotNil(t, traceExporter.tracerProvider)
				require.NotNil(t, traceExporter.meterProvider)
			} else {
				require.Nil(t, traceExporter.tracerProvider)
				require.Nil(t, traceExporter.meterProvider)
			}
		})
	}
}

func TestFlagsChanged_TraceIngestServerURL(t *testing.T) { //nolint:paralleltest
	tests := []struct {
		testName                        string
		currentTraceIngestServerURL     string
		newObservabilityIngestServerURL string
		tracingEnabled                  bool
		shouldReplaceProvider           bool
	}{
		{
			testName:                        "update ingest URL",
			currentTraceIngestServerURL:     "localhost:3417",
			newObservabilityIngestServerURL: "localhost:3418",
			tracingEnabled:                  true,
			shouldReplaceProvider:           true,
		},
		{
			testName:                    "update ingest URL, but tracing not enabled",
			currentTraceIngestServerURL: "localhost:3417",
			// new ingets url won't get set until we replace provider
			newObservabilityIngestServerURL: "localhost:3417",
			tracingEnabled:                  false,
			shouldReplaceProvider:           false,
		},
		{
			testName:                        "no update to ingest URL",
			currentTraceIngestServerURL:     "localhost:3417",
			newObservabilityIngestServerURL: "localhost:3417",
			tracingEnabled:                  true,
			shouldReplaceProvider:           false,
		},
	}

	for _, tt := range tests { //nolint:paralleltest
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("TraceIngestServerURL").Return(tt.newObservabilityIngestServerURL)
			mockKnapsack.On("UpdateChannel").Return("nightly").Maybe()
			mockKnapsack.On("GetRunID").Return(ulid.New()).Maybe()

			ctx, cancel := context.WithCancel(t.Context())
			traceExporter := &TelemetryExporter{
				knapsack:                  mockKnapsack,
				bufSpanProcessor:          bufspanprocessor.NewBufSpanProcessor(500),
				slogger:                   multislogger.NewNopLogger(),
				attrs:                     make([]attribute.KeyValue, 0),
				attrLock:                  sync.RWMutex{},
				ingestClientAuthenticator: newClientAuthenticator("test token", false),
				ingestAuthToken:           "test token",
				ingestUrl:                 tt.currentTraceIngestServerURL,
				disableIngestTLS:          false,
				enabled:                   tt.tracingEnabled,
				traceSamplingRate:         1.0,
				ctx:                       ctx,
				cancel:                    cancel,
			}

			traceExporter.FlagsChanged(ctx, keys.TraceIngestServerURL)

			require.Equal(t, tt.newObservabilityIngestServerURL, traceExporter.ingestUrl, "ingest url value not updated")

			if tt.shouldReplaceProvider {
				require.NotNil(t, traceExporter.tracerProvider)
				require.NotNil(t, traceExporter.meterProvider)
			} else {
				require.Nil(t, traceExporter.tracerProvider)
				require.Nil(t, traceExporter.meterProvider)
			}
		})
	}
}

func TestFlagsChanged_DisableTraceIngestTLS(t *testing.T) { //nolint:paralleltest
	tests := []struct {
		testName                     string
		currentDisableTraceIngestTLS bool
		newDisableTraceIngestTLS     bool
		tracingEnabled               bool
		shouldReplaceProvider        bool
	}{
		{
			testName:                     "update ingest TLS value",
			currentDisableTraceIngestTLS: true,
			newDisableTraceIngestTLS:     false,
			tracingEnabled:               true,
			shouldReplaceProvider:        true,
		},
		{
			testName:                     "update ingest TLS value, but tracing not enabled",
			currentDisableTraceIngestTLS: true,
			newDisableTraceIngestTLS:     false,
			tracingEnabled:               false,
			shouldReplaceProvider:        false,
		},
		{
			testName:                     "no update to ingest TLS value",
			currentDisableTraceIngestTLS: false,
			newDisableTraceIngestTLS:     false,
			tracingEnabled:               true,
			shouldReplaceProvider:        false,
		},
	}

	for _, tt := range tests { //nolint:paralleltest
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("DisableTraceIngestTLS").Return(tt.newDisableTraceIngestTLS)
			mockKnapsack.On("UpdateChannel").Return("nightly").Maybe()
			mockKnapsack.On("GetRunID").Return(ulid.New()).Maybe()

			if tt.shouldReplaceProvider {
				mockKnapsack.On("TraceIngestServerURL").Return("https://example.com")
			}

			clientAuthenticator := newClientAuthenticator("test token", tt.currentDisableTraceIngestTLS)

			ctx, cancel := context.WithCancel(t.Context())
			traceExporter := &TelemetryExporter{
				knapsack:                  mockKnapsack,
				bufSpanProcessor:          bufspanprocessor.NewBufSpanProcessor(500),
				slogger:                   multislogger.NewNopLogger(),
				attrs:                     make([]attribute.KeyValue, 0),
				attrLock:                  sync.RWMutex{},
				ingestClientAuthenticator: clientAuthenticator,
				ingestAuthToken:           "test token",
				ingestUrl:                 "localhost:4317",
				disableIngestTLS:          tt.currentDisableTraceIngestTLS,
				enabled:                   tt.tracingEnabled,
				traceSamplingRate:         1.0,
				ctx:                       ctx,
				cancel:                    cancel,
			}

			traceExporter.FlagsChanged(ctx, keys.DisableTraceIngestTLS)

			require.Equal(t, tt.newDisableTraceIngestTLS, traceExporter.disableIngestTLS, "ingest TLS value not updated")
			require.Equal(t, tt.newDisableTraceIngestTLS, clientAuthenticator.disableTLS, "ingest TLS value not updated for client authenticator")

			if tt.shouldReplaceProvider {
				require.NotNil(t, traceExporter.tracerProvider)
				require.NotNil(t, traceExporter.meterProvider)
			} else {
				require.Nil(t, traceExporter.tracerProvider)
				require.Nil(t, traceExporter.meterProvider)
			}
		})
	}
}

func TestFlagsChanged_TraceBatchTimeout(t *testing.T) { //nolint:paralleltest
	tests := []struct {
		testName              string
		currentBatchTimeout   time.Duration
		newBatchTimeout       time.Duration
		tracingEnabled        bool
		shouldReplaceProvider bool
	}{
		{
			testName:              "update",
			currentBatchTimeout:   1 * time.Minute,
			newBatchTimeout:       5 * time.Second,
			tracingEnabled:        true,
			shouldReplaceProvider: true,
		},
		{
			testName:              "update but tracing not enabled",
			currentBatchTimeout:   1 * time.Minute,
			newBatchTimeout:       5 * time.Second,
			tracingEnabled:        false,
			shouldReplaceProvider: false,
		},
		{
			testName:              "no update",
			currentBatchTimeout:   1 * time.Minute,
			newBatchTimeout:       1 * time.Minute,
			tracingEnabled:        true,
			shouldReplaceProvider: false,
		},
	}

	for _, tt := range tests { //nolint:paralleltest
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("TraceBatchTimeout").Return(tt.newBatchTimeout)
			mockKnapsack.On("UpdateChannel").Return("nightly").Maybe()
			mockKnapsack.On("GetRunID").Return(ulid.New()).Maybe()

			if tt.shouldReplaceProvider {
				mockKnapsack.On("TraceIngestServerURL").Return("https://example.com")
			}

			ctx, cancel := context.WithCancel(t.Context())
			traceExporter := &TelemetryExporter{
				knapsack:                  mockKnapsack,
				bufSpanProcessor:          bufspanprocessor.NewBufSpanProcessor(500),
				slogger:                   multislogger.NewNopLogger(),
				attrs:                     make([]attribute.KeyValue, 0),
				attrLock:                  sync.RWMutex{},
				ingestClientAuthenticator: newClientAuthenticator("test token", false),
				ingestAuthToken:           "test token",
				ingestUrl:                 "localhost:4317",
				disableIngestTLS:          false,
				enabled:                   tt.tracingEnabled,
				traceSamplingRate:         1.0,
				batchTimeout:              tt.currentBatchTimeout,
				ctx:                       ctx,
				cancel:                    cancel,
			}

			traceExporter.FlagsChanged(ctx, keys.TraceBatchTimeout)

			require.Equal(t, tt.newBatchTimeout, traceExporter.batchTimeout, "batch timeout value not updated")

			if tt.shouldReplaceProvider {
				require.NotNil(t, traceExporter.tracerProvider)
				require.NotNil(t, traceExporter.meterProvider)
			} else {
				require.Nil(t, traceExporter.tracerProvider)
				require.Nil(t, traceExporter.meterProvider)
			}
		})
	}
}

func testServerProvidedDataStore(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ServerProvidedDataStore.String())
	require.NoError(t, err)
	return s
}

func testTokenStore(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	return s
}
