package filewalker

import (
	"runtime"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func BenchmarkFilewalk(b *testing.B) {
	var testDir string
	switch runtime.GOOS {
	case "windows":
		testDir = `D:\a\`
	case "darwin":
		testDir = "/Users/"
	default:
		testDir = "/home/"
	}

	testFilewalker := newFilewalker(filewalkConfig{
		name:          "benchtest",
		walkInterval:  1 * time.Minute,
		rootDir:       testDir,
		fileNameRegex: nil,
	}, multislogger.NewNopLogger())

	b.ReportAllocs()
	for b.Loop() {
		testFilewalker.filewalk(b.Context())
		results := testFilewalker.Paths()
		require.LessOrEqual(b, 100, len(results))
	}
}
