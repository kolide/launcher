package katc

import (
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_zstdDecode(t *testing.T) {
	t.Parallel()

	expectedRow := map[string][]byte{
		"some_key_a":              []byte("some_value_a"),
		"some_key_b":              []byte("some_value_b"),
		"some_uncompressed_key_c": []byte("some_uncompressed_value_c"),
		"some_uncompressed_key_d": []byte("some_uncompressed_value_d"),
		"some_empty_key_e":        []byte(""),
	}

	var encoder, _ = zstd.NewWriter(nil)

	encodedRow := map[string][]byte{
		"some_key_a":              encoder.EncodeAll(expectedRow["some_key_a"], make([]byte, 0, len(expectedRow["some_key_a"]))),
		"some_key_b":              encoder.EncodeAll(expectedRow["some_key_b"], make([]byte, 0, len(expectedRow["some_key_b"]))),
		"some_uncompressed_key_c": []byte("some_uncompressed_value_c"),
		"some_uncompressed_key_d": []byte("some_uncompressed_value_d"),
		"some_empty_key_e":        []byte(""),
	}

	results, err := zstdDecode(t.Context(), multislogger.NewNopLogger(), encodedRow)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Validate that the keys are unchanged, and that the data was correctly decoded
	require.Equal(t, expectedRow, results[0])
}
