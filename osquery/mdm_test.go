// +build darwin

package osquery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMDMProfileStatus(t *testing.T) {
	_, err := getMDMProfileStatus()
	require.Nil(t, err)
}
