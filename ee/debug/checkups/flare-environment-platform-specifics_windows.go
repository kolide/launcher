//go:build windows

package checkups

import (
	"fmt"

	"github.com/kolide/launcher/v2/ee/currentprocess"
)

func flareEnvironmentPlatformSpecifics(flareEnv map[string]any) {
	elevated, err := currentprocess.IsElevated()

	if err != nil {
		flareEnv["invoked_with_elevated_permissions"] = "unknown"
		flareEnv["invoked_with_elevated_permissions_err"] = fmt.Sprintf("failed to check if elevated: %v", err)
	} else {
		flareEnv["invoked_with_elevated_permissions"] = elevated
	}
}
