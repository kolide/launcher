//go:build darwin
// +build darwin

package remotectl

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// parseDumpstate parses the result of /usr/libexec/remotectl into a map that can be flattened by dataflatten.
// We expect results in the following format, with empty newlines separating devices:
//
//	<device name>
//			Key: Value
//			Properties: {
//				Key => Value
//			}
//			Services:
//				service1
//				service2
func parseDumpstate(reader io.Reader) (any, error) {
	results := make(map[string]map[string]interface{})

	currentDeviceName := ""
	insidePropertiesDictionary := false
	insideServicesArray := false

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		// We need a device name to proceed -- if present, initialize its data in `results` and move on
		if isDeviceName(line) {
			currentDeviceName = extractDeviceName(line)
			results[currentDeviceName] = make(map[string]interface{})
			results[currentDeviceName]["Properties"] = make(map[string]interface{})
			results[currentDeviceName]["Services"] = make([]string, 0)
			continue
		}

		if currentDeviceName == "" {
			return nil, fmt.Errorf("no device name(s) given in remotectl dumpstate output")
		}

		if isDeviceDelimiter(line) {
			// We've reached a new device -- reset state and continue
			currentDeviceName = ""
			insidePropertiesDictionary = false
			insideServicesArray = false
			continue
		}

		line = strings.TrimSpace(line)

		if insidePropertiesDictionary {
			if line == "}" {
				insidePropertiesDictionary = false
				continue
			}

			propertyKey, propertyValue, err := extractPropertyKeyValue(line)
			if err != nil {
				return nil, err
			}

			properties := results[currentDeviceName]["Properties"].(map[string]interface{})
			properties[propertyKey] = propertyValue
		} else if insideServicesArray {
			services := results[currentDeviceName]["Services"].([]string)
			services = append(services, line)
			results[currentDeviceName]["Services"] = services
		} else {
			if strings.HasPrefix(line, "Properties:") {
				insidePropertiesDictionary = true
			} else if strings.HasPrefix(line, "Services:") {
				insideServicesArray = true
			} else {
				// We have a top-level key with a value we should extract to store in `results`
				propertyKey, propertyValue, err := extractTopLevelKeyValue(line)
				if err != nil {
					return nil, err
				}
				results[currentDeviceName][propertyKey] = propertyValue
			}
		}
	}

	return results, nil
}

func isDeviceName(line string) bool {
	// If the line is not indented (i.e. top-level), we have a device name
	return !strings.HasPrefix(line, "\t")
}

func extractDeviceName(line string) string {
	// Devices (besides "Local device") are identified as `Found <name> (<type>)` -- strip "Found"
	return strings.TrimSpace(strings.TrimPrefix(line, "Found"))
}

func isDeviceDelimiter(line string) bool {
	// A newline indicates a new device's information is coming next
	return strings.TrimSpace(line) == ""
}

func extractPropertyKeyValue(line string) (string, string, error) {
	// key-value pairs in the `Properties` dictionary are in the format `key => value`
	extracted := strings.Split(line, "=>")
	if len(extracted) != 2 {
		return "", "", errors.New("key/value pair in properties in remotectl output is in an unexpected format")
	}

	return strings.TrimSpace(extracted[0]), strings.TrimSpace(extracted[1]), nil
}

func extractTopLevelKeyValue(line string) (string, string, error) {
	// Top-level key-value pairs are in the format `key: value`
	extracted := strings.Split(line, ":")
	if len(extracted) != 2 {
		return "", "", errors.New("top-level key/value pair in remotectl output is in an unexpected format")
	}

	return strings.TrimSpace(extracted[0]), strings.TrimSpace(extracted[1]), nil
}
