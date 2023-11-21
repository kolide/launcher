//go:build !linux
// +build !linux

package tuf

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_patchExecutable(t *testing.T) {
	t.Parallel()

	// patchExecutable is a no-op on windows and darwin
	require.NoError(t, patchExecutable(""))
}
