package ringlogger

import (
	"testing"

	"github.com/go-kit/kit/log"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/persistentring"
	"github.com/stretchr/testify/require"
)

func TestRingLogger(t *testing.T) {
	t.Parallel()

	ringSize := uint16(10)

	s, err := storageci.NewStore(nil, log.NewNopLogger(), "persistenring-test")
	require.NoError(t, err)

	r, err := persistentring.New(s, ringSize)
	require.NoError(t, err)

	rl, err := New(r)
	require.NoError(t, err)

	for i := uint16(0); i < 2*ringSize; i++ {
		require.NoError(t, rl.Log("msg", "a random log", "i", i))
	}

	expected := []int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}

	logs, err := rl.GetAll()
	require.NoError(t, err)

	actual := make([]int, ringSize)

	for i, logLine := range logs {
		actual[i] = int(logLine["i"].(float64))
	}

	require.Equal(t, expected, actual)
}

func TestRingLoggerBufferClearing(t *testing.T) {
	t.Parallel()

	ringSize := uint16(10)

	s, err := storageci.NewStore(nil, log.NewNopLogger(), "persistenring-test")
	require.NoError(t, err)

	s = inmemory.NewStore(log.NewNopLogger())

	r, err := persistentring.New(s, ringSize)
	require.NoError(t, err)

	rl, err := New(r)
	require.NoError(t, err)

	require.NoError(t, rl.Log("msg", "This is a very long message"))
	require.NoError(t, rl.Log("msg", "short"))
	require.NoError(t, rl.Log("msg", "Another very long message"))

	// By calling GetAll this attempts to unmarshal the underlying json data. which has the effect of
	// validating that all the []byte buffers work as expected
	{
		_, err := rl.GetAll()
		require.NoError(t, err)
	}
}
