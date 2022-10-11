//go:build darwin
// +build darwin

package macos_software_update

import (
	"context"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/require"
)

func Test_generateRecommendedUpdatesHappyPath(t *testing.T) {
	table := Table{logger: log.NewNopLogger()}
	t.Run("HappyPath", func(t *testing.T) {
		_, err := table.generate(context.Background(), tablehelpers.MockQueryContext(nil))

		// Since the output is dynamic and can be empty, just verify no error
		require.NoError(t, err)
	})
}
