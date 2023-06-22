package exporter

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/localserver/mocks"
	"github.com/kolide/launcher/pkg/agent/flags/keys"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	typesmocks "github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

// NB - Tests that result in calls to `setNewGlobalProvider` should not be run in parallel
// to avoid race condition complaints.

func TestNewTraceExporter(t *testing.T) { //nolint:paralleltest
	mockKnapsack := typesmocks.NewKnapsack(t)

	tokenStore := testTokenStore(t)
	mockKnapsack.On("TokenStore").Return(tokenStore)
	tokenStore.Set(observabilityIngestTokenKey, []byte("test token"))

	serverProvidedDataStore := testServerProvidedDataStore(t)
	mockKnapsack.On("ServerProvidedDataStore").Return(serverProvidedDataStore)
	serverProvidedDataStore.Set([]byte("device_id"), []byte("500"))
	serverProvidedDataStore.Set([]byte("munemo"), []byte("nababe"))
	serverProvidedDataStore.Set([]byte("organization_id"), []byte("101"))
	serverProvidedDataStore.Set([]byte("serial_number"), []byte("abcdabcd"))

	mockKnapsack.On("TraceIngestServerURL").Return("localhost:3417")
	mockKnapsack.On("DisableObservabilityIngestTLS").Return(false)
	mockKnapsack.On("ExportTraces").Return(true)
	mockKnapsack.On("TraceSamplingRate").Return(1.0)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ExportTraces, keys.TraceSamplingRate, keys.TraceIngestServerURL, keys.DisableObservabilityIngestTLS).Return(nil)

	osqueryClient := mocks.NewQuerier(t)
	osqueryClient.On("Query", mock.Anything).Return([]map[string]string{
		{
			"osquery_version": "5.8.0",
			"os_name":         runtime.GOOS,
			"os_version":      "3.4.5",
			"hostname":        "Test-Hostname2",
		},
	}, nil)

	traceExporter, err := NewTraceExporter(context.Background(), mockKnapsack, osqueryClient, log.NewNopLogger())
	require.NoError(t, err)

	// Wait a few seconds to allow the osquery queries to go through
	time.Sleep(5 * time.Second)

	// We expect a total of 12 attributes: 3 initial attributes, 5 from the ServerProvidedDataStore, and 4 from osquery
	traceExporter.attrLock.RLock()
	defer traceExporter.attrLock.RUnlock()
	require.Equal(t, 12, len(traceExporter.attrs))

	// Confirm we set a provider
	traceExporter.providerLock.Lock()
	defer traceExporter.providerLock.Unlock()
	require.NotNil(t, traceExporter.provider, "expected provider to be created")

	mockKnapsack.AssertExpectations(t)
	osqueryClient.AssertExpectations(t)
}

func TestNewTraceExporter_exportNotEnabled(t *testing.T) {
	t.Parallel()

	tokenStore := testTokenStore(t)
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("TokenStore").Return(tokenStore)
	tokenStore.Set(observabilityIngestTokenKey, []byte("test token"))
	mockKnapsack.On("TraceIngestServerURL").Return("localhost:3417")
	mockKnapsack.On("DisableObservabilityIngestTLS").Return(false)
	mockKnapsack.On("ExportTraces").Return(false)
	mockKnapsack.On("TraceSamplingRate").Return(0.0)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ExportTraces, keys.TraceSamplingRate, keys.TraceIngestServerURL, keys.DisableObservabilityIngestTLS).Return(nil)

	traceExporter, err := NewTraceExporter(context.Background(), mockKnapsack, mocks.NewQuerier(t), log.NewNopLogger())
	require.NoError(t, err)

	// Confirm we didn't set a provider
	require.Nil(t, traceExporter.provider, "expected disabled exporter to not create a provider but one was created")

	// Confirm we added basic attributes
	require.Equal(t, 3, len(traceExporter.attrs))
	for _, actualAttr := range traceExporter.attrs {
		switch actualAttr.Key {
		case "service.name":
			require.Equal(t, applicationName, actualAttr.Value.AsString())
		case "service.version":
			require.Equal(t, version.Version().Version, actualAttr.Value.AsString())
		case "host.arch":
			require.Equal(t, runtime.GOARCH, actualAttr.Value.AsString())
		default:
			t.Fatalf("unexpected attr %s", actualAttr.Key)
		}
	}

	mockKnapsack.AssertExpectations(t)
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
	s.Set([]byte("serial_number"), []byte(expectedSerialNumber))

	traceExporter := &TraceExporter{
		knapsack:                  mockKnapsack,
		osqueryClient:             mocks.NewQuerier(t),
		logger:                    log.NewNopLogger(),
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
	require.Equal(t, 5, len(traceExporter.attrs))
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

	osqueryClient := mocks.NewQuerier(t)
	osqueryClient.On("Query", mock.Anything).Return([]map[string]string{
		{
			"osquery_version": expectedOsqueryVersion,
			"os_name":         expectedOsName,
			"os_version":      expectedOsVersion,
			"hostname":        expectedHostname,
		},
	}, nil)

	traceExporter := &TraceExporter{
		knapsack:                  typesmocks.NewKnapsack(t),
		osqueryClient:             osqueryClient,
		logger:                    log.NewNopLogger(),
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

	osqueryClient.AssertExpectations(t)
}

func TestPing(t *testing.T) {
	t.Parallel()

	// Set up the client authenticator + exporter with an initial token
	initialTestToken := "test token A"
	clientAuthenticator := newClientAuthenticator(initialTestToken, false)

	s := testTokenStore(t)
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("TokenStore").Return(s)

	traceExporter := &TraceExporter{
		knapsack:                  mockKnapsack,
		osqueryClient:             mocks.NewQuerier(t),
		logger:                    log.NewNopLogger(),
		attrs:                     make([]attribute.KeyValue, 0),
		attrLock:                  sync.RWMutex{},
		ingestClientAuthenticator: clientAuthenticator,
		ingestAuthToken:           initialTestToken,
		ingestUrl:                 "localhost:4317",
		disableIngestTLS:          false,
		enabled:                   true,
		traceSamplingRate:         1.0,
	}

	// Simulate a new token being set by updating the data store
	newToken := "test token B"
	require.NoError(t, s.Set(observabilityIngestTokenKey, []byte(newToken)))

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
			osqueryClient := mocks.NewQuerier(t)

			if tt.shouldReplaceProvider {
				mockKnapsack.On("ServerProvidedDataStore").Return(s)
				osqueryClient.On("Query", mock.Anything).Return([]map[string]string{
					{
						"osquery_version": "5.9.0",
						"os_name":         "Windows",
						"os_version":      "11",
						"hostname":        "Test device",
					},
				}, nil)
			}

			traceExporter := &TraceExporter{
				knapsack:                  mockKnapsack,
				osqueryClient:             osqueryClient,
				logger:                    log.NewNopLogger(),
				attrs:                     make([]attribute.KeyValue, 0),
				attrLock:                  sync.RWMutex{},
				ingestClientAuthenticator: newClientAuthenticator("test token", false),
				ingestAuthToken:           "test token",
				ingestUrl:                 "localhost:4317",
				disableIngestTLS:          false,
				enabled:                   tt.currentEnableValue,
				traceSamplingRate:         1.0,
			}

			traceExporter.FlagsChanged(keys.ExportTraces)

			require.Equal(t, tt.newEnableValue, traceExporter.enabled, "enabled value not updated")

			if tt.shouldReplaceProvider {
				mockKnapsack.AssertExpectations(t)
				osqueryClient.AssertExpectations(t)
				require.Greater(t, len(traceExporter.attrs), 0)
				require.NotNil(t, traceExporter.provider)
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
			osqueryClient := mocks.NewQuerier(t)

			traceExporter := &TraceExporter{
				knapsack:                  mockKnapsack,
				osqueryClient:             osqueryClient,
				logger:                    log.NewNopLogger(),
				attrs:                     make([]attribute.KeyValue, 0),
				attrLock:                  sync.RWMutex{},
				ingestClientAuthenticator: newClientAuthenticator("test token", false),
				ingestAuthToken:           "test token",
				ingestUrl:                 "localhost:4317",
				disableIngestTLS:          false,
				enabled:                   tt.tracingEnabled,
				traceSamplingRate:         tt.currentTraceSamplingRate,
			}

			traceExporter.FlagsChanged(keys.TraceSamplingRate)

			require.Equal(t, tt.newTraceSamplingRate, traceExporter.traceSamplingRate, "trace sampling rate value not updated")

			if tt.shouldReplaceProvider {
				require.NotNil(t, traceExporter.provider)
			} else {
				require.Nil(t, traceExporter.provider)
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
			testName:                        "update ingest URL, but tracing not enabled",
			currentTraceIngestServerURL:     "localhost:3417",
			newObservabilityIngestServerURL: "localhost:3418",
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
			osqueryClient := mocks.NewQuerier(t)

			traceExporter := &TraceExporter{
				knapsack:                  mockKnapsack,
				osqueryClient:             osqueryClient,
				logger:                    log.NewNopLogger(),
				attrs:                     make([]attribute.KeyValue, 0),
				attrLock:                  sync.RWMutex{},
				ingestClientAuthenticator: newClientAuthenticator("test token", false),
				ingestAuthToken:           "test token",
				ingestUrl:                 tt.currentTraceIngestServerURL,
				disableIngestTLS:          false,
				enabled:                   tt.tracingEnabled,
				traceSamplingRate:         1.0,
			}

			traceExporter.FlagsChanged(keys.TraceIngestServerURL)

			require.Equal(t, tt.newObservabilityIngestServerURL, traceExporter.ingestUrl, "ingest url value not updated")

			if tt.shouldReplaceProvider {
				require.NotNil(t, traceExporter.provider)
			} else {
				require.Nil(t, traceExporter.provider)
			}
		})
	}
}

func TestFlagsChanged_DisableObservabilityIngestTLS(t *testing.T) { //nolint:paralleltest
	tests := []struct {
		testName                             string
		currentDisableObservabilityIngestTLS bool
		newDisableObservabilityIngestTLS     bool
		tracingEnabled                       bool
		shouldReplaceProvider                bool
	}{
		{
			testName:                             "update ingest TLS value",
			currentDisableObservabilityIngestTLS: true,
			newDisableObservabilityIngestTLS:     false,
			tracingEnabled:                       true,
			shouldReplaceProvider:                true,
		},
		{
			testName:                             "update ingest TLS value, but tracing not enabled",
			currentDisableObservabilityIngestTLS: true,
			newDisableObservabilityIngestTLS:     false,
			tracingEnabled:                       false,
			shouldReplaceProvider:                false,
		},
		{
			testName:                             "no update to ingest TLS value",
			currentDisableObservabilityIngestTLS: false,
			newDisableObservabilityIngestTLS:     false,
			tracingEnabled:                       true,
			shouldReplaceProvider:                false,
		},
	}

	for _, tt := range tests { //nolint:paralleltest
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("DisableObservabilityIngestTLS").Return(tt.newDisableObservabilityIngestTLS)
			osqueryClient := mocks.NewQuerier(t)

			clientAuthenticator := newClientAuthenticator("test token", tt.currentDisableObservabilityIngestTLS)

			traceExporter := &TraceExporter{
				knapsack:                  mockKnapsack,
				osqueryClient:             osqueryClient,
				logger:                    log.NewNopLogger(),
				attrs:                     make([]attribute.KeyValue, 0),
				attrLock:                  sync.RWMutex{},
				ingestClientAuthenticator: clientAuthenticator,
				ingestAuthToken:           "test token",
				ingestUrl:                 "localhost:4317",
				disableIngestTLS:          tt.currentDisableObservabilityIngestTLS,
				enabled:                   tt.tracingEnabled,
				traceSamplingRate:         1.0,
			}

			traceExporter.FlagsChanged(keys.DisableObservabilityIngestTLS)

			require.Equal(t, tt.newDisableObservabilityIngestTLS, traceExporter.disableIngestTLS, "ingest TLS value not updated")
			require.Equal(t, tt.newDisableObservabilityIngestTLS, clientAuthenticator.disableTLS, "ingest TLS value not updated for client authenticator")

			if tt.shouldReplaceProvider {
				require.NotNil(t, traceExporter.provider)
			} else {
				require.Nil(t, traceExporter.provider)
			}
		})
	}
}

func testServerProvidedDataStore(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), storage.ServerProvidedDataStore.String())
	require.NoError(t, err)
	return s
}

func testTokenStore(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	return s
}
