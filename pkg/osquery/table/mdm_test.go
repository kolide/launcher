// +build darwin

package table

import (
	"testing"

	"github.com/kolide/kit/env"
	"github.com/stretchr/testify/require"
)

func TestMDMProfileStatus(t *testing.T) {
	if env.Bool("SKIP_TEST_MDM", true) {
		t.Skip("No docker")
	}

	_, err := getMDMProfileStatus()
	require.Nil(t, err)
}
