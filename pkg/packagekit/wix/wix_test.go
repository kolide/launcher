package wix

import (
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/env"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/packagekit/wix/internal"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

//go:generate go-bindata -pkg internal -o internal/assets.go internal/assets/

func TestWixPackage(t *testing.T) {
	t.Parallel()

	if !env.Bool("CI_TEST_PACKAGING", false) {
		t.Skip("No docker")
	}

	ctx := context.Background()

	// TODO this should be able to be t.Log or t or something
	logger := log.NewLogfmtLogger(os.Stderr)

	ctx = ctxlog.NewContext(ctx, logger)

	packageRoot, err := ioutil.TempDir("", "packaging-root")
	require.NoError(t, err)
	defer os.RemoveAll(packageRoot)

	err = setupPackageRoot(packageRoot)
	require.NoError(t, err)

	outMsi, err := ioutil.TempFile("", "wix-test-*.msi")
	require.NoError(t, err)
	defer os.Remove(outMsi.Name())

	mainWxsContent, err := internal.Asset("internal/assets/product.wxs")
	require.NoError(t, err)

	wixTool, err := New(packageRoot,
		mainWxsContent,
		As32bit(),                 // wine is 32bit
		SkipValidation(),          // wine can't validate
		WithDocker("felfert/wix"), // TODO Use a Kolide distributed Dockerfile
		WithWix("/opt/wix/bin"),
	)
	require.NoError(t, err)
	defer wixTool.Cleanup()

	err = wixTool.Package(ctx, outMsi)
	require.NoError(t, err)

	verifyMsi(ctx, t, outMsi)
}

// verifyMSI attempts to very MSI correctness. It leverages 7zip,
// which can mostly read MSI files.
func verifyMsi(ctx context.Context, t *testing.T, outMsi *os.File) {
	// Use the wix struct for its execOut
	execWix := &wixOptions{execCC: exec.CommandContext}

	fileContents, err := execWix.execOut(ctx, "7z", "x", "-so", outMsi.Name())
	require.NoError(t, err)
	// TODO Is this fragile? I don't know if file order is guaranteed.
	require.Equal(t, "HelloVroom Vroom", fileContents)

	listOutput, err := execWix.execOut(ctx, "7z", "l", outMsi.Name())
	require.NoError(t, err)
	require.Contains(t, listOutput, "Path = go.cab")
	require.Contains(t, listOutput, "2 files")
}

func setupPackageRoot(packageRoot string) error {
	binDir := filepath.Join(packageRoot, "bin")

	if err := os.MkdirAll(binDir, 0700); err != nil {
		return errors.Wrap(err, "mkdir bin")
	}

	if err := ioutil.WriteFile(filepath.Join(binDir, "hello.txt"), []byte("Hello"), 0755); err != nil {
		return errors.Wrap(err, "Unable to write bin/hello.txt")
	}

	varDir := filepath.Join(packageRoot, "var")

	if err := os.MkdirAll(varDir, 0700); err != nil {
		return errors.Wrap(err, "mkdir var")
	}

	if err := ioutil.WriteFile(filepath.Join(varDir, "vroom.txt"), []byte("Vroom Vroom"), 0755); err != nil {
		return errors.Wrap(err, "Unable to write var/vroom.txt")
	}
	return nil
}
