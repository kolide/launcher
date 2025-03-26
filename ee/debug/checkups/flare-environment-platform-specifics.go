//go:build !windows
// +build !windows

package checkups

import "os"

func flareEnvironmentPlatformSpecifics(flareEnv map[string]any) {
	flareEnv["seph"] = "Sdf"
	flareEnv["invoked_as_root"] = os.Geteuid() == 0
}
