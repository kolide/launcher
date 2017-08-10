package packaging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchOsqueryBinary(t *testing.T) {
	path1, err := FetchOsquerydBinary("stable", "linux")
	require.NoError(t, err)

	path2, err := FetchOsquerydBinary("stable", "linux")
	require.NoError(t, err)

	require.Equal(t, path1, path2)
}
