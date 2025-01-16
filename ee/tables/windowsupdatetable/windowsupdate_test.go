//go:build windows
// +build windows

package windowsupdatetable

import (
	"context"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/pkg/log/multislogger"
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

func BenchmarkUpdatesTable(b *testing.B) {
	for range b.N {
		table := Table{
			slogger:   multislogger.NewNopLogger(),
			queryFunc: queryUpdates,
		}

		_, err := table.generate(context.TODO(), tablehelpers.MockQueryContext(nil))
		require.NoError(b, err, "expected no error generating rows")
	}

	/*
		goos: windows
		goarch: amd64
		pkg: github.com/kolide/launcher/ee/tables/windowsupdatetable
		cpu: 12th Gen Intel(R) Core(TM) i5-1235U
		BenchmarkUpdatesTable-12               1        11490456300 ns/op
		BenchmarkUpdatesTable-12               1        10991756100 ns/op
		BenchmarkUpdatesTable-12               1        11363828700 ns/op
		BenchmarkUpdatesTable-12               1        29924337500 ns/op
		BenchmarkUpdatesTable-12               1        9703925900 ns/op
		BenchmarkUpdatesTable-12               1        11219771500 ns/op
	*/
}
