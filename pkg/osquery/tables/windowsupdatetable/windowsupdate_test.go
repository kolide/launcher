// +build windows

package windowsupdatetable

import (
	"context"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
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
		t.Run(tt.name, func(t *testing.T) {
			table := Table{
				logger:    log.NewNopLogger(),
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
