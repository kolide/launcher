//go:build darwin

package network_time

import (
	"bufio"
	"bytes"
	"strings"
)

// parseSystemsetupOutput parses the output of `systemsetup -getX`, mapping each
// column name to the value systemsetup printed for it.
func parseSystemsetupOutput(output []byte) map[string]string {
	result := make(map[string]string)

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		for _, s := range settings {
			if !strings.HasPrefix(line, s.outputPrefix) {
				continue
			}

			value := strings.TrimSpace(strings.TrimPrefix(line, s.outputPrefix))
			if s.isState {
				value = sanitizeState(value)
			}

			result[s.columnName] = value
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
