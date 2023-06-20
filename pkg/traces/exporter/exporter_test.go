package exporter

import (
	"runtime"
	"sync"
	"testing"

	"github.com/go-kit/kit/log"
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
		ingestClientAuthenticator: newClientAuthenticator("test token"),
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
	clientAuthenticator := newClientAuthenticator(initialTestToken)

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
				ingestClientAuthenticator: newClientAuthenticator("test token"),
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
				ingestClientAuthenticator: newClientAuthenticator("test token"),
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

func TestFlagsChanged_ObservabilityIngestServerURL(t *testing.T) { //nolint:paralleltest
	tests := []struct {
		testName                            string
		currentObservabilityIngestServerURL string
		newObservabilityIngestServerURL     string
		tracingEnabled                      bool
		shouldReplaceProvider               bool
	}{
		{
			testName:                            "update ingest URL",
			currentObservabilityIngestServerURL: "localhost:3417",
			newObservabilityIngestServerURL:     "localhost:3418",
			tracingEnabled:                      true,
			shouldReplaceProvider:               true,
		},
		{
			testName:                            "update ingest URL, but tracing not enabled",
			currentObservabilityIngestServerURL: "localhost:3417",
			newObservabilityIngestServerURL:     "localhost:3418",
			tracingEnabled:                      false,
			shouldReplaceProvider:               false,
		},
		{
			testName:                            "no update to ingest URL",
			currentObservabilityIngestServerURL: "localhost:3417",
			newObservabilityIngestServerURL:     "localhost:3417",
			tracingEnabled:                      true,
			shouldReplaceProvider:               false,
		},
	}

	for _, tt := range tests { //nolint:paralleltest
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("ObservabilityIngestServerURL").Return(tt.newObservabilityIngestServerURL)
			osqueryClient := mocks.NewQuerier(t)

			traceExporter := &TraceExporter{
				knapsack:                  mockKnapsack,
				osqueryClient:             osqueryClient,
				logger:                    log.NewNopLogger(),
				attrs:                     make([]attribute.KeyValue, 0),
				attrLock:                  sync.RWMutex{},
				ingestClientAuthenticator: newClientAuthenticator("test token"),
				ingestAuthToken:           "test token",
				ingestUrl:                 tt.currentObservabilityIngestServerURL,
				disableIngestTLS:          false,
				enabled:                   tt.tracingEnabled,
				traceSamplingRate:         1.0,
			}

			traceExporter.FlagsChanged(keys.ObservabilityIngestServerURL)

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

			traceExporter := &TraceExporter{
				knapsack:                  mockKnapsack,
				osqueryClient:             osqueryClient,
				logger:                    log.NewNopLogger(),
				attrs:                     make([]attribute.KeyValue, 0),
				attrLock:                  sync.RWMutex{},
				ingestClientAuthenticator: newClientAuthenticator("test token"),
				ingestAuthToken:           "test token",
				ingestUrl:                 "localhost:4317",
				disableIngestTLS:          tt.currentDisableObservabilityIngestTLS,
				enabled:                   tt.tracingEnabled,
				traceSamplingRate:         1.0,
			}

			traceExporter.FlagsChanged(keys.DisableObservabilityIngestTLS)

			require.Equal(t, tt.newDisableObservabilityIngestTLS, traceExporter.disableIngestTLS, "ingest TLS value not updated")

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
