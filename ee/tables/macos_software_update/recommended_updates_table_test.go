//go:build darwin
// +build darwin

package macos_software_update

import (
	"testing"

	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_generateRecommendedUpdatesHappyPath(t *testing.T) {
	t.Parallel()
	table := Table{slogger: multislogger.NewNopLogger()}

	_, err := table.generate(t.Context(), tablehelpers.MockQueryContext(nil))

	// Since the output is dynamic and can be empty, just verify no error
	require.NoError(t, err)
}
