//go:build windows
// +build windows

package authenticode

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kolide/kit/env"
	"github.com/stretchr/testify/require"
)

const (
	srcExe       = `C:\Windows\System32\netmsg.dll`
	signtoolPath = `C:\Program Files (x86)\Windows Kits\10\bin\10.0.18362.0\x64\signtool.exe`
)

func TestSign(t *testing.T) {
	t.Parallel()

	if !env.Bool("CI_TEST_WINDOWS_SIGNING", false) {
		t.Skip("No codesign")
	}

	// create a signtoolOptions object so we can call the exec method
	so := &signtoolOptions{
		execCC: exec.CommandContext,
	}

	ctx, ctxCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer ctxCancel()

	tmpDir := t.TempDir()

	testExe := filepath.Join(tmpDir, "test.exe")

	// copy our test file
	data, err := os.ReadFile(srcExe)
	require.NoError(t, err)
	err = os.WriteFile(testExe, data, 0755)
	require.NoError(t, err)

	// confirm that we _don't_ have a sig on this file
	_, verifyInitial, err := so.execOut(ctx, signtoolPath, "verify", "/pa", testExe)
	require.Error(t, err, "no initial signature")
	require.Contains(t, verifyInitial, "No signature found", "no initial signature")

	// Sign it!
	err = Sign(ctx, testExe, WithSigntoolPath(signtoolPath))
	require.NoError(t, err)

	// verify, as an explicit test. Gotta check both indexes manually.
	verifyOut0, _, err := so.execOut(ctx, signtoolPath, "verify", "/pa", "/ds", "0", testExe)
	require.NoError(t, err, "verify signature position 0")
	require.Contains(t, verifyOut0, "sha1", "contains algorithm verify output")
	require.Contains(t, verifyOut0, "Authenticode", "contains timestamp verify output")

	verifyOut1, _, err := so.execOut(ctx, signtoolPath, "verify", "/pa", "/ds", "1", testExe)
	require.NoError(t, err, "verify signature position 1")
	require.Contains(t, verifyOut1, "sha256", "contains algorithm verify output")
	require.Contains(t, verifyOut1, "RFC3161", "contains timestamp verify output")

}
