package falconctl

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// parseOptions parses the stdout returned from falconctl's displayed options. As far as we know, output is a single
// line, comma seperated. We parse multiple lines, but assume data does not space that. Eg: linebreaks and commas
// treated as seperators.
func parseOptions(reader io.Reader) (any, error) {
	results := make(map[string]interface{})
	errorLines := make([]error, 0)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		hasErr := false
		pairs := strings.Split(line, ",")
		for _, pair := range pairs {
			pair = strings.TrimSpace(pair)

			// The format is quite inconsistent. The following sample shows 4 possible
			// outputs. We'll try to parse them all:
			//
			//	cid="ac917ab****************************"
			//	aid is not set
			//	aph is not set
			//	app is not set
			//	rfm-state is not set
			//	rfm-reason is not set
			//	feature is not set
			//	metadata-query=enable (unset default)
			//	version = 6.38.13501.0
			// We see 4 different formats. We'll try to parse them all.

			if strings.HasSuffix(pair, " is not set") {
				// What should this be set to? nil? "is not set"? TBD!
				results[pair[:len(pair)-len(" is not set")]] = "is not set"
				continue
			}

			kv := strings.SplitN(pair, "=", 1)
			if len(kv) == 2 {
				// remove quotes and extra spaces
				kv[1] = strings.TrimSpace(strings.Trim(kv[1], `"`))

				// Remove parenthetical unset note
				if strings.HasSuffix(kv[1], " (unset default)") {
					kv[1] = kv[1][:len(kv[1])-len(" (unset default)")]
				}
				results[kv[0]] = kv[1]
				continue
			}

			// Unknown format. Note the error
			hasErr = true
		}

		if hasErr {
			errorLines = append(errorLines, line)
		}
	}

	if len(errorLines) > 0 {
		return results, fmt.Errorf("parseOptions: %d lines could not be parsed: %v", len(errorLines), errorLines)
	}
	return results, nil
}
