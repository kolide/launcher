package rpm

import (
	"bufio"
	"io"
	"strings"

	"golang.org/x/exp/slices"
)

var allowedKeys = []string{
	"name",
	"version",
	"release",
	"install date",
	"group",
	"build date",
	"summary",
	"description",
}

func rpmParse(reader io.Reader) (any, error) {
	results := make([]map[string]string, 0)
	row := make(map[string]string)
	readingDesc := false

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		// We expect rpm to return lines in the following format:
		// `Name: pm-utils`
		// `Version: 1.4.1`
		// `Release: 27.el7`...
		// We split each line to get a key -> value pair, then store
		// it into our row until we log the row and start a new one.
		kv := strings.SplitN(line, ":", 2)
		var key = strings.ToLower(strings.TrimSpace(kv[0]))
		if slices.Contains(allowedKeys, key) {
			// rpm doesn't provide a clean break. I'm collecting everything until
			// description, then storing each line for description until the next key.
			if key == "description" {
				readingDesc = true
			} else if readingDesc {
				readingDesc = false
				results = append(results, row)
				row = make(map[string]string)
			}

			row[key] = strings.TrimSpace(kv[1])
		} else if readingDesc {
			row["description"] = strings.TrimSpace(row["description"] + " " + strings.TrimSpace(line))
		}
	}

	if len(row) > 0 {
		results = append(results, row)
	}

	return results, nil
}
