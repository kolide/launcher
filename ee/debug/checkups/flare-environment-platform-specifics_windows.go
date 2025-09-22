//go:build windows
// +build windows

package checkups

import "golang.org/x/sys/windows"

func flareEnvironmentPlatformSpecifics(flareEnv map[string]any) {
	flareEnv["invoked_with_elevated_permissions"] = windows.GetCurrentProcessToken().IsElevated()
}
