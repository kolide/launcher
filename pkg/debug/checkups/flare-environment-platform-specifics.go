//go:build !windows
// +build !windows

package checkups

func flareEnvironmentPlatformSpecifics(flareEnv map[string]any) {
	flareEnv["seph"] = "Sdf"
	return
}
