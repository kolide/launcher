package currentprocess

import (
	"os/user"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestIsElevated(t *testing.T) {
	t.Parallel()

	// Cannot consistently assert outcome, but it should never error.
	_, err := IsElevated()
	require.NoError(t, err)
}

func TestUid(t *testing.T) {
	t.Parallel()

	currentUser, err := user.Current()
	require.NoError(t, err)

	expected := currentUser.Uid
	if runtime.GOOS == "windows" {
		expected = currentUser.Username
	}

	uid, err := Uid()
	require.NoError(t, err)
	require.Equal(t, expected, uid)
}
