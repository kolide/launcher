package packaging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchOsqueryBinary(t *testing.T) {
	_, err := FetchOsquerydBinary("stable", "darwin")
	require.NoError(t, err)
}
