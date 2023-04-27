package persistentring

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/go-kit/kit/log"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/stretchr/testify/require"
)

func TestIntByteHelpers(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		n uint16
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
		t.Run(strconv.Itoa(int(tt.n)), func(t *testing.T) {
			t.Parallel()

			b := intToByte(tt.n)
			actual := byteToInt(b)
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
		{b: []byte{}},
		{b: []byte("")},
		{b: []byte("x")},
		{b: []byte{0}},
		{b: []byte{1}},
		{b: []byte{0, 0}},
		{b: []byte("hello")},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(string(tt.b), func(t *testing.T) {
			t.Parallel()

			// This is all garbage data, we're only checking that we don't panic.
			byteToInt(tt.b)
		})
	}
}

func TestRings(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		size     uint16
		input    [][]byte
		expected [][]byte
	}{
		{
			size:     3,
			input:    [][]byte{[]byte("a"), []byte("b"), []byte("c")},
			expected: [][]byte{[]byte("a"), []byte("b"), []byte("c")},
		},
		{
			size:     4,
			input:    [][]byte{[]byte("a"), []byte("b"), []byte("c")},
			expected: [][]byte{nil, []byte("a"), []byte("b"), []byte("c")},
		},
		{
			size:     2,
			input:    [][]byte{[]byte("a"), []byte("b"), []byte("c")},
			expected: [][]byte{[]byte("b"), []byte("c")},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			t.Parallel()

			t.Run("persisted", func(t *testing.T) {
				t.Parallel()
				s, err := storageci.NewStore(nil, log.NewNopLogger(), "persistenring-test")
				require.NoError(t, err)

				r, err := New(s, tt.size)
				require.NoError(t, err)

				for _, v := range tt.input {
					require.NoError(t, r.Add(v))
				}

				actual, err := r.GetAll()
				require.NoError(t, err)
				require.Equal(t, tt.expected, actual)
			})

			t.Run("inmemory", func(t *testing.T) {
				t.Parallel()

				r := NewInMemory(tt.size)

				for _, v := range tt.input {
					require.NoError(t, r.Add(v))
				}

				actual, err := r.GetAll()
				require.NoError(t, err)
				require.Equal(t, tt.expected, actual)
			})

		})
	}
}

func TestRingBig(t *testing.T) {
	t.Parallel()

	bigSize := uint16(2048)

	seed := int64(78)
	random := rand.New(rand.NewSource(seed))

	data := make([][]byte, bigSize+10)

	for i := 0; i < cap(data); i++ {
		data[i] = randomTestBytes(random)
	}

	// To avoid the transaction overhead during this big tests, we explcitely create an inmemory store.
	s := inmemory.NewStore(log.NewNopLogger())

	r, err := New(s, bigSize)
	require.NoError(t, err)

	for _, v := range data {
		require.NoError(t, r.Add(v))
	}

	actual, err := r.GetAll()
	require.NoError(t, err)
	lastPos := uint16(len(data)) - bigSize
	require.Equal(t, data[lastPos:], actual) // FIXME: need last bigSize

}

const randomLetters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ 0123456789!@#$%^&*()"

func randomTestBytes(rand *rand.Rand) []byte {
	n := 3 // rand.Intn(1024)
	b := make([]byte, n)
	for i := range b {
		/* #nosec */
		b[i] = randomLetters[rand.Intn(len(randomLetters))]
	}
	return b
}
