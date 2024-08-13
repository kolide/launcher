//go:build linux
// +build linux

package launcher

import (
	"fmt"
	"regexp"
	"strings"
)

// allow alphanumeric characters plus - or _ within the identifier, we will replace anything else with a dash
var linuxIdentifierWhitelistRegex = regexp.MustCompile(`[^a-zA-Z0-9-_]+`)

const defaultLauncherIdentifier string = "kolide-k2"

// ServiceName embeds the given identifier into our service name template after sanitization,
// and returns the service name (label) generated to match our packaging logic
func ServiceName(identifier string) string {
	// this check might be overkill but is intended to prevent any backwards compatibility/misconfiguration issues
	if strings.TrimSpace(identifier) == "" {
		identifier = defaultLauncherIdentifier
	}

	sanitizedServiceName := linuxIdentifierWhitelistRegex.ReplaceAllString(identifier, "-")

	return fmt.Sprintf("launcher.%s.service", sanitizedServiceName)
}
