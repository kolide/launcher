package filewalker

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func testDir() string {
	switch runtime.GOOS {
	case "windows":
		return `D:\a\`
	case "darwin":
		return "/Users/"
	default:
		return "/home/"
	}
}

func BenchmarkFilewalk(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		results, err := Filewalk(b.Context(), testDir())
		require.NoError(b, err)
		require.LessOrEqual(b, 100, len(results))
	}
}

func BenchmarkFastwalkWithChannel(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		results, err := FastwalkWithChannel(b.Context(), testDir())
		require.NoError(b, err)
		require.LessOrEqual(b, 100, len(results))
	}
}

func BenchmarkFastwalkWithLock(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		results, err := FastwalkWithLock(b.Context(), testDir())
		require.NoError(b, err)
		require.LessOrEqual(b, 100, len(results))
	}
}
