package repcli

// repcli is responsible for parsing the output of the CarbonBlack
// repcli sensor status utility. Some of the output format has
// changed from the published documentation, as noted here:
// https://community.carbonblack.com/t5/Knowledge-Base/Endpoint-Standard-How-to-Verify-Sensor-Status-With-RepCLI/ta-p/62524

import (
	"bufio"
	"io"
	"strings"
	"unicode"
)

// formatKey prepares raw (potentially multi-word) key values by:
// - stripping all surrounding whitespace
// - coercing the entire string to lowercase
// - splitting multiple words and joining them as snake_case
func formatKey(key string) string {
	processed := strings.TrimSpace(strings.ToLower(key))
	words := strings.Fields(processed)
	return strings.Join(words, "_")
}

// parseLine reads the next line from a scanner and attempts to
// pull out the key, value, and key depth (level of nesting).
// an empty key-value pair is returned if the line is malformed
func parseLine(scanner *bufio.Scanner) (string, string, int) {
	line := scanner.Text()
	if len(line) == 0 {
		return "", "", 0 // blank lines are not expected or meaningful
	}

	kv := strings.SplitN(line, ":", 2)
	if len(kv) < 2 {
		return "", "", 0 // lines without a colon are not expected or meaningful
	}

	nestedDepth := len(kv[0]) - len(strings.TrimLeftFunc(kv[0], unicode.IsSpace))

	return formatKey(kv[0]), strings.TrimSpace(kv[1]), nestedDepth
}

// parseSection works recursively to handle nested sections. There is slight duplication
// between the outer scanner.Scan() call here and in the main repcliParse function to ensure
// this would still function as expected with arbitrary levels of nesting while supporting
// standard sections at the top level.
// This returns the given results for the section, along with a boolean flag indicating
// that the previous line should be re-read when the parsing determines we've moved
// back outside the target depth.
func parseSection(scanner *bufio.Scanner, currentDepth int) (map[string]any, bool) {
	results := make(map[string]any)
	skipScan := false
	for skipScan || scanner.Scan() {
		skipScan = false // always reset here because we'll have completed the skip
		key, value, nextDepth := parseLine(scanner)

		if key == "" {
			continue
		}

		isSectionHeader := len(value) == 0
		if nextDepth <= currentDepth {
			// we must pass skipScan here because we'll need to re-read this line on the next pass
			return results, true
		}

		if isSectionHeader {
			results[key], skipScan = parseSection(scanner, nextDepth)
			continue
		}

		// handle any cases where there is already a value set for key
		if existingValue, ok := results[key]; ok {
			switch existingValue.(type) {
			case []string:
				results[key] = append(results[key].([]string), value)
			case string:
				results[key] = []string{results[key].(string), value}
			default:
				// if additional nested types are supported they should be added to the switch here
				continue
			}

			continue
		}

		results[key] = value
	}

	return results, false
}

// repcliParse will take a reader containing stdout data from a cli invocation of repcli.
// We are expecting to parse something like the following into an arbitrarily-nested map[string]any:
//
// General Info:
//
//	Sensor Version: 2.14.0.1234321
//	DeviceHash: test6b7v9Xo5bX50okW5KABCD+wHxb/YZeSzrZACKo0=
//
// Sensor Status:
//
//	State: Enabled
func repcliParse(reader io.Reader) (any, error) {
	scanner := bufio.NewScanner(reader)
	results := make(map[string]any)
	skipScan := false
	var section map[string]any
	for skipScan || scanner.Scan() {
		skipScan = false
		key, value, nextDepth := parseLine(scanner)

		if key == "" {
			continue
		}

		// we don't currently expect but would support any top-level key-value pairs
		if value != "" {
			results[key] = value
			continue
		}

		section, skipScan = parseSection(scanner, nextDepth)
		results[key] = section
	}

	return results, nil
}
