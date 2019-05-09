package authenticode

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

//const notepadeExe = `C:\Windows\System32\Notepad.exe`
const (
	srcExe       = `C:\Windows\System32\netmsg.dll`
	signtoolPath = `C:\Program Files (x86)\Windows Kits\10\bin\10.0.18362.0\x64\signtool.exe`
)

func TestSign(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("not windows")
	}

	tmpDir, err := ioutil.TempDir("", "packagekit-authenticode-signing")
	defer os.RemoveAll(tmpDir)
	require.NoError(t, err)

	testExe := filepath.Join(tmpDir, "test.exe")

	// confirm that we _don't_ have a sig on this file
	verifyInitial, err := so.execOut(ctx, signtoolPath, "verify", "/pa", testExe)
	require.Error(t, err, "no initial signature")

	// copy our test file
	data, err := ioutil.ReadFile(srcExe)
	require.NoError(t, err)
	err = ioutil.WriteFile(testExe, data, 0755)
	require.NoError(t, err)

	// Sign it!
	err = Sign(context.TODO(), testExe, WithSigntoolPath(signtoolPath))
	require.NoError(t, err)

	// verify, as an explicit test. Gotta check both indexes manually.
	verifyOut0, err := so.execOut(ctx, signtoolPath, "verify", "/pa", "/ds", "0", testExe)
	require.NoError(t, err, "verify signature position 0")
	require.Contains(t, verifyOut0, "sha1", "contains algorithm verify output")
	require.Contains(t, verifyOut0, "Authenticode", "contains timestamp verify output")

	verifyOut1, err := so.execOut(ctx, signtoolPath, "verify", "/pa", "/ds", "1", testExe)
	require.NoError(t, err, "verify signature position 1")
	require.Contains(t, verifyOut1, "sha1", "contains algorithm verify output")
	require.Contains(t, verifyOut1, "Authenticode", "contains timestamp verify output")

}
