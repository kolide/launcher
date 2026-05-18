package atomic

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBool(t *testing.T) {
	t.Parallel()

	var b Bool
	require.False(t, b.Load(), "zero value should be false")

	b.Store(true)
	require.True(t, b.Load())

	require.True(t, b.Swap(false), "Swap should return previous value")
	require.False(t, b.Load())

	require.False(t, b.Swap(true))
	require.True(t, b.Load())
}

func TestDuration(t *testing.T) {
	t.Parallel()

	d := NewDuration(5 * time.Second)
	require.Equal(t, 5*time.Second, d.Load())

	d.Store(2 * time.Minute)
	require.Equal(t, 2*time.Minute, d.Load())

	var zero Duration
	require.Equal(t, time.Duration(0), zero.Load(), "zero value should be 0")
}

func TestString(t *testing.T) {
	t.Parallel()

	s := NewString("hello")
	require.Equal(t, "hello", s.Load())

	s.Store("world")
	require.Equal(t, "world", s.Load())

	var zero String
	require.Equal(t, "", zero.Load(), "zero value should be empty string")

	zero.Store("set")
	require.Equal(t, "set", zero.Load())
}
