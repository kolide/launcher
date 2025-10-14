package katc

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_hexDecode(t *testing.T) {
	t.Parallel()

	originalValue := []byte("some_test_data")
	encodedStr := hex.EncodeToString(originalValue)
	encodedStrQuoted := fmt.Sprintf("X'%s'", encodedStr)

	results, err := hexDecode(t.Context(), multislogger.NewNopLogger(), map[string][]byte{
		"data": []byte(encodedStrQuoted),
	})
	require.NoError(t, err)
	require.Contains(t, results, "data")
	require.Equal(t, originalValue, results["data"])
}
