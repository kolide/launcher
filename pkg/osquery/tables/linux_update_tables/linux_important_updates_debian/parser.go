package linux_important_updates_debian

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
)

func aptOutput(rawdata []byte) (map[string]map[string]string, error) {
	data := make(map[string]map[string]string)

	if len(rawdata) == 0 {
		return nil, errors.New("No data")
	}

	scanner := bufio.NewScanner(bytes.NewReader(rawdata))
	for scanner.Scan() {
		line := scanner.Text()
		pair := strings.Split(line, "/")
		if len(pair) < 2 {
			continue
		}

		package_name := strings.ToLower(strings.TrimSpace(pair[0]))
		if len(package_name) > 0 {
			values := strings.Split(pair[1], " ")
			if len(values) >= 6 {
				data[package_name] = make(map[string]string)
				data[package_name]["package"] = package_name
				data[package_name]["sources"] = strings.TrimSpace(values[0])
				data[package_name]["update_version"] = strings.TrimSpace(values[1])
				data[package_name]["current_version"] = strings.TrimRight(values[5], "]")
			}
		}
	}

	return data, nil
}

func dpkgOutput(rawdata []byte) (map[string]map[string]string, error) {
	var package_name string
	var allowedKeys = []string{
		"package",
		"essential",
		"priority",
		"section",
		"task",
		"build_essential",
	}
	data := make(map[string]map[string]string)
	dataset := make(map[string]string)

	if len(rawdata) == 0 {
		return nil, errors.New("No data")
	}

	scanner := bufio.NewScanner(bytes.NewReader(rawdata))
	for scanner.Scan() {
		line := scanner.Text()
		// dpkg gives puts a line break between package output.
		// We can use that to know when to start a new dataset.
		if len(line) == 0 {
			if len(package_name) > 0 {
				data[package_name] = dataset
			}

			dataset = make(map[string]string)
			continue
		}

		kv := strings.Split(line, ": ")
		if len(kv) < 2 {
			continue
		}

		// dpkg retutns a lot of unused data, so lets filter it.
		for _, allowedKey := range allowedKeys {
			var key = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(kv[0])), "-", "_")
			var value = strings.TrimSpace(kv[1])
			if key == allowedKey {
				if key == "package" {
					package_name = value
					continue
				}

				dataset[key] = value
			}
		}
	}

	return data, nil
}
