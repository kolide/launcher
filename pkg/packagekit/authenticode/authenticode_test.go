package authenticode

import (
	"context"
	"fmt"
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

	// copy our test file
	data, err := ioutil.ReadFile(srcExe)
	require.NoError(t, err)
	err = ioutil.WriteFile(testExe, data, 0755)
	require.NoError(t, err)

	// Sign it!
	fmt.Println("seph in ")
	err = Sign(context.TODO(), testExe, WithSigntoolPath("echo"))
	require.NoError(t, err)

}
