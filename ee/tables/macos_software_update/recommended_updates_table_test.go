//go:build darwin
// +build darwin

package macos_software_update

import (
	"context"
	"testing"

	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_generateRecommendedUpdatesHappyPath(t *testing.T) {
	t.Parallel()
	table := Table{slogger: multislogger.New().Logger}

	_, err := table.generate(context.Background(), tablehelpers.MockQueryContext(nil))

	// Since the output is dynamic and can be empty, just verify no error
	require.NoError(t, err)
}
