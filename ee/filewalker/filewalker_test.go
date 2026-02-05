package filewalker

import (
	"runtime"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func BenchmarkFilewalk(b *testing.B) {
	testFilewalker := NewFilewalker(typesmocks.NewKnapsack(b), multislogger.NewNopLogger(), 1*time.Minute)

	var testDir string
	switch runtime.GOOS {
	case "windows":
		testDir = `D:\a\`
	case "darwin":
		testDir = "/Users/"
	default:
		testDir = "/home/"
	}

	b.ReportAllocs()
	for b.Loop() {
		results, err := testFilewalker.filewalk(b.Context(), testDir, nil)
		require.NoError(b, err)
		require.LessOrEqual(b, 100, len(results))
	}
}
