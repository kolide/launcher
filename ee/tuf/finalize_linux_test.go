//go:build linux
// +build linux

package tuf

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_getInterpreter(t *testing.T) {
	t.Parallel()

	// Use the current executable in our test
	currentRunningExecutable, err := os.Executable()
	require.NoError(t, err, "getting current executable")

	// Confirm we pick the expected interpreter
	interpreter, err := getInterpreter(currentRunningExecutable)
	require.NoError(t, err, "expected no error getting interpreter")
	require.Equal(t, "ld-linux-x86-64.so.2", interpreter)
}
