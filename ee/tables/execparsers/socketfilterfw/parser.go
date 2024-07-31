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
	parse_app_data := false

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		// When parsing the app list, the first line of output is a total
		// count of apps. We can break on this line to start parsing apps.
		if strings.Contains(line, "Total number of apps") {
			parse_app_data = true

			if len(row) > 0 {
				results = append(results, row)
				row = make(map[string]string)
			}

			continue
		}

		var matches []string
		if parse_app_data {
			matches = appRegex.FindStringSubmatch(line)
		} else {
			matches = lineRegex.FindStringSubmatch(strings.ToLower(line))
		}

		if len(matches) != 3 {
			continue
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
			if parse_app_data {
				row["name"] = matches[1]
				row["allow_incoming_connections"] = sanitizeState(matches[2])
				results = append(results, row)
				row = make(map[string]string)
			}

			continue
		}

		row[key] = sanitizeState(matches[2])
	}

	if len(row) > 0 {
		results = append(results, row)
	}

	return results, nil
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
	case "1", "on", "enabled", "connections":
		return "1"
	case "throttled", "brief", "detail":
		// The "logging option" value differs from the booleans.
		// Can be one of `throttled`, `brief`, or `detail`.
		return state
	default:
		return ""
	}
}
