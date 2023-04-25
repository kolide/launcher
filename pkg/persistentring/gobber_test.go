package persistentring

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func XTestGobber(t *testing.T) {
	t.Parallel()

	gobber := NewGobber()

	var tests = []struct {
		in any
	}{
		{"hello"},
		{0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			t.Parallel()

			encoded, err := gobber.Encode(tt.in)
			require.NoError(t, err)

			spew.Dump(encoded)

			var decoded any
			require.NoError(t, gobber.Decode(encoded, decoded))

			require.Equal(t, tt.in, decoded)

		})
	}
}
