package dnf

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

func dnfParse(reader io.Reader) (any, error) {
	results := make([]map[string]string, 0)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		// We expect dnf to return lines in the following format:
		// `apr-util.x86_64 1.5.2-6.el7_9.1 updates`
		// We split on the last period in the first string, and on the spaces to get the following output:
		// `<package name>.<arch> <update version> <source>`
		fields := regexp.MustCompile("  *").Split(strings.TrimSpace(line), -1)
		if len(fields) != 3 {
			continue
		}

		splitIndex := strings.LastIndex(fields[0], ".")

		row := make(map[string]string)
		row["package"] = strings.TrimSpace(fields[0][:splitIndex])
		// Leaving this here in case we want to use it.
		//row["arch"] = strings.TrimSpace(fields[0][splitIndex+1:])
		row["version"] = strings.TrimSpace(fields[1])
		row["source"] = strings.TrimSpace(fields[2])

		results = append(results, row)
	}

	return results, nil
}
