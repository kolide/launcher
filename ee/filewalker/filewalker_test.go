package filewalker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkFilewalk(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		results, err := Filewalk(b.Context(), "/Users/")
		require.NoError(b, err)
		require.LessOrEqual(b, 1470000, len(results))
	}
}

func BenchmarkFastwalkWithChannel(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		results, err := FastwalkWithChannel(b.Context(), "/Users/")
		require.NoError(b, err)
		require.LessOrEqual(b, 1470000, len(results))
	}
}

func BenchmarkFastwalkWithLock(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		results, err := FastwalkWithLock(b.Context(), "/Users/")
		require.NoError(b, err)
		require.LessOrEqual(b, 1470000, len(results))
	}
}
