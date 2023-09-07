package repcli

// repcli is responsible for parsing the output of the CarbonBlack
// repcli sensor status utility. Some of the output format has
// changed from the published documentation, as noted here:
// https://community.carbonblack.com/t5/Knowledge-Base/Endpoint-Standard-How-to-Verify-Sensor-Status-With-RepCLI/ta-p/62524

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode"
)

type (
	resultMap map[string]any

	repcliLine struct {
		isSectionHeader bool
		indentLevel     int
		key             string
		value           string
	}
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

// parseLine reads the next line from a scanner and attempts to pull out the
// key, value, and key depth (level of nesting) into a repcliLine struct.
// an empty key-value pair is returned if the line is malformed
func parseLine(line string) *repcliLine {
	if len(line) == 0 {
		return nil // blank lines are not expected or meaningful
	}

	kv := strings.SplitN(line, ":", 2)
	if len(kv) < 2 {
		return nil // lines without a colon are not expected or meaningful
	}

	indentLen := len(kv[0]) - len(strings.TrimLeftFunc(kv[0], unicode.IsSpace))
	formattedValue := strings.TrimSpace(kv[1])

	return &repcliLine{
		isSectionHeader: (len(formattedValue) == 0),
		indentLevel:     indentLen,
		key:             formatKey(kv[0]),
		value:           formattedValue,
	}
}

func updatedKeyPaths(currentPaths []*repcliLine, newSection *repcliLine) []*repcliLine {
	updatedPaths := make([]*repcliLine, 0)

	if len(currentPaths) == 0 {
		return append(updatedPaths, newSection)
	}

	for idx, sectionLine := range currentPaths {
		// we only let this fall through if we should add in the new section at the very end
		if newSection.indentLevel > sectionLine.indentLevel {
			updatedPaths = append(updatedPaths, sectionLine)
			continue
		}

		// we've gone too far and need to replace the previous key
		if newSection.indentLevel < sectionLine.indentLevel {
			return append(currentPaths[:idx-1], newSection)
		}

		// this key is at the same level as our new section, replace that in the currentPaths
		return append(currentPaths[:idx], newSection)
	}

	return append(updatedPaths, newSection)
}

// setNestedValue works to recursively dive into the resultMap while traversing the
// lines provided to set the final (deepest) value
func setNestedValue(results resultMap, lines []*repcliLine) error {
	if len(lines) == 0 {
		return fmt.Errorf("at least one line is required to set the value")
	}

	key, value := lines[0].key, lines[0].value
	if len(lines) == 1 {
		// handle any cases where there is already a value set for key
		switch knownValue := results[key].(type) {
		case []string:
			results[key] = append(knownValue, value)
		case string:
			results[key] = []string{knownValue, value}
		case resultMap, interface{}, nil:
			results[key] = value
		default:
			// if additional nested types are required they should be added above
			return fmt.Errorf("unknown type %T requested for nested set", knownValue)
		}

		return nil
	}

	_, ok := results[key]
	if !ok {
		results[key] = make(resultMap, 0)
	}

	return setNestedValue(results[key].(resultMap), lines[1:])
}

// repcliParse will take a reader containing stdout data from a cli invocation of repcli.
// We are expecting to parse something like the following into an arbitrarily-nested map[string]any:
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
	currentKeyPaths := make([]*repcliLine, 0)
	for scanner.Scan() {
		line := parseLine(scanner.Text())
		if line == nil {
			continue
		}

		currentKeyPaths = updatedKeyPaths(currentKeyPaths, line)

		if line.isSectionHeader {
			continue
		}

		setNestedValue(results, currentKeyPaths)
	}

	return results, nil
}
