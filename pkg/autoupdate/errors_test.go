package autoupdate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLauncherRestartNeededError(t *testing.T) {
	t.Parallel()

	restartErr := NewLauncherRestartNeededErr("an error")
	require.Error(t, restartErr)
	require.True(t, IsLauncherRestartNeededErr(restartErr))

	otherErr := errors.New("an error")
	require.Error(t, otherErr)
	require.False(t, IsLauncherRestartNeededErr(otherErr))

}
