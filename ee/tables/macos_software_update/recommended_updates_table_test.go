//go:build darwin

package macos_software_update

import (
	"testing"

	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_generateRecommendedUpdatesHappyPath(t *testing.T) {
	t.Parallel()
	table := Table{slogger: multislogger.NewNopLogger()}

	_, err := table.generate(t.Context(), tablehelpers.MockQueryContext(nil))

	// Since the output is dynamic and can be empty, just verify no error
	require.NoError(t, err)
}
