//go:build darwin

package network_time

import (
	"strings"
)

func parseSystemsetupOutput(output string) map[string]string {
	result := make(map[string]string, len(settings))

	for line := range strings.Lines(output) {
		line = strings.TrimSpace(line)

		for _, s := range settings {
			if !strings.HasPrefix(line, s.outputPrefix) {
				continue
			}

			result[s.columnName] = strings.TrimSpace(strings.TrimPrefix(line, s.outputPrefix))
			break
		}
	}

	return result
}

// Converts string values into booleans
func sanitizeState(state string) string {
	switch strings.ToLower(state) {
	case "off":
		return "0"
	case "on":
		return "1"
	default:
		return ""
	}
}
