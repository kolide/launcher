package ringlogger

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/go-kit/kit/log"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/persistentring"
	"github.com/stretchr/testify/require"
)

func TestRingLogger(t *testing.T) {
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

	results, err := rl.GetAll()
	require.NoError(t, err)

	actual := make([]int, ringSize)
	for i, res := range results {
		var logLine struct{ I int }
		reader := bytes.NewReader(res)
		require.NoError(t, json.NewDecoder(reader).Decode(&logLine))
		actual[i] = logLine.I

	}

	require.Equal(t, expected, actual)
}
