package socketfilterfw

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var lineRegex = regexp.MustCompile("(state|block|built-in|downloaded|stealth|log mode|log option)(?:.*\\s)([0-9a-z]+)")
var disabledStateRegex = regexp.MustCompile("(0|off|disabled)")

// socketfilterfw returns lines for each `get` argument supplied.
// The output data is in the same order as the supplied arguments.
//
// Each line describes a part of the feature and what state it's in.
// These are not very well-formed, so I'm doing some regex magic to
// know which option the current line is, and then sanitize the state.
func socketfilterfwParse(reader io.Reader) (any, error) {
	results := make([]map[string]string, 0)
	row := make(map[string]string)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		matches := lineRegex.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}

		var key string
		value := sanitizeState(matches[2])

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
			// The logging option value differs from the booleans.
			// Can be one of `throttled`, `brief`, or `detail`.
			value = matches[2]
		default:
			continue
		}

		row[key] = value
	}

	// There should only be one row of data for application firewall,
	// so this append is slightly awkward but should be fine.
	if len(row) > 0 {
		results = append(results, row)
	}

	return results, nil
}

// sanitizeState takes in a state like string and returns
// the correct boolean to create a consistent state value.
//
// When the "block all" firewall option is enabled, it doesn't
// include a state like string, which is why we search for
// a disabled state, and return the reversed value of that match.
func sanitizeState(state string) string {
	return strconv.FormatBool(!disabledStateRegex.MatchString(state))
}
