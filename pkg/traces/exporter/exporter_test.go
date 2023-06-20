package exporter

import (
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

func TestFlagsChanged_ExportTraces(t *testing.T) {
	t.Parallel()

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

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			s := setupStorage(t)
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

func TestFlagsChanged_TraceSamplingRate(t *testing.T) {
	t.Parallel()

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

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

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

func TestFlagsChanged_ObservabilityIngestServerURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName                            string
		currentObservabilityIngestServerURL string
		newObservabilityIngestServerURL     string
		tracingEnabled                      bool
		shouldReplaceProvider               bool
	}{
		{
			testName:                            "update",
			currentObservabilityIngestServerURL: "localhost:3417",
			newObservabilityIngestServerURL:     "localhost:3418",
			tracingEnabled:                      true,
			shouldReplaceProvider:               true,
		},
		{
			testName:                            "update but tracing not enabled",
			currentObservabilityIngestServerURL: "localhost:3417",
			newObservabilityIngestServerURL:     "localhost:3418",
			tracingEnabled:                      false,
			shouldReplaceProvider:               false,
		},
		{
			testName:                            "no update",
			currentObservabilityIngestServerURL: "localhost:3417",
			newObservabilityIngestServerURL:     "localhost:3417",
			tracingEnabled:                      true,
			shouldReplaceProvider:               false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

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

func TestFlagsChanged_DisableObservabilityIngestTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName                             string
		currentDisableObservabilityIngestTLS bool
		newDisableObservabilityIngestTLS     bool
		tracingEnabled                       bool
		shouldReplaceProvider                bool
	}{
		{
			testName:                             "update",
			currentDisableObservabilityIngestTLS: true,
			newDisableObservabilityIngestTLS:     false,
			tracingEnabled:                       true,
			shouldReplaceProvider:                true,
		},
		{
			testName:                             "update but tracing not enabled",
			currentDisableObservabilityIngestTLS: true,
			newDisableObservabilityIngestTLS:     false,
			tracingEnabled:                       false,
			shouldReplaceProvider:                false,
		},
		{
			testName:                             "no update",
			currentDisableObservabilityIngestTLS: false,
			newDisableObservabilityIngestTLS:     false,
			tracingEnabled:                       true,
			shouldReplaceProvider:                false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

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

func setupStorage(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), storage.ServerProvidedDataStore.String())
	require.NoError(t, err)
	return s
}
