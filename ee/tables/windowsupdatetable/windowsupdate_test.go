//go:build windows
// +build windows

package windowsupdatetable

import (
	"context"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTable(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name      string
		queryFunc queryFuncType
	}{
		{name: "updates", queryFunc: queryUpdates},
		{name: "history", queryFunc: queryHistory},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			table := Table{
				slogger:   multislogger.NewNopLogger(),
				queryFunc: tt.queryFunc,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			// ci doesn;t return data, but we can, at least, check that the underlying API doesn't error.
			_, err := table.generate(ctx, tablehelpers.MockQueryContext(nil))
			require.NoError(t, err, "generate")
		})
	}

}

func BenchmarkWindowsUpdatesTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	updatesTable := TablePlugin(UpdatesTable, mockFlags, slogger)

	// Confirm we can call the table successfully
	response := updatesTable.Call(context.TODO(), map[string]string{
		"action":  "generate",
		"context": "{}",
	})

	require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
}

func BenchmarkWindowsHistoryTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	historyTable := TablePlugin(HistoryTable, mockFlags, slogger)

	// Confirm we can call the table successfully
	response := historyTable.Call(context.TODO(), map[string]string{
		"action":  "generate",
		"context": "{}",
	})

	require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
}
