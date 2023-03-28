package persistentring

import (
	"strconv"
	"testing"

	"github.com/go-kit/log"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/stretchr/testify/require"
)

func TestIntByteHelpers(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		n int
	}{
		{n: 0},
		{n: 1},
		{n: 255},
		{n: 256},
		{n: 1023},
		{n: 1025},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(strconv.Itoa(tt.n), func(t *testing.T) {
			t.Parallel()

			b, err := intToByte(tt.n)
			require.NoError(t, err)

			actual, err := byteToInt(b)
			require.NoError(t, err)
			require.Equal(t, tt.n, actual)

		})
	}
}

func TestBadByteToInt(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		b []byte
	}{
		{b: nil},
		{b: []byte("hello")},
	}

	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			t.Parallel()

			_, err := byteToInt(tt.b)
			require.Error(t, err)
		})
	}
}

func TestRing(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		size     int
		input    []int
		expected []int
	}{}

	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			t.Parallel()

			s, err := storageci.NewStore(nil, log.NewNopLogger(), "persistenring-test")
			require.NoError(err)

			r := NewRing(s, tt.size)

		})
	}
}
