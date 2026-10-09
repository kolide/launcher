package tufci

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed testdata/stub_osqueryd/main.go
var stubOsquerydSource []byte

// buildStubOsqueryd builds a stub osqueryd binary once that responds
// to the requested version. The path to the binary in a temporary folder is
// returned. Compilation occurs once and is reused by all subsequent callers.
//
// Function compiles with the go binary on the current process's path.
func buildStubOsqueryd(version string) (string, error) {
	dir, err := os.MkdirTemp("", "tufci-osqueryd-")
	if err != nil {
		return "", fmt.Errorf("creating stub build dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), stubOsquerydSource, 0644); err != nil {
		return "", fmt.Errorf("writing stub source: %w", err)
	}

	binaryPath := filepath.Join(dir, "osqueryd")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	cmd := exec.CommandContext(context.TODO(), "go", "build", "-o", binaryPath, "-ldflags", "-X main.version="+version, "main.go") //nolint:forbidigo // test-only build of a stub binary
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building stub osqueryd %s: %w: %s", version, err, out)
	}

	return binaryPath, nil
}

const binaryVersion = "5.99.0"

var buildStubOnce = sync.OnceValues(func() (string, error) {
	return buildStubOsqueryd(binaryVersion)
})

// CopyBinary copies the shared stub osqueryd to the provided path.
func CopyBinary(t *testing.T, executablePath string) {
	t.Helper()
	copyStub(t, buildStubOnce, executablePath)
}

const OlderBinaryVersion = "5.17.0"

var buildOlderStubOnce = sync.OnceValues(func() (string, error) {
	return buildStubOsqueryd(OlderBinaryVersion)
})

func CopyOlderBinary(t *testing.T, executablePath string) {
	t.Helper()
	copyStub(t, buildOlderStubOnce, executablePath)
}

func copyStub(t *testing.T, build func() (string, error), executablePath string) {
	t.Helper()
	stubPath, err := build()
	require.NoError(t, err, "could not build stub osqueryd, cannot proceed with tests")

	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
	require.NoError(t, os.Symlink(stubPath, executablePath))
}
