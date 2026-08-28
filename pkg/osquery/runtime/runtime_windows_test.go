//go:build windows

package runtime

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// hasPermissionsToRunTest return true if the current process has elevated permissions (administrator),
// this is required to run tests on windows
func hasPermissionsToRunTest() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// delayOsqueryd wraps the configured osqueryd invocation in a sleep script.
func delayOsqueryd(t *testing.T, delay time.Duration) OsqueryInstanceOption {
	script, err := os.CreateTemp(t.TempDir(), "*_osquery-wrapper.ps1")
	require.NoError(t, err, "failed to make temp file")
	// script file is simpler than escaping or encoding
	// powershell does not propagate a native exit code on its own
	_, err = script.WriteString("$delay, $exe, $rest = $args\nStart-Sleep -Seconds ([int]$delay)\n& $exe @rest\nexit $LASTEXITCODE\n")
	require.NoError(t, err)
	require.NoError(t, script.Close())

	t.Cleanup(func() {
		os.Remove(script.Name())
	})

	return WithStartFunc(func(cmd *exec.Cmd) error {
		powershell, err := exec.LookPath("powershell.exe")
		require.NoError(t, err)

		cmd.Args = append([]string{
			"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script.Name(),
			strconv.Itoa(int(delay.Seconds())), cmd.Path,
		}, cmd.Args[1:]...)
		cmd.Path = powershell

		return cmd.Start()
	})
}
