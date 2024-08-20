package socketfilterfw

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

var appRegex = regexp.MustCompile("(.*)(?:\\s\\(state:\\s)([0-9]+)")
var lineRegex = regexp.MustCompile("(state|block|built-in|downloaded|stealth|log mode|log option)(?:.*\\s)([0-9a-z]+)")

// socketfilterfw returns lines for each `get` argument supplied.
// The output data is in the same order as the supplied arguments.
//
// This supports parsing the list of apps and their allow state, or
// each line describes a part of the feature and what state it's in.
//
// These are not very well-formed, so I'm doing some regex magic to
// know which option the current line is, and then sanitize the state.
func socketfilterfwParse(reader io.Reader) (any, error) {
	results := make([]map[string]string, 0)
	row := make(map[string]string)
	parseAppData := false

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		// When parsing the app list, the first line of output is a total
		// count of apps. We can break on this line to start parsing apps.
		if strings.Contains(line, "Total number of apps") {
			parseAppData = true
			continue
		}

		if parseAppData {
			appRow := parseAppList(line)
			if appRow != nil {
				results = append(results, appRow)
			}

			continue
		}

		k, v := parseLine(line)
		if k != "" {
			row[k] = v
		}
	}

	if len(row) > 0 {
		results = append(results, row)
	}

	return results, nil
}

// parseAppList parses the current line and returns the app name and
// state matches as a row of data.
func parseAppList(line string) map[string]string {
	matches := appRegex.FindStringSubmatch(line)
	if len(matches) != 3 {
		return nil
	}

	return map[string]string{
		"name":                       matches[1],
		"allow_incoming_connections": sanitizeState(matches[2]),
	}
}

// parseLine parse the current line and returns a feature key with the
// respective state/mode of said feature. We want all features to be a
// part of the same row of data, so we do not return this pair as a row.
func parseLine(line string) (string, string) {
	matches := lineRegex.FindStringSubmatch(strings.ToLower(line))
	if len(matches) != 3 {
		return "", ""
	}

	var key string
	switch matches[1] {
	case "state":
		key = "global_state_enabled"
	case "block":
		key = "block_all_enabled"
	case "built-in":
		key = "allow_built-in_signed_enabled"
	case "downloaded":
		key = "allow_downloaded_signed_enabled"
	case "stealth":
		key = "stealth_enabled"
	case "log mode":
		key = "logging_enabled"
	case "log option":
		key = "logging_option"
	default:
		return "", ""
	}

	return key, sanitizeState(matches[2])
}

// sanitizeState takes in a state like string and returns
// the correct boolean to create a consistent state value.
func sanitizeState(state string) string {
	switch state {
	// The app list state for when an app is blocking incoming connections
	// is output as `4`, while `1` is the state to allow those connections.
	case "0", "off", "disabled", "4":
		return "0"
	// When the "block all" firewall option is enabled, it doesn't
	// include a state like string, which is why we match on
	// the string value of "connections" for that mode.
	//
	// When both the Firewall and Stealth Mode are enabled,
	// the global firewall state value is `2` instead of `1`.
	case "1", "2", "on", "enabled", "connections":
		return "1"
	case "throttled", "brief", "detail":
		// The "logging option" value differs from the booleans.
		// Can be one of `throttled`, `brief`, or `detail`.
		return state
	default:
		return ""
	}
}
