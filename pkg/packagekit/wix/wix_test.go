package wix

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/env"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"

	"github.com/stretchr/testify/require"
)

//go:embed testdata
var testdata embed.FS

func TestWixPackage(t *testing.T) {
	t.Parallel()

	if !env.Bool("CI_TEST_PACKAGING", false) {
		t.Skip("No docker")
	}

	ctx := context.Background()

	// TODO this should be able to be t.Log or t or something
	logger := log.NewLogfmtLogger(os.Stderr)

	ctx = ctxlog.NewContext(ctx, logger)

	packageRoot := t.TempDir()

	err := setupPackageRoot(packageRoot)
	require.NoError(t, err)

	mainWxsContent, err := testdata.ReadFile("product.wxs")
	require.NoError(t, err)

	wixTool, err := New(packageRoot,
		mainWxsContent,
		As32bit(),                 // wine is 32bit
		SkipValidation(),          // wine can't validate
		WithDocker("felfert/wix"), // TODO Use a Kolide distributed Dockerfile
		WithWix("/opt/wix/bin"),
		WithService(NewService("hello.txt")),
	)
	require.NoError(t, err)
	defer wixTool.Cleanup()

	outMsi, err := wixTool.Package(ctx)
	require.NoError(t, err)

	verifyMsi(ctx, t, outMsi)
}

// verifyMSI attempts to very MSI correctness. It leverages 7zip,
// which can mostly read MSI files.
func verifyMsi(ctx context.Context, t *testing.T, outMsi string) {
	// Use the wix struct for its execOut
	execWix := &wixTool{execCC: exec.CommandContext}

	fileContents, err := execWix.execOut(ctx, "7z", "x", "-so", outMsi)
	require.NoError(t, err)
	require.Contains(t, fileContents, "Hello")
	require.Contains(t, fileContents, "Vroom Vroom")

	listOutput, err := execWix.execOut(ctx, "7z", "l", outMsi)
	require.NoError(t, err)
	require.Contains(t, listOutput, "Path = go.cab")
	require.Contains(t, listOutput, "2 files")
}

func setupPackageRoot(packageRoot string) error {
	binDir := filepath.Join(packageRoot, "bin")

	if err := os.MkdirAll(binDir, 0700); err != nil {
		return fmt.Errorf("mkdir bin: %w", err)
	}

	if err := os.WriteFile(filepath.Join(binDir, "hello.txt"), []byte("Hello"), 0755); err != nil {
		return fmt.Errorf("Unable to write bin/hello.txt: %w", err)
	}

	varDir := filepath.Join(packageRoot, "var")

	if err := os.MkdirAll(varDir, 0700); err != nil {
		return fmt.Errorf("mkdir var: %w", err)
	}

	if err := os.WriteFile(filepath.Join(varDir, "vroom.txt"), []byte("Vroom Vroom"), 0755); err != nil {
		return fmt.Errorf("Unable to write var/vroom.txt: %w", err)
	}
	return nil
}
