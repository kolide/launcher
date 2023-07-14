package apt

import (
	"bufio"
	//"fmt"
	"io"
	"strings"
)

func aptParse(reader io.Reader) (any, error) {
	results := make([]map[string]string, 0)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		pair := strings.Split(line, "/")
		if len(pair) < 2 {
			continue
		}

		package_name := strings.ToLower(strings.TrimSpace(pair[0]))
		if len(package_name) < 1 {
			continue
		}

		values := strings.Split(pair[1], " ")
		if len(values) < 6 {
			continue
		}

		row := make(map[string]string)
		row["package"] = package_name
		row["sources"] = strings.TrimSpace(values[0])
		row["update_version"] = strings.TrimSpace(values[1])
		row["current_version"] = strings.TrimRight(values[5], "]")

		results = append(results, row)
	}

	return results, nil
}
