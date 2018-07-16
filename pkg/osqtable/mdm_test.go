// +build darwin

package osqtable

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMDMProfileStatus(t *testing.T) {
	_, err := getMDMProfileStatus()
	require.Nil(t, err)
}
