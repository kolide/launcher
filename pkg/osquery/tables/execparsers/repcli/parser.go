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

func NewStatusTemplate() map[string]map[string]any {
	return map[string]map[string]interface{}{
		"general_info": {
			"devicehash":         "",
			"deviceid":           "",
			"quarantine":         "",
			"sensor_version":     "",
			"sensor_restarts":    "",
			"kernel_type":        "",
			"system_extension":   "",
			"kernel_file_filter": "",
			"background_scan":    "",
			"last_reset":         "",
			"fips_mode":          "",
		},
		"full_disk_access_configurations": {
			"repmgr":           "",
			"system_extension": "",
			"osquery":          "",
			"uninstall_helper": "",
			"uninstall_ui":     "",
		},
		"sensor_status": {
			"status":                      "",
			"details":                     make(map[string][]string, 0),
			"svcstable":                   "",
			"boot_count":                  "",
			"first_boot_after_os_upgrade": "",
			"service_uptime":              "",
			"service_waketime":            "",
		},
		"cloud_status": {
			"registered":         "",
			"server_address":     "",
			"next_check-in":      "",
			"private_logging":    "",
			"next_cloud_upgrade": "",
			"mdm_device_id":      "",
			"platform_type":      "",
			"proxy":              "",
		},
		"proxy_settings": {
			"proxy_configured": "",
		},
		"enforcement_status": {
			"execution_blocks":     "",
			"network_restrictions": "",
		},
		"rules_status": {
			"policy_name":               "",
			"policy_timestamp":          "",
			"endpoint_standard_product": "",
			"enterprise_edr_product":    "",
			"active_policies":           make(map[string]string, 0),
		},
	}
}

// formatKey prepares raw (potentially multi-word) key values by:
// - stripping all surrounding whitespace
// - coercing the entire string to lowercase
// - splitting multiple words and joining them as snake_case
func formatKey(key string) string {
	processed := strings.TrimSpace(strings.ToLower(key))
	words := strings.Fields(processed)
	// State and Status are used interchangeably in section headers
	// across device types, so we coerce all to status here
	for idx, word := range words {
		if word == "state" {
			words[idx] = "status"
		}
	}

	return strings.Join(words, "_")
}

func repcliParse(reader io.Reader) (any, error) {
	results := NewStatusTemplate()
	var sectionHeaders []string
	nestedLevel := 0

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		rawLine := scanner.Text()
		if len(rawLine) == 0 {
			continue // blank lines are not expected or meaningful
		}

		kv := strings.SplitN(rawLine, ":", 2)
		if len(kv) < 2 {
			continue // lines without a colon are not expected or meaningful
		}

		formattedKey := formatKey(kv[0])
		formattedValue := strings.TrimSpace(kv[1])
		isSectionHeader := len(formattedValue) == 0
		// a nested section header has left padding and no associated value
		currentNestedLevel := len(kv[0]) - len(strings.TrimLeftFunc(kv[0], unicode.IsSpace))
		if isSectionHeader && currentNestedLevel <= 0 {
			// reset key paths for any new, top-level header
			sectionHeaders = []string{}
		}

		if isSectionHeader {
			nestedLevel = currentNestedLevel
			sectionHeaders = append(sectionHeaders, formattedKey)
			continue
		}
		// from this point forward we expect that we're working with a
		// line containing a full key-value pair. the following logic
		// supports up to 2 levels of nesting
		if len(sectionHeaders) > 2 || len(sectionHeaders) < 1 {
			continue // log?
		}

		// if this is the case then we're leaving a nested section (and should
		// re-enter the parent section instead)
		if currentNestedLevel < nestedLevel && len(sectionHeaders) > 1 {
			sectionHeaders = sectionHeaders[0 : len(sectionHeaders)-1]
		}

		if _, ok := results[sectionHeaders[0]]; !ok {
			continue // we don't handle this section
		}

		if len(sectionHeaders) == 1 {
			if _, ok := results[sectionHeaders[0]][formattedKey]; !ok {
				continue // we don't handle this key
			}

			results[sectionHeaders[0]][formattedKey] = formattedValue
			continue
		}

		if _, ok := results[sectionHeaders[0]][sectionHeaders[1]]; !ok {
			continue
		}

		switch results[sectionHeaders[0]][sectionHeaders[1]].(type) {
		case map[string][]string:
			if coerced, ok := results[sectionHeaders[0]][sectionHeaders[1]].(map[string][]string); ok {
				coerced[formattedKey] = append(coerced[formattedKey], formattedValue)
			}
		case map[string]string:
			if coerced, ok := results[sectionHeaders[0]][sectionHeaders[1]].(map[string]string); ok {
				coerced[formattedKey] = formattedValue
			}
		default:
			// if additional nested types are supported they should be added to the switch here
			continue
		}

		nestedLevel = currentNestedLevel
	}

	return results, nil
}
