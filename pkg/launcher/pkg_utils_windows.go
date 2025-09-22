//go:build windows
// +build windows

package launcher

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/serenize/snaker"
)

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// ServiceName embeds the given identifier into our service name template after sanitization,
// and returns the camelCased service name generated to match our packaging logic
func ServiceName(identifier string) string {
	// this check might be overkill but is intended to prevent any backwards compatibility/misconfiguration issues
	if strings.TrimSpace(identifier) == "" {
		identifier = DefaultLauncherIdentifier
	}

	sanitizedServiceName := nonAlphanumericRegex.ReplaceAllString(identifier, "_") // e.g. identifier=kolide-k2 becomes kolide_k2
	sanitizedServiceName = fmt.Sprintf("launcher_%s_svc", sanitizedServiceName)    // wrapped as launcher_kolide_k2_svc
	return snaker.SnakeToCamel(sanitizedServiceName)                               // will produce LauncherKolideK2Svc
}

// TaskName embeds the given identifier into our task name template after sanitization,
// and returns the camelCased service name generated to match our packaging logic
func TaskName(identifier, taskType string) string {
	if strings.TrimSpace(identifier) == "" {
		identifier = DefaultLauncherIdentifier
	}

	sanitizedIdentifier := nonAlphanumericRegex.ReplaceAllString(identifier, "_")                   // e.g. identifier=kolide-k2 becomes kolide_k2
	sanitizedTaskType := nonAlphanumericRegex.ReplaceAllString(taskType, "_")                       // e.g. taskName=watchdog
	sanitizedTaskName := fmt.Sprintf("launcher_%s_%s_task", sanitizedIdentifier, sanitizedTaskType) // wrapped as launcher_kolide_k2_watchdog_task
	return snaker.SnakeToCamel(sanitizedTaskName)                                                   // will produce LauncherKolideK2WatchdogTask
}
