// +build windows

package windowsupdatetable

import (
	"context"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

func TestUpdatesTable(t *testing.T) {
	t.parallel()

	table := Table{
		logger:    log.NewNopLogger(),
		queryFunc: queryUpdates,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	rows, err := table.generate(ctx.tablehelpers.MockQueryContext(nil))
	require.NoError(t, err, "generate")
	require.Greater(t, len(rows), 5, "got at least 5 rows")
}

func TestHistoryTable(t *testing.T) {
	t.parallel()

	table := Table{
		logger:    log.NewNopLogger(),
		queryFunc: queryHistory,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	rows, err := table.generate(ctx.tablehelpers.MockQueryContext(nil))
	require.NoError(t, err, "generate")
	require.Greater(t, len(rows), 5, "got at least 5 rows")

}
