//go:build darwin

package pkgutil

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/kolide/launcher/v2/ee/tables/execparsers/key_value"
)

var pkgInfoKeyMap = map[string]string{
	"package-id":   "package_id",
	"version":      "version",
	"volume":       "volume",
	"location":     "location",
	"install-time": "install_time",
	"groups":       "groups",
}

// parsePkgInfoOutput parses the output of `pkgutil --pkg-info=<package_id>`.
// install_time is validated and normalized as a Unix timestamp in seconds.
func parsePkgInfoOutput(output []byte) (map[string]string, error) {
	parsed, err := key_value.NewWithDelimiter(":").Parse(bytes.NewReader(output))
	if err != nil {
		return nil, fmt.Errorf("parsing pkg-info output: %w", err)
	}

	raw, ok := parsed.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected pkg-info parse result type %T", parsed)
	}

	result := make(map[string]string)

	for rawKey, column := range pkgInfoKeyMap {
		value, ok := raw[rawKey]
		if !ok {
			continue
		}

		valueStr, err := pkgInfoValue(value)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", column, err)
		}

		valueStr = strings.TrimSpace(valueStr)
		if valueStr == "" {
			continue
		}

		if column == "install_time" {
			installTime, err := strconv.ParseInt(valueStr, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing install-time unix timestamp %q: %w", valueStr, err)
			}

			if installTime < 0 {
				return nil, fmt.Errorf("invalid install-time unix timestamp %d", installTime)
			}

			result[column] = strconv.FormatInt(installTime, 10)
			continue
		}

		result[column] = valueStr
	}

	return result, nil
}

func pkgInfoValue(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return "", fmt.Errorf("unexpected pkg-info value type %T", item)
			}

			parts = append(parts, s)
		}

		return strings.Join(parts, ","), nil
	default:
		return "", fmt.Errorf("unexpected pkg-info value type %T", value)
	}
}
