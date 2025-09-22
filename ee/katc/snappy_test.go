package katc

import (
	"context"
	"testing"

	"github.com/golang/snappy"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_snappyDecode(t *testing.T) {
	t.Parallel()

	expectedRow := map[string][]byte{
		"some_key_a": []byte("some_value_a"),
		"some_key_b": []byte("some_value_b"),
	}

	encodedRow := map[string][]byte{
		"some_key_a": snappy.Encode(nil, expectedRow["some_key_a"]),
		"some_key_b": snappy.Encode(nil, expectedRow["some_key_b"]),
	}

	results, err := snappyDecode(context.TODO(), multislogger.NewNopLogger(), encodedRow)
	require.NoError(t, err)

	// Validate that the keys are unchanged, and that the data was correctly decoded
	require.Equal(t, expectedRow, results)
}
