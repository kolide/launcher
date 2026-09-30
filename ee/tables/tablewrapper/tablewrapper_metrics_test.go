package tablewrapper

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/flags/keys"
	typesmocks "github.com/kolide/launcher/v2/ee/agent/types/mocks"
	"github.com/kolide/launcher/v2/ee/observability"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// Not parallel: this test replaces the global meter provider, so it must not
// interleave with other tests that increment the package-level counters.
func TestCall_recordsTableHealthMetrics(t *testing.T) { //nolint:paralleltest
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	observability.ReinitializeMetrics()
	t.Cleanup(func() {
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		observability.ReinitializeMetrics()
		require.NoError(t, mp.Shutdown(context.Background()))
	})

	testCases := []struct {
		name          string
		gen           table.GenerateFunc
		expectedQuery int64
		expectedError int64
		expectedEmpty int64
	}{
		{
			name: "success with rows",
			gen: func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
				return []map[string]string{{"somekey": "somevalue"}}, nil
			},
			expectedQuery: 1,
		},
		{
			name: "success with no rows",
			gen: func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
				return nil, nil
			},
			expectedQuery: 1,
			expectedEmpty: 1,
		},
		{
			name: "error",
			gen: func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
				return nil, errors.New("generate failed")
			},
			expectedQuery: 1,
			expectedError: 1,
		},
		{
			name: "panic",
			gen: func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
				panic("panic in generate") //nolint:forbidigo // Fine to use panic in tests
			},
			expectedQuery: 1,
			expectedError: 1,
		},
	}

	// Each subtest uses its own table name, so its data points don't mix with other subtests'.
	for _, tt := range testCases { //nolint:paralleltest // subtests share the global meter provider
		t.Run(tt.name, func(t *testing.T) {
			mockFlags := typesmocks.NewFlags(t)
			mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
			mockFlags.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return()

			tableName := "test_metrics_" + tt.name
			w := New(mockFlags, multislogger.NewNopLogger(), tableName, nil, tt.gen)
			w.Call(t.Context(), map[string]string{"action": "generate", "context": "{}"})

			var rm metricdata.ResourceMetrics
			require.NoError(t, reader.Collect(t.Context(), &rm))

			require.Equal(t, tt.expectedQuery, counterValueForTable(t, &rm, "launcher.tablewrapper.query", tableName), "query counter")
			require.Equal(t, tt.expectedError, counterValueForTable(t, &rm, "launcher.tablewrapper.error", tableName), "error counter")
			require.Equal(t, tt.expectedEmpty, counterValueForTable(t, &rm, "launcher.tablewrapper.empty", tableName), "empty counter")
		})
	}
}

// counterValueForTable returns the value of the data point with the given table_name
// attribute on the named counter, or 0 if no such data point has been recorded.
func counterValueForTable(t *testing.T, rm *metricdata.ResourceMetrics, metricName string, tableName string) int64 {
	t.Helper()

	wantAttr := attribute.String("table_name", tableName)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != metricName {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "expected %s to be an int64 sum", metricName)
			for _, dp := range sum.DataPoints {
				if val, found := dp.Attributes.Value(wantAttr.Key); found && val.AsString() == tableName {
					return dp.Value
				}
			}
		}
	}
	return 0
}
