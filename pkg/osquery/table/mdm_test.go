//go:build darwin

package table

import (
	"testing"

	"github.com/kolide/kit/env"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestMDMProfileStatus(t *testing.T) {
	t.Parallel()

	if env.Bool("SKIP_TEST_MDM", true) {
		t.Skip("Skipping MDM Test")
	}

	_, err := getMDMProfileStatus(t.Context(), multislogger.NewNopLogger())
	require.Nil(t, err)
}
