package dpkg

import (
	"bufio"
	//"fmt"
	"io"
	"strings"

	"golang.org/x/exp/slices"
)

var allowedKeys = []string{
	"package",
	"essential",
	"priority",
	"section",
	"version",
	"description",
	"build_essential",
}

func dpkgParse(reader io.Reader) (any, error) {
	results := make([]map[string]string, 0)
	row := make(map[string]string)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			results = append(results, row)
			row = make(map[string]string)
			continue
		}

		kv := strings.Split(line, ": ")
		if len(kv) < 2 {
			continue
		}

		var key = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(kv[0])), "-", "_")
		if slices.Contains(allowedKeys, key) {
			row[key] = strings.TrimSpace(kv[1])
		}
	}

	return results, nil
}
